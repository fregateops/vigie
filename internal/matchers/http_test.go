package matchers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fregateops/vigie/internal/dsl"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
)

func httpCtx(inApplyTier bool) EvalContext {
	return EvalContext{
		InApplyTier: inApplyTier,
		RESTConfig:  &rest.Config{Host: "https://localhost:6443"},
	}
}

func TestEvalHTTP_TierGuard(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via: "direct",
		Target: &dsl.HTTPTarget{
			URL: "http://example.com",
		},
	}

	result := evalHTTP(spec, EvalContext{InApplyTier: false})
	if result.Pass {
		t.Error("expected fail outside apply tier")
	}
	if result.Message != errHTTPTierMismatch {
		t.Errorf("expected tier mismatch message, got %q", result.Message)
	}
}

func TestEvalHTTP_MissingRESTConfig(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via: "direct",
		Target: &dsl.HTTPTarget{
			URL: "http://example.com",
		},
	}

	result := evalHTTP(spec, EvalContext{InApplyTier: true, RESTConfig: nil})
	if result.Pass {
		t.Error("expected fail with nil RESTConfig")
	}
}

func TestEvalHTTP_DirectMode_StatusAssert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	spec := &dsl.HTTPAssert{
		Via: "direct",
		Target: &dsl.HTTPTarget{
			URL: srv.URL,
		},
		Path:   "/",
		Method: "GET",
		Assert: &dsl.HTTPAssertBlock{
			Status: 200,
		},
	}

	result := evalHTTP(spec, httpCtx(true))
	if !result.Pass {
		t.Errorf("expected pass, got message: %s", result.Message)
	}
}

func TestEvalHTTP_DirectMode_StatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	spec := &dsl.HTTPAssert{
		Via: "direct",
		Target: &dsl.HTTPTarget{
			URL: srv.URL,
		},
		Assert: &dsl.HTTPAssertBlock{
			Status: 200,
		},
	}

	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail on status mismatch")
	}
}

func TestEvalHTTP_DirectMode_BodyContains(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world uptime 123"))
	}))
	defer srv.Close()

	tests := []struct {
		name        string
		bodyContain string
		wantPass    bool
	}{
		{"present substring", "uptime", true},
		{"absent substring", "notpresent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &dsl.HTTPAssert{
				Via:    "direct",
				Target: &dsl.HTTPTarget{URL: srv.URL},
				Assert: &dsl.HTTPAssertBlock{
					BodyContains: tt.bodyContain,
				},
			}
			result := evalHTTP(spec, httpCtx(true))
			if result.Pass != tt.wantPass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.wantPass, result.Message)
			}
		})
	}
}

func TestEvalHTTP_DirectMode_HeaderExact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		headers  map[string]string
		wantPass bool
	}{
		{"exact match", map[string]string{"Content-Type": "application/json"}, true},
		{"exact mismatch", map[string]string{"Content-Type": "text/plain"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &dsl.HTTPAssert{
				Via:    "direct",
				Target: &dsl.HTTPTarget{URL: srv.URL},
				Assert: &dsl.HTTPAssertBlock{
					Headers: tt.headers,
				},
			}
			result := evalHTTP(spec, httpCtx(true))
			if result.Pass != tt.wantPass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.wantPass, result.Message)
			}
		})
	}
}

func TestEvalHTTP_DirectMode_HeaderRegex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		headers  map[string]string
		wantPass bool
	}{
		{"regex match", map[string]string{"Content-Type": "~/^application\\/json/"}, true},
		{"regex no match", map[string]string{"Content-Type": "~/^text\\//"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &dsl.HTTPAssert{
				Via:    "direct",
				Target: &dsl.HTTPTarget{URL: srv.URL},
				Assert: &dsl.HTTPAssertBlock{
					Headers: tt.headers,
				},
			}
			result := evalHTTP(spec, httpCtx(true))
			if result.Pass != tt.wantPass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.wantPass, result.Message)
			}
		})
	}
}

func TestEvalHTTP_DirectMode_JSONPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body := map[string]any{
			"status": "ok",
			"checks": map[string]any{"db": "pass"},
			"items":  []any{"a", "b", "c"},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		jsonPath map[string]any
		wantPass bool
	}{
		{"flat key match", map[string]any{"$.status": "ok"}, true},
		{"flat key mismatch", map[string]any{"$.status": "fail"}, false},
		{"nested key match", map[string]any{"$.checks.db": "pass"}, true},
		{"array index match", map[string]any{"$.items[1]": "b"}, true},
		{"missing key", map[string]any{"$.missing": "val"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &dsl.HTTPAssert{
				Via:    "direct",
				Target: &dsl.HTTPTarget{URL: srv.URL},
				Assert: &dsl.HTTPAssertBlock{
					JSONPath: tt.jsonPath,
				},
			}
			result := evalHTTP(spec, httpCtx(true))
			if result.Pass != tt.wantPass {
				t.Errorf("pass=%v want=%v message=%q", result.Pass, tt.wantPass, result.Message)
			}
		})
	}
}

