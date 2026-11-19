package runner

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNamespaceDiagnostics_ReportsUnreadyPodsAndLatestWarnings(t *testing.T) {
	const ns = "vg-test"
	ready := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ok", Namespace: ns},
		Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	pulling := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: ns},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull access denied"}},
			}},
		},
	}
	objs := []runtime.Object{&ready, &pulling}
	base := time.Now()
	for i := range maxDiagnosticEvents + 2 {
		objs = append(objs, &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "ev" + string(rune('a'+i)), Namespace: ns},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web"},
			Type:           corev1.EventTypeWarning,
			Reason:         "Failed",
			Message:        "attempt " + string(rune('a'+i)),
			LastTimestamp:  metav1.NewTime(base.Add(time.Duration(i) * time.Second)),
		})
	}

	lines := namespaceDiagnostics(fake.NewClientset(objs...), ns)
	got := strings.Join(lines, "\n")
	if !strings.Contains(got, "pod web not ready (Pending): app ImagePullBackOff: pull access denied") {
		t.Errorf("want the unready pod with its waiting reason, got:\n%s", got)
	}
	if strings.Contains(got, "pod ok") {
		t.Errorf("a Ready pod is not a diagnostic, got:\n%s", got)
	}
	if strings.Contains(got, "attempt a") || !strings.Contains(got, "attempt "+string(rune('a'+maxDiagnosticEvents+1))) {
		t.Errorf("want only the %d most recent warnings, got:\n%s", maxDiagnosticEvents, got)
	}
}

func TestNamespaceDiagnostics_NothingToReport(t *testing.T) {
	if lines := namespaceDiagnostics(fake.NewClientset(), "empty"); lines != nil {
		t.Errorf("want no lines for a healthy namespace, got %v", lines)
	}
}
