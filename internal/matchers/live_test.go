package matchers

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestKindToGVR verifies the scheme-derived resolver maps common built-in kinds
// to their preferred (stable) GroupVersionResource - matching the GVRs the
// previous hardcoded table produced, including the multi-version kinds whose
// resolution depends on the stable-version preference (Deployment, Ingress,
// HorizontalPodAutoscaler).
func TestKindToGVR(t *testing.T) {
	want := map[string]schema.GroupVersionResource{
		"Pod":                     {Group: "", Version: "v1", Resource: "pods"},
		"Service":                 {Group: "", Version: "v1", Resource: "services"},
		"ConfigMap":               {Group: "", Version: "v1", Resource: "configmaps"},
		"Secret":                  {Group: "", Version: "v1", Resource: "secrets"},
		"ServiceAccount":          {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"Namespace":               {Group: "", Version: "v1", Resource: "namespaces"},
		"PersistentVolumeClaim":   {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"Deployment":              {Group: "apps", Version: "v1", Resource: "deployments"},
		"StatefulSet":             {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"DaemonSet":               {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"ReplicaSet":              {Group: "apps", Version: "v1", Resource: "replicasets"},
		"Job":                     {Group: "batch", Version: "v1", Resource: "jobs"},
		"CronJob":                 {Group: "batch", Version: "v1", Resource: "cronjobs"},
		"Ingress":                 {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"NetworkPolicy":           {Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"},
		"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"},
		"Role":                    {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		"RoleBinding":             {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		"ClusterRole":             {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		"ClusterRoleBinding":      {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
	}

	for kind, expected := range want {
		got, err := kindToGVR(kind)
		if err != nil {
			t.Errorf("kindToGVR(%q) returned error: %v", kind, err)
			continue
		}
		if got != expected {
			t.Errorf("kindToGVR(%q) = %v, want %v", kind, got, expected)
		}
	}
}

func TestKindToGVR_Unknown(t *testing.T) {
	if _, err := kindToGVR("UnknownCustomResource"); err == nil {
		t.Fatal("expected error for a kind that is not a built-in")
	}
}