func TestEvalHTTP_DirectMode_Retries(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	spec := &dsl.HTTPAssert{
		Via:    "direct",
		Target: &dsl.HTTPTarget{URL: srv.URL},
		Retries: &dsl.HTTPRetries{
			Count:    5,
			Interval: "1ms",
		},
		Assert: &dsl.HTTPAssertBlock{
			Status: 200,
		},
	}

	result := evalHTTP(spec, httpCtx(true))
	if !result.Pass {
		t.Errorf("expected pass after retries, got: %s", result.Message)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 fail + 1 success), got %d", callCount)
	}
}

func TestEvalHTTP_DirectMode_RetryUntilCEL(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	spec := &dsl.HTTPAssert{
		Via:    "direct",
		Target: &dsl.HTTPTarget{URL: srv.URL},
		Retries: &dsl.HTTPRetries{
			Count:    5,
			Interval: "1ms",
			Until:    "status == 200",
		},
		Assert: &dsl.HTTPAssertBlock{
			Status: 200,
		},
	}

	result := evalHTTP(spec, httpCtx(true))
	if !result.Pass {
		t.Errorf("expected pass after retries with until expr, got: %s", result.Message)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestEvalHTTP_DirectMode_MissingURL(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via:    "direct",
		Target: &dsl.HTTPTarget{},
	}

	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail when direct mode has no URL")
	}
}

func TestEvalHTTP_IngressMode_MissingTargetName(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via:    "ingress",
		Target: &dsl.HTTPTarget{},
	}

	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail when ingress mode has no target name")
	}
	if !strings.Contains(result.Message, "target.name") {
		t.Errorf("expected 'target.name' in message, got: %q", result.Message)
	}
}

func TestEvalHTTP_IngressMode_MissingTarget(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via: "ingress",
	}

	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail when ingress mode has no target")
	}
	if !strings.Contains(result.Message, "target.name") {
		t.Errorf("expected 'target.name' in message, got: %q", result.Message)
	}
}

func TestEvalHTTP_NodePortMode_MissingTarget(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via: "nodeport",
	}
	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail when nodeport mode has no target")
	}
	if !strings.Contains(result.Message, "requires target to be set") {
		t.Errorf("expected 'requires target to be set' in message, got: %q", result.Message)
	}
}

func TestEvalHTTP_NodePortMode_MissingTargetName(t *testing.T) {
	spec := &dsl.HTTPAssert{
		Via:    "nodeport",
		Target: &dsl.HTTPTarget{},
	}
	result := evalHTTP(spec, httpCtx(true))
	if result.Pass {
		t.Error("expected fail when nodeport mode has no target.name")
	}
	if !strings.Contains(result.Message, "requires target.name to be set") {
		t.Errorf("expected 'requires target.name to be set' in message, got: %q", result.Message)
	}
}

func TestPickNodePort_FirstPort(t *testing.T) {
	ports := []corev1.ServicePort{
		{
			Port:       80,
			TargetPort: intstr.FromInt(8080),
			NodePort:   30080,
		},
	}
	nodePort, err := pickNodePort(ports, 0, "default", "my-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodePort != 30080 {
		t.Errorf("expected nodePort 30080, got %d", nodePort)
	}
}

func TestPickNodePort_MatchByServicePort(t *testing.T) {
	ports := []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), NodePort: 30080},
		{Port: 443, TargetPort: intstr.FromInt(8443), NodePort: 30443},
	}
	nodePort, err := pickNodePort(ports, 443, "default", "my-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodePort != 30443 {
		t.Errorf("expected nodePort 30443, got %d", nodePort)
	}
}

func TestPickNodePort_MatchByTargetPort(t *testing.T) {
	ports := []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), NodePort: 30080},
		{Port: 9090, TargetPort: intstr.FromInt(9000), NodePort: 30090},
	}
	nodePort, err := pickNodePort(ports, 9000, "default", "my-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodePort != 30090 {
		t.Errorf("expected nodePort 30090, got %d", nodePort)
	}
}

