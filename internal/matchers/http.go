package matchers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	celpkg "github.com/fregateops/vigie/internal/cel"
	"github.com/fregateops/vigie/internal/dsl"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func init() {
	Register(simpleMatcher{
		name:     "http",
		matches:  func(a dsl.Assertion) bool { return a.HTTP != nil },
		tiers:    Tiers(TierE2E),
		evaluate: func(a dsl.Assertion, ctx EvalContext) Result { return evalHTTP(a.HTTP, ctx) },
	})
}

const errHTTPTierMismatch = "http matcher requires a real-cluster backend (run test with --cluster kind|k3d|kubeconfig; envtest has no networking)"

func evalHTTP(spec *dsl.HTTPAssert, ctx EvalContext) Result {
	if !ctx.InApplyTier {
		return Result{Pass: false, Message: errHTTPTierMismatch}
	}
	if ctx.RESTConfig == nil {
		return Result{Pass: false, Message: "http matcher: RESTConfig is required in apply tier context"}
	}

	targetURL, cleanup, err := resolveHTTPTargetURL(spec, ctx.RESTConfig, ctx.Namespace)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("http matcher: resolve target: %v", err)}
	}
	if cleanup != nil {
		defer cleanup()
	}

	reqURL, err := url.JoinPath(targetURL, spec.Path)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("http matcher: invalid URL path: %v", err)}
	}

	timeout, err := parseHTTPTimeout(spec.Timeout)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("http matcher: %v", err)}
	}
	retryCount, retryInterval, retryUntil, err := parseHTTPRetries(spec.Retries)
	if err != nil {
		return Result{Pass: false, Message: fmt.Sprintf("http matcher: %v", err)}
	}

	var lastResult Result
	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		resp, body, reqErr := doHTTPRequest(reqURL, spec, timeout)
		if reqErr != nil {
			lastResult = Result{Pass: false, Message: fmt.Sprintf("http matcher: request error: %v", reqErr)}
			continue
		}

		if retryUntil != "" {
			done, evalErr := evalRetryUntilCEL(retryUntil, resp.StatusCode, body, resp.Header)
			if evalErr != nil {
				return Result{Pass: false, Message: fmt.Sprintf("http matcher: retries.until eval error: %v", evalErr)}
			}
			if !done {
				lastResult = Result{
					Pass:    false,
					Message: fmt.Sprintf("http matcher: retries.until %q not yet satisfied (status=%d)", retryUntil, resp.StatusCode),
				}
				continue
			}
		}

		lastResult = checkHTTPAssertions(spec.Assert, resp, body)
		if lastResult.Pass {
			return lastResult
		}
	}

	return lastResult
}

// resolveHTTPTargetURL returns the base URL to connect to and an optional cleanup
// function (for portforward teardown). fallbackNS is the per-test namespace
// used when the spec doesn't pin one - the apply-tier runner allocates a
// fresh namespace per test and passes it through EvalContext.Namespace.
func resolveHTTPTargetURL(spec *dsl.HTTPAssert, restCfg *rest.Config, fallbackNS string) (string, func(), error) {
	via := spec.Via
	if via == "" {
		via = "portforward"
	}

	switch via {
	case "portforward":
		return resolveViaPortForward(spec, restCfg, fallbackNS)
	case "direct":
		return resolveViaDirect(spec)
	case "ingress":
		return resolveViaIngress(spec, restCfg, fallbackNS)
	case "nodeport":
		return resolveViaNodePort(spec, restCfg, fallbackNS)
	default:
		return "", nil, fmt.Errorf("unknown routing mode %q (supported: portforward, direct, ingress, nodeport)", via)
	}
}

func resolveViaDirect(spec *dsl.HTTPAssert) (string, func(), error) {
	if spec.Target != nil && spec.Target.URL != "" {
		return spec.Target.URL, nil, nil
	}
	return "", nil, fmt.Errorf("direct mode requires target.url to be set")
}

