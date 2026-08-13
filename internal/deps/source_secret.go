package deps

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	defaultGenerateShell   = "sh"
	defaultGenerateTimeout = 30 * time.Second
	secretManagedBy        = "secrets"
)

// applySecret resolves every key in the secret dep and creates / patches one
// K8s Secret in dep.Namespace. The dep.Name doubles as the Secret name.
//
// Secret values resolved via env / file / generate are NEVER logged; only the
// dep name and which source was used (or the empty result from each source on
// failure) appears in logs.
func applySecret(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config, baseDir string) error {
	src := dep.Source.Secret
	if src == nil || len(src.Keys) == 0 {
		return fmt.Errorf("secret dep %q: no keys defined", dep.Name)
	}

	namespace := dep.Namespace
	if namespace == "" {
		return fmt.Errorf("secret dep %q: namespace is required", dep.Name)
	}

	data := make(map[string][]byte, len(src.Keys))
	for _, key := range src.Keys {
		if key.Key == "" {
			return fmt.Errorf("secret dep %q: empty key entry", dep.Name)
		}
		value, source, err := resolveSecretValue(ctx, key, baseDir)
		if err != nil {
			return fmt.Errorf("secret dep %q key %q: %w", dep.Name, key.Key, err)
		}
		slog.Debug("resolved secret key", "dep", dep.Name, "key", key.Key, "source", source)
		data[key.Key] = value
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("secret dep %q: building clientset: %w", dep.Name, err)
	}

	if err := ensureNamespace(ctx, clientset, namespace); err != nil {
		return fmt.Errorf("secret dep %q: ensuring namespace %q: %w", dep.Name, namespace, err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dep.Name,
			Namespace: namespace,
			Labels: map[string]string{
				managedByLabel: secretManagedBy,
			},
			Annotations: map[string]string{
				managedByAnnotation: dep.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	return upsertSecret(ctx, clientset, secret)
}

// teardownSecret deletes the K8s Secret created by applySecret. Cluster-scoped
// secrets live outside per-test namespaces and need explicit teardown; per-test
// and per-suite Secrets are cleaned up when their namespace is deleted, but
// calling Delete here is harmless (NotFound is ignored).
func teardownSecret(ctx context.Context, dep dsl.Dependency, restCfg *rest.Config) error {
	if dep.Namespace == "" {
		return nil
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("secret dep %q teardown: building clientset: %w", dep.Name, err)
	}
	err = clientset.CoreV1().Secrets(dep.Namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("secret dep %q teardown: %w", dep.Name, err)
	}
	return nil
}

// resolveSecretValue walks the env → file → generate → fallback chain for one
// SecretKeySpec, returning the first source that produces a value. The returned
// source string identifies which path won (for logs only - never the value).
func resolveSecretValue(ctx context.Context, key dsl.SecretKeySpec, baseDir string) ([]byte, string, error) {
	value, source, err := resolveOneSource(ctx, key, baseDir)
	if err != nil {
		return nil, "", err
	}
	if value != nil {
		return value, source, nil
	}

	if key.Fallback != nil {
		fbValue, fbSource, fbErr := resolveSecretValue(ctx, *key.Fallback, baseDir)
		if fbErr != nil {
			return nil, "", fmt.Errorf("fallback: %w", fbErr)
		}
		if fbValue != nil {
			return fbValue, "fallback:" + fbSource, nil
		}
	}

	return nil, "", errors.New("no source produced a value (tried env, file, generate" + fallbackChainSuffix(key) + ")")
}

func fallbackChainSuffix(key dsl.SecretKeySpec) string {
	if key.Fallback == nil {
		return ""
	}
	return ", fallback"
}

// resolveOneSource tries env/file/generate in order. Returns (nil, "", nil)
// when no source is configured or the configured source resolves to empty,
// signalling the caller to try the fallback.
func resolveOneSource(ctx context.Context, key dsl.SecretKeySpec, baseDir string) ([]byte, string, error) {
	switch {
	case key.Env != "":
		val := os.Getenv(key.Env)
		if val == "" {
			return nil, "", nil
		}
		return []byte(val), "env:" + key.Env, nil

	case key.File != "":
		path := key.File
		if !filepath.IsAbs(path) && baseDir != "" {
			path = filepath.Join(baseDir, path)
		}
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("reading file %q: %w", path, err)
		}
		return raw, "file:" + path, nil

	case key.Generate != nil:
		raw, err := runGenerate(ctx, *key.Generate, baseDir)
		if err != nil {
			return nil, "", err
		}
		return raw, "generate", nil
	}

	return nil, "", nil
}

// runGenerate executes the user-supplied shell command and returns stdout as
// raw bytes. The command runs with `shell -c run`; stderr is captured for
// diagnostics but never logged when the command succeeds.
func runGenerate(parent context.Context, cmd dsl.ShellCommand, baseDir string) ([]byte, error) {
	if cmd.Run == "" {
		return nil, errors.New("generate.run is required")
	}
	shell := cmd.Shell
	if shell == "" {
		shell = defaultGenerateShell
	}
	timeout := defaultGenerateTimeout
	if cmd.Timeout != "" {
		d, err := time.ParseDuration(cmd.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parsing timeout %q: %w", cmd.Timeout, err)
		}
		timeout = d
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	execCmd := exec.CommandContext(ctx, shell, "-c", cmd.Run)
	if cmd.Dir != "" {
		execCmd.Dir = cmd.Dir
	} else if baseDir != "" {
		execCmd.Dir = baseDir
	}
	if len(cmd.Env) > 0 {
		envSlice := append([]string{}, os.Environ()...)
		for k, v := range cmd.Env {
			envSlice = append(envSlice, k+"="+v)
		}
		execCmd.Env = envSlice
	}

	var stdout, stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	if err := execCmd.Run(); err != nil {
		// stderr captured separately; never include stdout in error messages.
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return nil, fmt.Errorf("generate command failed: %w (stderr: %s)", err, stderrStr)
		}
		return nil, fmt.Errorf("generate command failed: %w", err)
	}
	return stdout.Bytes(), nil
}

// upsertSecret creates or patches an Opaque Secret. We use Get-then-Update to
// preserve compatibility with K8s versions where SSA on Secret data has
// known quirks.
func upsertSecret(ctx context.Context, clientset *kubernetes.Clientset, secret *corev1.Secret) error {
	secrets := clientset.CoreV1().Secrets(secret.Namespace)
	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, getErr := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("getting existing secret: %w", getErr)
		}
		existing.Data = secret.Data
		existing.Type = secret.Type
		existing.Labels = secret.Labels
		existing.Annotations = secret.Annotations
		_, err = secrets.Update(ctx, existing, metav1.UpdateOptions{})
	}
	return err
}

// ensureNamespace creates the namespace if it doesn't already exist. Used by
// the secret source so users don't need to manually create namespaces before
// referencing them in dep.namespace.
func ensureNamespace(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