func TestPickNodePort_NoMatchingPort(t *testing.T) {
	ports := []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), NodePort: 30080},
	}
	_, err := pickNodePort(ports, 9999, "default", "my-svc")
	if err == nil {
		t.Fatal("expected error for non-matching port, got nil")
	}
	if !strings.Contains(err.Error(), "no port matching") {
		t.Errorf("expected 'no port matching' in error, got: %q", err)
	}
}

func TestPickNodePort_NoNodePortAssigned(t *testing.T) {
	ports := []corev1.ServicePort{
		{Port: 80, TargetPort: intstr.FromInt(8080), NodePort: 0},
	}
	_, err := pickNodePort(ports, 0, "default", "my-svc")
	if err == nil {
		t.Fatal("expected error when nodePort is 0, got nil")
	}
	if !strings.Contains(err.Error(), "no nodePort assigned") {
		t.Errorf("expected 'no nodePort assigned' in error, got: %q", err)
	}
}

func TestPickNodeAddressIP_InternalFirst(t *testing.T) {
	addresses := []corev1.NodeAddress{
		{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
		{Type: corev1.NodeInternalIP, Address: "10.0.0.5"},
	}
	ip := pickNodeAddressIP(addresses)
	if ip != "10.0.0.5" {
		t.Errorf("expected InternalIP 10.0.0.5, got %q", ip)
	}
}

func TestPickNodeAddressIP_FallbackExternal(t *testing.T) {
	addresses := []corev1.NodeAddress{
		{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
	}
	ip := pickNodeAddressIP(addresses)
	if ip != "1.2.3.4" {
		t.Errorf("expected ExternalIP 1.2.3.4, got %q", ip)
	}
}

func TestPickNodeAddressIP_NoAddress(t *testing.T) {
	addresses := []corev1.NodeAddress{
		{Type: corev1.NodeHostName, Address: "node1"},
	}
	ip := pickNodeAddressIP(addresses)
	if ip != "" {
		t.Errorf("expected empty string for no addressable IP, got %q", ip)
	}
}

func TestIsNodeReady(t *testing.T) {
	readyNode := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	}
	notReadyNode := &corev1.Node{
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	}
	noConditionsNode := &corev1.Node{}

	if !isNodeReady(readyNode) {
		t.Error("expected ready node to be ready")
	}
	if isNodeReady(notReadyNode) {
		t.Error("expected not-ready node to not be ready")
	}
	if isNodeReady(noConditionsNode) {
		t.Error("expected node with no conditions to not be ready")
	}
}

func TestCheckHTTPHeaders_Patterns(t *testing.T) {
	tests := []struct {
		name     string
		expected map[string]string
		actual   http.Header
		wantMsg  string
	}{
		{
			name:     "exact match",
			expected: map[string]string{"X-Custom": "value"},
			actual:   http.Header{"X-Custom": []string{"value"}},
			wantMsg:  "",
		},
		{
			name:     "exact mismatch",
			expected: map[string]string{"X-Custom": "expected"},
			actual:   http.Header{"X-Custom": []string{"actual"}},
			wantMsg:  "does not match expected",
		},
		{
			name:     "regex match",
			expected: map[string]string{"Content-Type": "~/^application\\//"},
			actual:   http.Header{"Content-Type": []string{"application/json"}},
			wantMsg:  "",
		},
		{
			name:     "regex mismatch",
			expected: map[string]string{"Content-Type": "~/^text\\//"},
			actual:   http.Header{"Content-Type": []string{"application/json"}},
			wantMsg:  "does not match pattern",
		},
		{
			name:     "invalid regex",
			expected: map[string]string{"X-H": "~/[invalid/"},
			actual:   http.Header{"X-H": []string{"val"}},
			wantMsg:  "invalid regex pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := checkHTTPHeaders(tt.expected, tt.actual)
			if tt.wantMsg == "" && msg != "" {
				t.Errorf("expected no error, got: %q", msg)
			}
			if tt.wantMsg != "" && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("expected message containing %q, got: %q", tt.wantMsg, msg)
			}
		})
	}
}

func TestEvalJSONPath(t *testing.T) {
	data := map[string]any{
		"status": "ok",
		"count":  float64(42),
		"nested": map[string]any{
			"key": "val",
		},
		"list": []any{"a", "b", "c"},
	}

	tests := []struct {
		expr       string
		wantValStr string
		wantOk     bool
		wantErr    bool
		skipValCmp bool
	}{
		{"$", "", true, false, true},
		{"$.status", "ok", true, false, false},
		{"$.count", "42", true, false, false},
		{"$.nested.key", "val", true, false, false},
		{"$.list[0]", "a", true, false, false},
		{"$.list[2]", "c", true, false, false},
		{"$.missing", "", false, false, false},
		{"$.nested.missing", "", false, false, false},
		{"notdollar", "", false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			val, found, err := evalJSONPath(tt.expr, data)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if found != tt.wantOk {
				t.Errorf("found=%v want=%v", found, tt.wantOk)
			}
			if tt.wantOk && !tt.skipValCmp {
				actualStr := fmt.Sprintf("%v", val)
				if actualStr != tt.wantValStr {
					t.Errorf("val=%v want=%v", actualStr, tt.wantValStr)
				}
			}
		})
	}
}