// resolveViaIngress resolves the Ingress LoadBalancer IP (or hostname), then
// constructs the request URL as http(s)://<ip>:<port><path> and sets the Host
// header to the first rule host found in the Ingress spec. If the Ingress has
// no LoadBalancer address yet, it polls until the timeout elapses.
//
// For simulated backends (kwok), no real load-balancer provisions IPs, so the
// function falls back to the first Node's internal IP and emits a warning.
func resolveViaIngress(spec *dsl.HTTPAssert, restCfg *rest.Config, fallbackNS string) (string, func(), error) {
	if spec.Target == nil || spec.Target.Name == "" {
		return "", nil, fmt.Errorf("ingress mode requires target.name to be set")
	}

	namespace := resolveTargetNamespace(spec.Target.Namespace, fallbackNS)

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	timeout, err := parseHTTPTimeout(spec.Timeout)
	if err != nil {
		return "", nil, fmt.Errorf("parse timeout for ingress wait: %w", err)
	}
	pollTimeout := timeout
	if pollTimeout < 5*time.Second {
		pollTimeout = 5 * time.Second
	}

	ingress, err := waitForIngressAddress(clientset, namespace, spec.Target.Name, pollTimeout)
	if err != nil {
		return "", nil, err
	}

	address, usedFallback, err := resolveIngressAddress(clientset, ingress)
	if err != nil {
		return "", nil, err
	}
	if usedFallback {
		slog.Warn("ingress mode: no LoadBalancer IP assigned (simulated backend?); using Node IP — ingress routing requires a real ingress controller",
			"ingress", namespace+"/"+spec.Target.Name)
	}

	port := spec.Target.Port
	scheme := selectIngressScheme(ingress, port)
	targetURL := buildIngressURL(scheme, address, port)

	if spec.Host == "" {
		spec.Host = pickIngressRuleHost(ingress)
	}

	return targetURL, nil, nil
}

// waitForIngressAddress polls until the named Ingress has a LoadBalancer
// address or the timeout elapses.
func waitForIngressAddress(clientset *kubernetes.Clientset, namespace, name string, timeout time.Duration) (*networkingv1.Ingress, error) {
	pollCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var ingress *networkingv1.Ingress
	err := wait.PollUntilContextCancel(pollCtx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		ing, getErr := clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return false, fmt.Errorf("get ingress %s/%s: %w", namespace, name, getErr)
		}
		ingress = ing
		return len(ing.Status.LoadBalancer.Ingress) > 0, nil
	})
	if err != nil {
		if ingress != nil {
			return nil, fmt.Errorf("ingress %s/%s has no LoadBalancer address after %s (status: %+v)",
				namespace, name, timeout, ingress.Status.LoadBalancer)
		}
		return nil, fmt.Errorf("wait for ingress %s/%s address: %w", namespace, name, err)
	}
	return ingress, nil
}

// resolveIngressAddress extracts the IP or hostname from the Ingress status.
// When neither is set but the cluster has Nodes (simulated backend), it falls
// back to the first Node's InternalIP and returns usedFallback=true.
func resolveIngressAddress(clientset *kubernetes.Clientset, ingress *networkingv1.Ingress) (address string, usedFallback bool, err error) {
	if len(ingress.Status.LoadBalancer.Ingress) > 0 {
		lb := ingress.Status.LoadBalancer.Ingress[0]
		if lb.IP != "" {
			return lb.IP, false, nil
		}
		if lb.Hostname != "" {
			return lb.Hostname, false, nil
		}
	}

	// Simulated-backend fallback: use a Node's InternalIP.
	nodeIP, nodeErr := firstNodeInternalIP(clientset)
	if nodeErr != nil {
		return "", false, fmt.Errorf("no ingress IP/hostname and could not find node IP: %w", nodeErr)
	}
	return nodeIP, true, nil
}

// firstNodeInternalIP returns the InternalIP of the first Ready node found.
func firstNodeInternalIP(clientset *kubernetes.Clientset) (string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list nodes: %w", err)
	}
	for nodeIdx := range nodes.Items {
		node := &nodes.Items[nodeIdx]
		for addrIdx := range node.Status.Addresses {
			addr := &node.Status.Addresses[addrIdx]
			if addr.Type == corev1.NodeInternalIP && addr.Address != "" {
				return addr.Address, nil
			}
		}
	}
	return "", fmt.Errorf("no node with InternalIP found")
}

// selectIngressScheme returns "https" when the Ingress has a TLS entry that
// covers the first rule host, otherwise "http". An explicit port of 443
// also selects https.
func selectIngressScheme(ingress *networkingv1.Ingress, port int) string {
	if port == 443 {
		return "https"
	}
	if len(ingress.Spec.TLS) > 0 {
		return "https"
	}
	return "http"
}

