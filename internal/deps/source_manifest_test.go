package deps

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

var configMapGVK = schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}

func configMapObj(namespace string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(configMapGVK)
	obj.SetName("cm")
	obj.SetNamespace(namespace)
	obj.SetLabels(labels)
	return obj
}

func fakeCluster(existing ...runtime.Object) (*dynamicfake.FakeDynamicClient, meta.RESTMapper) {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(configMapGVK, meta.RESTScopeNamespace)
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{{Version: "v1", Resource: "configmaps"}: "ConfigMapList"}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, existing...), mapper
}

var configMaps = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

func TestApplyDoc_RefusesAnObjectVigieDidNotCreate(t *testing.T) {
	client, mapper := fakeCluster(configMapObj("prod", map[string]string{"app": "real"}))
	desired := configMapObj("prod", nil)
	labelResource(desired, "dep")

	err := applyDoc(context.Background(), client, mapper, desired, "")
	if err == nil || !strings.Contains(err.Error(), "not created by vigie") {
		t.Fatalf("want a refusal naming the cause, got %v", err)
	}
	live, _ := client.Resource(configMaps).Namespace("prod").Get(context.Background(), "cm", metav1.GetOptions{})
	if managedByVigie(live.GetLabels()) || live.GetLabels()["app"] != "real" {
		t.Errorf("the existing object must be left untouched, got labels %v", live.GetLabels())
	}
}

func TestApplyDoc_UpdatesAnObjectVigieCreated(t *testing.T) {
	managed := configMapObj("ns", map[string]string{managedByLabel: managedByValue})
	client, mapper := fakeCluster(managed)
	desired := configMapObj("ns", nil)
	labelResource(desired, "dep")
	if err := applyDoc(context.Background(), client, mapper, desired, ""); err != nil {
		t.Fatalf("re-applying vigie's own object: %v", err)
	}
}

func TestDeleteDoc_LeavesAnObjectVigieDidNotCreate(t *testing.T) {
	client, mapper := fakeCluster(configMapObj("prod", nil))
	if err := deleteDoc(context.Background(), client, mapper, configMapObj("prod", nil), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(configMaps).Namespace("prod").Get(context.Background(), "cm", metav1.GetOptions{}); err != nil {
		t.Fatalf("teardown deleted an object vigie did not create: %v", err)
	}
}

func TestResolveGVR_FallsBackToTheDependencyNamespace(t *testing.T) {
	_, mapper := fakeCluster()
	cases := []struct{ objNS, depNS, want string }{
		{"own", "dep", "own"},
		{"", "vg-test-1234", "vg-test-1234"},
		{"", "", "default"},
	}
	for _, tc := range cases {
		_, ns, err := resolveGVR(mapper, configMapObj(tc.objNS, nil), tc.depNS)
		if err != nil || ns != tc.want {
			t.Errorf("resolveGVR(obj ns %q, dep ns %q) = %q, %v; want %q", tc.objNS, tc.depNS, ns, err, tc.want)
		}
	}
}