func TestSelectIngressScheme(t *testing.T) {
	tests := []struct {
		name       string
		ingress    *networkingv1.Ingress
		port       int
		wantScheme string
	}{
		{
			name:       "port 443 forces https",
			ingress:    &networkingv1.Ingress{},
			port:       443,
			wantScheme: "https",
		},
		{
			name: "TLS configured selects https",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					TLS: []networkingv1.IngressTLS{{Hosts: []string{"example.com"}}},
				},
			},
			port:       80,
			wantScheme: "https",
		},
		{
			name:       "no TLS and non-443 port selects http",
			ingress:    &networkingv1.Ingress{},
			port:       80,
			wantScheme: "http",
		},
		{
			name:       "zero port with no TLS selects http",
			ingress:    &networkingv1.Ingress{},
			port:       0,
			wantScheme: "http",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := selectIngressScheme(tt.ingress, tt.port)
			if scheme != tt.wantScheme {
				t.Errorf("selectIngressScheme() = %q, want %q", scheme, tt.wantScheme)
			}
		})
	}
}

func TestBuildIngressURL(t *testing.T) {
	tests := []struct {
		name    string
		scheme  string
		address string
		port    int
		wantURL string
	}{
		{"http default port omitted", "http", "10.0.0.1", 80, "http://10.0.0.1"},
		{"https default port omitted", "https", "10.0.0.1", 443, "https://10.0.0.1"},
		{"http non-default port included", "http", "10.0.0.1", 8080, "http://10.0.0.1:8080"},
		{"https non-default port included", "https", "10.0.0.1", 8443, "https://10.0.0.1:8443"},
		{"zero port omitted", "http", "10.0.0.1", 0, "http://10.0.0.1"},
		{"hostname address", "http", "ingress.example.com", 80, "http://ingress.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildIngressURL(tt.scheme, tt.address, tt.port)
			if got != tt.wantURL {
				t.Errorf("buildIngressURL(%q, %q, %d) = %q, want %q", tt.scheme, tt.address, tt.port, got, tt.wantURL)
			}
		})
	}
}

func TestPickIngressRuleHost(t *testing.T) {
	tests := []struct {
		name     string
		ingress  *networkingv1.Ingress
		wantHost string
	}{
		{
			name:     "no rules returns empty",
			ingress:  &networkingv1.Ingress{},
			wantHost: "",
		},
		{
			name: "rule with empty host returns empty",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{Host: ""}},
				},
			},
			wantHost: "",
		},
		{
			name: "first non-empty rule host returned",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{Host: "app.example.com"},
						{Host: "other.example.com"},
					},
				},
			},
			wantHost: "app.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickIngressRuleHost(tt.ingress)
			if got != tt.wantHost {
				t.Errorf("pickIngressRuleHost() = %q, want %q", got, tt.wantHost)
			}
		})
	}
}

func TestResolveTargetNamespace(t *testing.T) {
	tests := []struct {
		targetNS   string
		fallbackNS string
		wantNS     string
	}{
		{"my-ns", "fallback", "my-ns"},
		{"", "fallback", "fallback"},
		{"", "", "default"},
	}

	for _, tt := range tests {
		got := resolveTargetNamespace(tt.targetNS, tt.fallbackNS)
		if got != tt.wantNS {
			t.Errorf("resolveTargetNamespace(%q, %q) = %q, want %q", tt.targetNS, tt.fallbackNS, got, tt.wantNS)
		}
	}
}

func TestParseHTTPTimeout(t *testing.T) {
	tests := []struct {
		input    string
		wantSecs float64
		wantErr  bool
	}{
		{"", 30, false},
		{"5s", 5, false},
		{"10m", 600, false},
		{"invalid", 0, true},
	}
	for _, tt := range tests {
		dur, err := parseHTTPTimeout(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseHTTPTimeout(%q): expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHTTPTimeout(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if dur.Seconds() != tt.wantSecs {
			t.Errorf("parseHTTPTimeout(%q) = %v, want %vs", tt.input, dur, tt.wantSecs)
		}
	}
}
