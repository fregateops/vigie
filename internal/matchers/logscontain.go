package matchers

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fregateops/vigie/internal/dsl"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func init() {
	Register(simpleMatcher{
		name:     "logsContain",
		matches:  func(a dsl.Assertion) bool { return a.LogsContain != nil },
		tiers:    Tiers(TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalLogsContain(a.LogsContain, ctx) },
	})
}

const (
	errLogsContainTierMismatch = "logsContain matcher requires a real-cluster backend (run test with --cluster kind|k3d|kubeconfig; envtest has no kubelet to source logs from)"
	defaultLogsWithin          = 30 * time.Second
	debugTailLines             = 20
)

func evalLogsContain(spec *dsl.LogsAssert, ctx EvalContext) Result {
	if !ctx.InApplyTier || ctx.RESTConfig == nil {
		return Result{Pass: false, Message: errLogsContainTierMismatch}
	}

	clientset, err := kubernetes.NewForConfig(ctx.RESTConfig)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("logsContain: failed to build kubernetes client: %v", err)}
	}

	within, err := parseWithin(spec.Within)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("logsContain: invalid within duration %q: %v", spec.Within, err)}
	}

	matcher, err := compilePattern(spec.Pattern)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("logsContain: invalid pattern %q: %v", spec.Pattern, err)}
	}

	deadline, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()

	pods, err := selectPods(deadline, ctx, clientset, spec.Pod)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("logsContain: pod selection failed: %v", err)}
	}
	if len(pods) == 0 {
		return Result{Pass: false, Message: fmt.Sprintf("logsContain: no pods matched selector %+v", spec.Pod)}
	}

	type streamResult struct {
		matched   bool
		lastLines []string
		podName   string
	}
	results := make(chan streamResult, len(pods))

	for _, pod := range pods {
		go func(podName, namespace string) {
			matched, lastLines := streamPodLogs(deadline, clientset, podName, namespace, spec.Container, matcher)
			results <- streamResult{matched: matched, lastLines: lastLines, podName: podName}
		}(pod.Name, pod.Namespace)
	}

	var failDebug []string
	for range pods {
		res := <-results
		if res.matched {
			return Result{Pass: true}
		}
		failDebug = append(failDebug, fmt.Sprintf("pod %s last %d lines: %s", res.podName, debugTailLines, strings.Join(res.lastLines, "\n")))
	}

	return Result{
		Pass:    false,
		Message: fmt.Sprintf("logsContain: pattern %q not found within %s\n%s", spec.Pattern, within, strings.Join(failDebug, "\n---\n")),
	}
}

// compilePattern returns a function that checks whether a log line matches the
// pattern. Patterns wrapped in /.../ are treated as regular expressions;
// everything else is a plain substring match.
func compilePattern(pattern string) (func(string) bool, error) {
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") && len(pattern) >= 2 {
		inner := pattern[1 : len(pattern)-1]
		if inner == "" {
			return nil, fmt.Errorf("empty regex pattern %q", pattern)
		}
		re, err := regexp.Compile(inner)
		if err != nil {
			return nil, err
		}
		return re.MatchString, nil
	}
	return func(line string) bool { return strings.Contains(line, pattern) }, nil
}

// parseWithin parses a duration string, defaulting to defaultLogsWithin when empty.
func parseWithin(within string) (time.Duration, error) {
	if within == "" {
		return defaultLogsWithin, nil
	}
	return time.ParseDuration(within)
}

// selectPods resolves the pods described by the selector. Direct name lookup
// takes precedence over labelSelector.
func selectPods(callCtx context.Context, evalCtx EvalContext, clientset kubernetes.Interface, sel dsl.LogsPodSelector) ([]corev1.Pod, error) {
	namespace := resolveNS(sel.Namespace, evalCtx)

	if sel.Name != "" {
		pod, err := clientset.CoreV1().Pods(namespace).Get(callCtx, sel.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get pod %q in %q: %w", sel.Name, namespace, err)
		}
		return []corev1.Pod{*pod}, nil
	}

	if sel.LabelSelector != "" {
		list, err := clientset.CoreV1().Pods(namespace).List(callCtx, metav1.ListOptions{
			LabelSelector: sel.LabelSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("list pods with selector %q in %q: %w", sel.LabelSelector, namespace, err)
		}
		return list.Items, nil
	}

	return nil, fmt.Errorf("pod selector requires either name or labelSelector")
}

// streamPodLogs follows pod logs until the context deadline, returning true and
// an empty slice on first match, or false and the last debugTailLines lines on
// deadline expiry.
func streamPodLogs(ctx context.Context, clientset kubernetes.Interface, podName, namespace, container string, matches func(string) bool) (bool, []string) {
	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, buildLogOptions(container))
	stream, err := req.Stream(ctx)
	if err != nil {
		return false, []string{fmt.Sprintf("error opening log stream: %v", err)}
	}
	defer func() { _ = stream.Close() }()

	var ring []string
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		line := scanner.Text()
		if matches(line) {
			return true, nil
		}
		ring = appendRing(ring, line, debugTailLines)
	}
	return false, ring
}

func buildLogOptions(container string) *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{Follow: true}
	if container != "" {
		opts.Container = container
	}
	return opts
}

// appendRing appends line to buf, evicting the oldest entry when the buffer
// exceeds maxLen. This avoids allocating a proper ring buffer for the small
// debug tail we need.
func appendRing(buf []string, line string, maxLen int) []string {
	buf = append(buf, line)
	if len(buf) > maxLen {
		buf = buf[len(buf)-maxLen:]
	}
	return buf
}