// buildIngressURL assembles scheme://host[:port] omitting the port when it is
// the default for the scheme (80 for http, 443 for https).
func buildIngressURL(scheme, address string, port int) string {
	defaultPort := map[string]int{"http": 80, "https": 443}
	if port == 0 || port == defaultPort[scheme] {
		return fmt.Sprintf("%s://%s", scheme, address)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, address, port)
}

// pickIngressRuleHost returns the host from the first non-empty Ingress rule,
// or an empty string when the Ingress has no rules or the rule has no host.
func pickIngressRuleHost(ingress *networkingv1.Ingress) string {
	for ruleIdx := range ingress.Spec.Rules {
		if ingress.Spec.Rules[ruleIdx].Host != "" {
			return ingress.Spec.Rules[ruleIdx].Host
		}
	}
	return ""
}

// resolveTargetNamespace returns target namespace, falling back to fallbackNS, then "default".
func resolveTargetNamespace(targetNS, fallbackNS string) string {
	if targetNS != "" {
		return targetNS
	}
	if fallbackNS != "" {
		return fallbackNS
	}
	return "default"
}

func resolveViaNodePort(spec *dsl.HTTPAssert, restCfg *rest.Config, fallbackNS string) (string, func(), error) {
	if spec.Target == nil {
		return "", nil, fmt.Errorf("nodeport mode requires target to be set")
	}
	if spec.Target.Name == "" {
		return "", nil, fmt.Errorf("nodeport mode requires target.name to be set")
	}

	namespace := spec.Target.Namespace
	if namespace == "" {
		namespace = fallbackNS
	}
	if namespace == "" {
		namespace = "default"
	}

	resolveCtx, resolveCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer resolveCancel()

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	nodePort, err := resolveServiceNodePort(resolveCtx, clientset, namespace, spec.Target.Name, spec.Target.Port)
	if err != nil {
		return "", nil, fmt.Errorf("resolve nodePort for service %s/%s: %w", namespace, spec.Target.Name, err)
	}

	nodeIP, err := resolveNodeIP(resolveCtx, clientset)
	if err != nil {
		return "", nil, fmt.Errorf("resolve node IP: %w", err)
	}

	return fmt.Sprintf("http://%s:%d", nodeIP, nodePort), nil, nil
}

// resolveServiceNodePort returns the nodePort assigned to the named service
// for the given port number (or the first port if port is 0). It returns a
// clear error if the service is not of type NodePort.
func resolveServiceNodePort(ctx context.Context, clientset *kubernetes.Clientset, namespace, serviceName string, targetPort int) (int32, error) {
	svc, err := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("get service %s/%s: %w", namespace, serviceName, err)
	}

	if svc.Spec.Type != corev1.ServiceTypeNodePort {
		return 0, fmt.Errorf("service %s/%s has type %q, not NodePort — via: nodeport requires a NodePort-typed Service", namespace, serviceName, svc.Spec.Type)
	}

	if len(svc.Spec.Ports) == 0 {
		return 0, fmt.Errorf("service %s/%s has no ports", namespace, serviceName)
	}

	return pickNodePort(svc.Spec.Ports, targetPort, namespace, serviceName)
}

// pickNodePort selects the nodePort from the service ports slice. If
// targetPort is non-zero, it must match either Port or TargetPort.Port;
// otherwise the first port is used.
func pickNodePort(ports []corev1.ServicePort, targetPort int, namespace, serviceName string) (int32, error) {
	if targetPort == 0 {
		nodePort := ports[0].NodePort
		if nodePort == 0 {
			return 0, fmt.Errorf("service %s/%s first port has no nodePort assigned", namespace, serviceName)
		}
		return nodePort, nil
	}

	for _, port := range ports {
		if int(port.Port) == targetPort || port.TargetPort.IntValue() == targetPort {
			if port.NodePort == 0 {
				return 0, fmt.Errorf("service %s/%s port %d has no nodePort assigned", namespace, serviceName, targetPort)
			}
			return port.NodePort, nil
		}
	}
	return 0, fmt.Errorf("service %s/%s has no port matching %d", namespace, serviceName, targetPort)
}

