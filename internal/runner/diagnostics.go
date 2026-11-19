package runner

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// diagnosticsTimeout bounds the reads made after a failure, so a sick API
	// server cannot stall the report.
	diagnosticsTimeout = 10 * time.Second
	// maxDiagnosticEvents keeps the most recent warnings; older ones are
	// usually repeats of the same cause.
	maxDiagnosticEvents = 8
)

// namespaceDiagnostics describes why workloads in namespace may be unhealthy:
// pods that are not Ready, with their containers' waiting/terminated reasons,
// and the latest Warning events. It returns failure lines ready to append, or
// none when there is nothing to report.
func namespaceDiagnostics(clientset kubernetes.Interface, namespace string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), diagnosticsTimeout)
	defer cancel()

	var lines []string
	if pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{}); err == nil {
		for _, pod := range pods.Items {
			if line, unhealthy := describePod(pod); unhealthy {
				lines = append(lines, line)
			}
		}
	}
	if events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "type=Warning"}); err == nil {
		lines = append(lines, describeEvents(events.Items)...)
	}
	if len(lines) == 0 {
		return nil
	}
	out := []string{fmt.Sprintf("        diagnostics (namespace %s):", namespace)}
	for _, line := range lines {
		out = append(out, "          "+line)
	}
	return out
}

func describePod(pod corev1.Pod) (string, bool) {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return "", false
		}
	}
	var reasons []string
	for _, cs := range slices.Concat(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses) {
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			reasons = append(reasons, fmt.Sprintf("%s %s: %s", cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message))
		case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
			reasons = append(reasons, fmt.Sprintf("%s exited %d (%s)", cs.Name, cs.State.Terminated.ExitCode, cs.State.Terminated.Reason))
		}
		if cs.RestartCount > 0 {
			reasons = append(reasons, fmt.Sprintf("%s restarted %d time(s)", cs.Name, cs.RestartCount))
		}
	}
	line := fmt.Sprintf("pod %s not ready (%s)", pod.Name, pod.Status.Phase)
	if len(reasons) > 0 {
		line += ": " + strings.Join(reasons, "; ")
	}
	return line, true
}

// eventTime is when an event last fired: core/v1 producers set LastTimestamp,
// events.k8s.io producers only EventTime.
func eventTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	return ev.EventTime.Time
}

func describeEvents(events []corev1.Event) []string {
	slices.SortFunc(events, func(a, b corev1.Event) int {
		return eventTime(a).Compare(eventTime(b))
	})
	if len(events) > maxDiagnosticEvents {
		events = events[len(events)-maxDiagnosticEvents:]
	}
	lines := make([]string, len(events))
	for i, ev := range events {
		lines[i] = fmt.Sprintf("event %s/%s %s: %s", ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Reason, strings.TrimSpace(ev.Message))
	}
	return lines
}
