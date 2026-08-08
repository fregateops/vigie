package report

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/fregateops/vigie/internal/cienv"
	"github.com/fregateops/vigie/internal/lint"
)

var (
	passColor = color.New(color.FgGreen, color.Bold)
	failColor = color.New(color.FgRed, color.Bold)
	nameColor = color.New(color.FgCyan)
	warnColor = color.New(color.FgYellow, color.Bold)
	infoColor = color.New(color.FgBlue)
)

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