// resolveNodeIP picks an InternalIP from any Ready node in the cluster,
// falling back to ExternalIP if no InternalIP is available.
func resolveNodeIP(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Prefer a Ready node; fall back to first available node.
	for idx := range nodes.Items {
		if isNodeReady(&nodes.Items[idx]) {
			if ip := pickNodeAddressIP(nodes.Items[idx].Status.Addresses); ip != "" {
				return ip, nil
			}
		}
	}
	// No Ready node with an IP - try any node.
	for idx := range nodes.Items {
		if ip := pickNodeAddressIP(nodes.Items[idx].Status.Addresses); ip != "" {
			return ip, nil
		}
	}
	return "", fmt.Errorf("no node with an addressable IP found")
}

// pickNodeAddressIP returns the first InternalIP address from the list,
// falling back to the first ExternalIP.
func pickNodeAddressIP(addresses []corev1.NodeAddress) string {
	var externalIP string
	for _, addr := range addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			return addr.Address
		case corev1.NodeExternalIP:
			if externalIP == "" {
				externalIP = addr.Address
			}
		}
	}
	return externalIP
}

// isNodeReady reports whether the Node has Ready=True in its conditions.
func isNodeReady(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func resolveViaPortForward(spec *dsl.HTTPAssert, restCfg *rest.Config, fallbackNS string) (string, func(), error) {
	if spec.Target == nil {
		return "", nil, fmt.Errorf("portforward mode requires target to be set")
	}
	if spec.Target.Name == "" {
		return "", nil, fmt.Errorf("portforward mode requires target.name to be set")
	}
	if spec.Target.Port == 0 {
		return "", nil, fmt.Errorf("portforward mode requires target.port to be set")
	}

	resolveCtx, resolveCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer resolveCancel()

	podName, namespace, err := resolvePodForTarget(resolveCtx, spec.Target, restCfg, fallbackNS)
	if err != nil {
		return "", nil, fmt.Errorf("resolve pod for portforward: %w", err)
	}

	localPort, err := allocateFreePort()
	if err != nil {
		return "", nil, fmt.Errorf("allocate local port: %w", err)
	}

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	errChan := make(chan error, 1)

	forwarder, err := buildPortForwarder(restCfg, namespace, podName, localPort, spec.Target.Port, stopChan, readyChan)
	if err != nil {
		close(stopChan)
		return "", nil, fmt.Errorf("build portforwarder: %w", err)
	}

	go func() {
		errChan <- forwarder.ForwardPorts()
	}()

	select {
	case <-readyChan:
	case fwdErr := <-errChan:
		close(stopChan)
		return "", nil, fmt.Errorf("portforward failed: %w", fwdErr)
	case <-time.After(30 * time.Second):
		close(stopChan)
		return "", nil, fmt.Errorf("portforward timed out waiting for ready")
	}

	cleanup := func() {
		close(stopChan)
		// drain any error from the goroutine
		<-errChan
	}

	return fmt.Sprintf("http://localhost:%d", localPort), cleanup, nil
}

func resolvePodForTarget(ctx context.Context, target *dsl.HTTPTarget, restCfg *rest.Config, fallbackNS string) (podName, namespace string, err error) {
	clientset, createErr := kubernetes.NewForConfig(restCfg)
	if createErr != nil {
		return "", "", fmt.Errorf("create kubernetes client: %w", createErr)
	}

	namespace = resolveTargetNamespace(target.Namespace, fallbackNS)

	kind := strings.ToLower(target.Kind)
	switch kind {
	case "pod", "":
		return target.Name, namespace, nil
	case "service", "svc":
		return resolvePodViaService(ctx, clientset, namespace, target.Name)
	default:
		return "", "", fmt.Errorf("unsupported target kind %q for portforward (use Service or Pod)", target.Kind)
	}
}

func resolvePodViaService(ctx context.Context, clientset *kubernetes.Clientset, namespace, serviceName string) (podName, ns string, err error) {
	svc, svcErr := clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if svcErr != nil {
		return "", "", fmt.Errorf("get service %s/%s: %w", namespace, serviceName, svcErr)
	}

	if len(svc.Spec.Selector) == 0 {
		return "", "", fmt.Errorf("service %s/%s has no selector", namespace, serviceName)
	}

	selector := buildLabelSelector(svc.Spec.Selector)
	pods, listErr := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if listErr != nil {
		return "", "", fmt.Errorf("list pods for service %s/%s: %w", namespace, serviceName, listErr)
	}

	for idx := range pods.Items {
		pod := &pods.Items[idx]
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, namespace, nil
		}
	}

	return "", "", fmt.Errorf("no running pod found for service %s/%s", namespace, serviceName)
}

