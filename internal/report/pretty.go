package report

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/lint"
	"github.com/fregateops/vigie/internal/runner"
)

var (
	passColor     = color.New(color.FgGreen, color.Bold)
	failColor     = color.New(color.FgRed, color.Bold)
	skipColor     = color.New(color.FgYellow)
	nameColor     = color.New(color.FgCyan)
	warnColor     = color.New(color.FgYellow, color.Bold)
	infoColor     = color.New(color.FgBlue)
	durationColor = color.New(color.Faint)
)

// formatDuration renders a duration compactly for the pretty reporter.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// Report writes test-suite results in a human-readable format.
func (r *PrettyReporter) Report(results []runner.SuiteResult) error {
	if r.CI != cienv.KindNone && os.Getenv("NO_COLOR") == "" {
		color.NoColor = false
	}

	ciw := newCIWriter(r.CI, r.Out)
	pass, fail, skip := 0, 0, 0
	var totalTestTime time.Duration

	for _, sr := range results {
		suiteTitle := fmt.Sprintf("%s (%s)", sr.Suite, formatDuration(sr.Duration))
		allPass := !runner.SuiteHasFailure(sr)
		ciw.GroupStart(suiteTitle, allPass)

		fmt.Fprintf(r.Out, "\n%s  %s\n", nameColor.Sprint(sr.Suite), durationColor.Sprintf("(%s)", formatDuration(sr.Duration)))
		fmt.Fprintf(r.Out, "%s\n", strings.Repeat("─", 60))

		for _, tr := range sr.Results {
			totalTestTime += tr.Duration
			timing := durationColor.Sprintf("(%s)", formatDuration(tr.Duration))
			switch {
			case tr.Skipped:
				skip++
				if tr.SkipReason != "" {
					fmt.Fprintf(r.Out, "  %s  %s  %s  %s\n", skipColor.Sprint("SKIP"), tr.TestName, durationColor.Sprintf("(%s)", tr.SkipReason), timing)
				} else {
					fmt.Fprintf(r.Out, "  %s  %s  %s\n", skipColor.Sprint("SKIP"), tr.TestName, timing)
				}
			case tr.Pass:
				pass++
				fmt.Fprintf(r.Out, "  %s  %s  %s\n", passColor.Sprint("PASS"), tr.TestName, timing)
			default:
				fail++
				fmt.Fprintf(r.Out, "  %s  %s  %s\n", failColor.Sprint("FAIL"), tr.TestName, timing)
				for _, f := range tr.Failures {
					fmt.Fprintf(r.Out, "%s\n", f)
				}
				ciw.Annotation("error", sr.File, 0, sr.Suite+"/"+tr.TestName, strings.Join(tr.Failures, "\n"))
			}
		}

		ciw.GroupEnd()
	}

	fmt.Fprintf(r.Out, "\n%s\n", strings.Repeat("─", 60))
	total := pass + fail + skip
	summary := fmt.Sprintf("Tests: %d total", total)
	if pass > 0 {
		summary += fmt.Sprintf(", %s passed", passColor.Sprintf("%d", pass))
	}
	if fail > 0 {
		summary += fmt.Sprintf(", %s failed", failColor.Sprintf("%d", fail))
	}
	if skip > 0 {
		summary += fmt.Sprintf(", %s skipped", skipColor.Sprintf("%d", skip))
	}
	summary += fmt.Sprintf(" %s", durationColor.Sprintf("(%s total test time)", formatDuration(totalTestTime)))
	fmt.Fprintln(r.Out, summary)

	return nil
}

// PrettyReporter writes human-readable colored output.
type PrettyReporter struct {
	Out io.Writer
	CI  cienv.Kind
}

// ReportLint writes lint findings in a human-readable format.
func (r *PrettyReporter) ReportLint(result lint.Result) error {
	if r.CI != cienv.KindNone && os.Getenv("NO_COLOR") == "" {
		color.NoColor = false
	}

	ciw := newCIWriter(r.CI, r.Out)
	noErrors := !result.HasErrors()
	ciw.GroupStart("Lint: "+result.ChartPath, noErrors)

	fmt.Fprintf(r.Out, "\n%s\n", nameColor.Sprintf("Lint: %s", result.ChartPath))
	fmt.Fprintf(r.Out, "%s\n", strings.Repeat("─", 60))

	errors, warnings, infos := 0, 0, 0
	for _, f := range result.Findings {
		switch f.Severity {
		case lint.SeverityError:
			errors++
			fmt.Fprintf(r.Out, "  %s  [%s] %s\n", failColor.Sprint("ERROR"), f.RuleID, f.Message)
		case lint.SeverityWarning:
			warnings++
			fmt.Fprintf(r.Out, "  %s  [%s] %s\n", warnColor.Sprint("WARN "), f.RuleID, f.Message)
		default:
			infos++
			fmt.Fprintf(r.Out, "  %s  [%s] %s\n", infoColor.Sprint("INFO "), f.RuleID, f.Message)
		}
		ciw.Annotation(severityToCI(f.Severity), f.File, f.Line, f.RuleID, f.Message)
	}

	fmt.Fprintf(r.Out, "\n%s\n", strings.Repeat("─", 60))
	if len(result.Findings) == 0 {
		fmt.Fprintf(r.Out, "%s  no findings\n", passColor.Sprint("PASS"))
	} else {
		fmt.Fprintf(r.Out, "Findings: %d total", len(result.Findings))
		if errors > 0 {
			fmt.Fprintf(r.Out, ", %s", failColor.Sprintf("%d errors", errors))
		}
		if warnings > 0 {
			fmt.Fprintf(r.Out, ", %s", warnColor.Sprintf("%d warnings", warnings))
		}
		if infos > 0 {
			fmt.Fprintf(r.Out, ", %s", infoColor.Sprintf("%d info", infos))
		}
		fmt.Fprintln(r.Out)
	}

	ciw.GroupEnd()
	return nil
}

// severityToCI maps lint severity to a GitHub Actions annotation level.
func severityToCI(sev lint.Severity) string {
	switch sev {
	case lint.SeverityError:
		return "error"
	case lint.SeverityWarning:
		return "warning"
	default:
		return "notice"
	}
}
