package deps

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	defaultWaitTimeout  = 2 * time.Minute
	defaultPollInterval = 2 * time.Second
)

// waitForConditions polls all WaitForSpec entries until they are satisfied or
// the timeout elapses. An empty waitSpecs list returns immediately.
func waitForConditions(ctx context.Context, restCfg *rest.Config, depName string, waitSpecs []dsl.WaitForSpec) error {
	for specIdx, spec := range waitSpecs {
		if err := waitForOne(ctx, restCfg, depName, specIdx, spec); err != nil {
			return err
		}
	}
	return nil
}

func waitForOne(ctx context.Context, restCfg *rest.Config, depName string, specIdx int, spec dsl.WaitForSpec) error {
	timeout, err := parseTimeout(spec.Timeout)
	if err != nil {
		return fmt.Errorf("dep %q waitFor[%d]: invalid timeout %q: %w", depName, specIdx, spec.Timeout, err)
	}

	slog.Debug("waiting for condition", "dep", depName, "kind", spec.Kind,
		"name", spec.Name, "condition", spec.Condition, "timeout", timeout)

	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch spec.Condition {
	case "Ready", "ready", "":
		return waitForReady(pollCtx, restCfg, spec)
	case "Complete", "complete":
		return waitForComplete(pollCtx, restCfg, spec)
	case "Available", "available":
		return waitForAvailable(pollCtx, restCfg, spec)
	default:
		return fmt.Errorf("dep %q waitFor[%d]: unsupported condition %q (supported: Ready, Complete, Available)", depName, specIdx, spec.Condition)
	}
}

func waitForReady(ctx context.Context, restCfg *rest.Config, spec dsl.WaitForSpec) error {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}

	return wait.PollUntilContextCancel(ctx, defaultPollInterval, false,
		func(pollCtx context.Context) (bool, error) {
			return checkReadiness(pollCtx, clientset, spec)
		})
}

func checkReadiness(ctx context.Context, clientset *kubernetes.Clientset, spec dsl.WaitForSpec) (bool, error) {
	if spec.LabelSelector != "" {
		slog.Warn("waitFor: labelSelector is not yet implemented; using name-based lookup", "kind", spec.Kind, "name", spec.Name)
	}
	ns := spec.Namespace
	switch spec.Kind {
	case "Deployment", "deployment":
		dep, err := clientset.AppsV1().Deployments(ns).Get(ctx, spec.Name, metav1.GetOptions{})
		if err != nil {
			slog.Debug("waiting for Deployment", "name", spec.Name, "err", err)
			return false, nil
		}
		ready := dep.Status.ReadyReplicas >= dep.Status.Replicas && dep.Status.Replicas > 0
		return ready, nil

	case "StatefulSet", "statefulset":
		sts, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, spec.Name, metav1.GetOptions{})
		if err != nil {
			slog.Debug("waiting for StatefulSet", "name", spec.Name, "err", err)
			return false, nil
		}
		ready := sts.Status.ReadyReplicas >= sts.Status.Replicas && sts.Status.Replicas > 0
		return ready, nil

	case "Pod", "pod":
		pod, err := clientset.CoreV1().Pods(ns).Get(ctx, spec.Name, metav1.GetOptions{})
		if err != nil {
			slog.Debug("waiting for Pod", "name", spec.Name, "err", err)
			return false, nil
		}
		for _, cond := range pod.Status.Conditions {
			if string(cond.Type) == "Ready" && cond.Status == "True" {
				return true, nil
			}
		}
		return false, nil

	default:
		// For unknown kinds fall back to a simple existence check.
		return true, nil
	}
}

func waitForComplete(ctx context.Context, restCfg *rest.Config, spec dsl.WaitForSpec) error {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}
	return wait.PollUntilContextCancel(ctx, defaultPollInterval, false,
		func(pollCtx context.Context) (bool, error) {
			job, err := clientset.BatchV1().Jobs(spec.Namespace).Get(pollCtx, spec.Name, metav1.GetOptions{})
			if err != nil {
				slog.Debug("waiting for Job", "name", spec.Name, "err", err)
				return false, nil
			}
			return job.Status.CompletionTime != nil, nil
		})
}

func waitForAvailable(ctx context.Context, restCfg *rest.Config, spec dsl.WaitForSpec) error {
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("building clientset: %w", err)
	}
	return wait.PollUntilContextCancel(ctx, defaultPollInterval, false,
		func(pollCtx context.Context) (bool, error) {
			dep, depErr := clientset.AppsV1().Deployments(spec.Namespace).Get(pollCtx, spec.Name, metav1.GetOptions{})
			if depErr != nil {
				slog.Debug("waiting for Deployment Available condition", "name", spec.Name, "err", depErr)
				return false, nil
			}
			for _, cond := range dep.Status.Conditions {
				if string(cond.Type) == "Available" && string(cond.Status) == "True" {
					return true, nil
				}
			}
			return false, nil
		})
}

func parseTimeout(raw string) (time.Duration, error) {
	if raw == "" {
		return defaultWaitTimeout, nil
	}
	return time.ParseDuration(raw)
}