func buildLabelSelector(labels map[string]string) string {
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func buildPortForwarder(restCfg *rest.Config, namespace, podName string, localPort, remotePort int, stopChan, readyChan chan struct{}) (*portforward.PortForwarder, error) {
	roundTripper, upgrader, err := spdy.RoundTripperFor(restCfg)
	if err != nil {
		return nil, fmt.Errorf("create round tripper: %w", err)
	}

	serverURL, urlErr := buildPortForwardURL(restCfg, namespace, podName)
	if urlErr != nil {
		return nil, urlErr
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, serverURL)
	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}

	return portforward.New(dialer, ports, stopChan, readyChan, io.Discard, io.Discard)
}

func buildPortForwardURL(restCfg *rest.Config, namespace, podName string) (*url.URL, error) {
	host := restCfg.Host
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	baseURL, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse server URL %q: %w", host, err)
	}
	baseURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)
	return baseURL, nil
}

func allocateFreePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, fmt.Errorf("listen on random port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if closeErr := listener.Close(); closeErr != nil {
		return 0, fmt.Errorf("close listener: %w", closeErr)
	}
	return port, nil
}

func parseHTTPTimeout(timeoutStr string) (time.Duration, error) {
	if timeoutStr == "" {
		return 30 * time.Second, nil
	}
	dur, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", timeoutStr, err)
	}
	return dur, nil
}

func parseHTTPRetries(retries *dsl.HTTPRetries) (count int, interval time.Duration, until string, err error) {
	if retries == nil {
		return 0, 0, "", nil
	}
	count = retries.Count
	interval = time.Second
	if retries.Interval != "" {
		dur, parseErr := time.ParseDuration(retries.Interval)
		if parseErr != nil {
			return 0, 0, "", fmt.Errorf("invalid retries.interval %q: %w", retries.Interval, parseErr)
		}
		interval = dur
	}
	until = retries.Until
	return count, interval, until, nil
}

func doHTTPRequest(targetURL string, spec *dsl.HTTPAssert, timeout time.Duration) (*http.Response, string, error) {
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if spec.Body != nil {
		bodyBytes, err := json.Marshal(spec.Body)
		if err != nil {
			return nil, "", fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, targetURL, bodyReader)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	for key, val := range spec.Headers {
		req.Header.Set(key, val)
	}
	if spec.Host != "" {
		req.Host = spec.Host
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response body: %w", err)
	}

	return resp, string(bodyBytes), nil
}

func evalRetryUntilCEL(expr string, statusCode int, body string, headers http.Header) (bool, error) {
	flatHeaders := make(map[string]any, len(headers))
	for key, vals := range headers {
		if len(vals) > 0 {
			flatHeaders[key] = vals[0]
		}
	}

	bindings := map[string]any{
		"status":  int64(statusCode),
		"body":    body,
		"headers": flatHeaders,
	}

	result, err := celpkg.Eval(expr, bindings)
	if err != nil {
		return false, err
	}

	done, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("retries.until expression %q must return bool (got %T)", expr, result)
	}
	return done, nil
}

func checkHTTPAssertions(assertBlock *dsl.HTTPAssertBlock, resp *http.Response, body string) Result {
	if assertBlock == nil {
		return Result{Pass: true}
	}

	if assertBlock.Status != 0 && resp.StatusCode != assertBlock.Status {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("http matcher: status %d does not match expected %d (body: %s)", resp.StatusCode, assertBlock.Status, truncate(body, 200)),
		}
	}

	if msg := checkHTTPHeaders(assertBlock.Headers, resp.Header); msg != "" {
		return Result{Pass: false, Message: msg}
	}

	if assertBlock.BodyContains != "" && !strings.Contains(body, assertBlock.BodyContains) {
		return Result{
			Pass:    false,
			Message: fmt.Sprintf("http matcher: body does not contain %q (body: %s)", assertBlock.BodyContains, truncate(body, 200)),
		}
	}

	if len(assertBlock.JSONPath) > 0 {
		if msg := checkJSONPath(assertBlock.JSONPath, body); msg != "" {
			return Result{Pass: false, Message: msg}
		}
	}

	return Result{Pass: true}
}

// checkHTTPHeaders validates response headers against expected values.
// Values starting with "~/" are treated as regex patterns.
func checkHTTPHeaders(expected map[string]string, actual http.Header) string {
	for key, expectedVal := range expected {
		actualVal := actual.Get(key)
		if strings.HasPrefix(expectedVal, "~/") && strings.HasSuffix(expectedVal, "/") {
			pattern := expectedVal[2 : len(expectedVal)-1]
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Sprintf("http matcher: header %q: invalid regex pattern %q: %v", key, pattern, err)
			}
			if !re.MatchString(actualVal) {
				return fmt.Sprintf("http matcher: header %q value %q does not match pattern %q", key, actualVal, pattern)
			}
		} else {
			if actualVal != expectedVal {
				return fmt.Sprintf("http matcher: header %q value %q does not match expected %q", key, actualVal, expectedVal)
			}
		}
	}
	return ""
}

// checkJSONPath evaluates JSONPath expressions against the response body.
// Expressions use $.foo.bar syntax; values are compared as strings.
func checkJSONPath(paths map[string]any, body string) string {
	var data any
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return fmt.Sprintf("http matcher: jsonPath: response body is not valid JSON: %v", err)
	}

	for expr, expectedVal := range paths {
		actualVal, found, err := evalJSONPath(expr, data)
		if err != nil {
			return fmt.Sprintf("http matcher: jsonPath %q: evaluation error: %v", expr, err)
		}
		if !found {
			return fmt.Sprintf("http matcher: jsonPath %q: path not found", expr)
		}

		actualStr := fmt.Sprintf("%v", actualVal)
		expectedStr := fmt.Sprintf("%v", expectedVal)
		if actualStr != expectedStr {
			return fmt.Sprintf("http matcher: jsonPath %q: got %q want %q", expr, actualStr, expectedStr)
		}
	}
	return ""
}

// evalJSONPath evaluates a JSONPath expression ($.foo.bar[0]) against parsed JSON data.
// Supports simple dot-notation paths and array index access.
func evalJSONPath(expr string, data any) (any, bool, error) {
	if !strings.HasPrefix(expr, "$.") && expr != "$" {
		return nil, false, fmt.Errorf("jsonPath expression must start with '$.' or '$'")
	}

	if expr == "$" {
		return data, true, nil
	}

	// Strip leading "$." and split on "." while handling bracket notation
	path := expr[2:]
	return traverseJSONPath(path, data)
}

// traverseJSONPath walks through the JSON data following a dot-separated path.
// Handles array indexing via [N] notation.
func traverseJSONPath(path string, data any) (any, bool, error) {
	if path == "" {
		return data, true, nil
	}

	// Split on first "." or "[" to get the next segment
	segment, rest := splitJSONPathSegment(path)

	switch typed := data.(type) {
	case map[string]any:
		val, ok := typed[segment]
		if !ok {
			return nil, false, nil
		}
		return traverseJSONPath(rest, val)
	case []any:
		idx, parseErr := parseArrayIndex(segment)
		if parseErr != nil {
			return nil, false, fmt.Errorf("expected array index, got %q: %w", segment, parseErr)
		}
		if idx < 0 || idx >= len(typed) {
			return nil, false, fmt.Errorf("array index %d out of bounds (len=%d)", idx, len(typed))
		}
		return traverseJSONPath(rest, typed[idx])
	default:
		if segment != "" {
			return nil, false, nil
		}
		return data, true, nil
	}
}

// splitJSONPathSegment extracts the next path segment and returns the remainder.
// Handles both dot notation (foo.bar) and bracket notation (foo[0].bar).
func splitJSONPathSegment(path string) (segment, rest string) {
	// Check for bracket notation at start
	if strings.HasPrefix(path, "[") {
		end := strings.Index(path, "]")
		if end == -1 {
			return path, ""
		}
		segment = path[1:end]
		rest = strings.TrimPrefix(path[end+1:], ".")
		return segment, rest
	}

	// Dot notation: split on first "." or "["
	dotIdx := strings.Index(path, ".")
	bracketIdx := strings.Index(path, "[")

	switch {
	case dotIdx == -1 && bracketIdx == -1:
		return path, ""
	case dotIdx == -1:
		return path[:bracketIdx], path[bracketIdx:]
	case bracketIdx == -1:
		return path[:dotIdx], path[dotIdx+1:]
	case dotIdx < bracketIdx:
		return path[:dotIdx], path[dotIdx+1:]
	default:
		return path[:bracketIdx], path[bracketIdx:]
	}
}

func parseArrayIndex(s string) (int, error) {
	var idx int
	_, err := fmt.Sscanf(s, "%d", &idx)
	return idx, err
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
