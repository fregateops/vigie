package deps

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/fregateops/vigie/internal/dsl"
)

const (
	cacheNamespace = "vigie-system"
	cacheConfigMap = "vigie.io/dep-cache"
	managedByLabel = "vigie.io/managed-by"
	managedByValue = "deps"
	cacheNameKey   = "dep-name"
)

// InstallRecord is the serialised form of a single cached install stored as
// JSON in the ConfigMap data field, keyed by the dep name.
type InstallRecord struct {
	Name  string `json:"name"`
	Hash  string `json:"hash"`
	Scope string `json:"scope"`
}

// CacheClient wraps the Kubernetes clientset to read/write InstallRecords.
type CacheClient struct {
	clientset *kubernetes.Clientset
}

// NewCacheClient builds a CacheClient from a REST config. Returns an error if
// the client cannot be constructed.
func NewCacheClient(restCfg *rest.Config) (*CacheClient, error) {
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("cache: building clientset: %w", err)
	}
	return &CacheClient{clientset: cs}, nil
}

// HashDep computes a deterministic SHA-256 of the dep's source, values, and
// namespace fields. The hash is stable across runs as long as the fields don't
// change.
func HashDep(dep dsl.Dependency) (string, error) {
	type hashInput struct {
		Source    dsl.DependencySource `json:"source"`
		Values    map[string]any       `json:"values"`
		Namespace string               `json:"namespace"`
	}
	payload := hashInput{
		Source:    dep.Source,
		Values:    dep.Values,
		Namespace: dep.Namespace,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("cache: marshalling dep hash input: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

// IsCached returns (true, nil) when the dep's hash matches the stored record,
// meaning the dep is already installed and healthy. Returns (false, nil) when
// not cached or the hash differs. Returns (false, err) on unexpected errors.
func (c *CacheClient) IsCached(ctx context.Context, dep dsl.Dependency) (bool, error) {
	hash, err := HashDep(dep)
	if err != nil {
		return false, err
	}

	record, err := c.loadRecord(ctx, dep.Name)
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, nil
	}
	if record.Hash != hash {
		slog.Debug("dep cache miss: hash changed", "dep", dep.Name)
		return false, nil
	}
	slog.Debug("dep cache hit: skipping reinstall", "dep", dep.Name)
	return true, nil
}

// StoreRecord persists the install record for the given dep.
func (c *CacheClient) StoreRecord(ctx context.Context, dep dsl.Dependency) error {
	hash, err := HashDep(dep)
	if err != nil {
		return err
	}
	if err := c.ensureNamespace(ctx); err != nil {
		return err
	}
	record := InstallRecord{Name: dep.Name, Hash: hash, Scope: dep.Scope}
	serialised, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("cache: serialising record: %w", err)
	}

	return c.upsertConfigMap(ctx, dep.Name, string(serialised))
}

// DeleteRecord removes the cached record for dep, if present.
func (c *CacheClient) DeleteRecord(ctx context.Context, depName string) error {
	cmName := configMapName(depName)
	err := c.clientset.CoreV1().ConfigMaps(cacheNamespace).Delete(ctx, cmName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("cache: deleting record for %q: %w", depName, err)
	}
	return nil
}

func (c *CacheClient) loadRecord(ctx context.Context, depName string) (*InstallRecord, error) {
	cmName := configMapName(depName)
	cm, err := c.clientset.CoreV1().ConfigMaps(cacheNamespace).Get(ctx, cmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache: loading record for %q: %w", depName, err)
	}

	raw, ok := cm.Data["record"]
	if !ok {
		return nil, nil
	}
	var record InstallRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, fmt.Errorf("cache: parsing record for %q: %w", depName, err)
	}
	return &record, nil
}

func (c *CacheClient) upsertConfigMap(ctx context.Context, depName, serialised string) error {
	cmName := configMapName(depName)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cacheNamespace,
			Labels: map[string]string{
				managedByLabel: managedByValue,
				cacheNameKey:   depName,
			},
			Annotations: map[string]string{
				cacheConfigMap: "true",
			},
		},
		Data: map[string]string{"record": serialised},
	}

	existing, err := c.clientset.CoreV1().ConfigMaps(cacheNamespace).Get(ctx, cmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, createErr := c.clientset.CoreV1().ConfigMaps(cacheNamespace).Create(ctx, cm, metav1.CreateOptions{})
		if apierrors.IsAlreadyExists(createErr) {
			// Lost the race; fall through to update below.
			existing, err = c.clientset.CoreV1().ConfigMaps(cacheNamespace).Get(ctx, cmName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("cache: re-fetching configmap %q after create race: %w", cmName, err)
			}
		} else {
			return createErr
		}
	}
	if err != nil {
		return fmt.Errorf("cache: checking existing configmap %q: %w", cmName, err)
	}

	existing.Data = cm.Data
	existing.Labels = cm.Labels
	existing.Annotations = cm.Annotations
	_, err = c.clientset.CoreV1().ConfigMaps(cacheNamespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func (c *CacheClient) ensureNamespace(ctx context.Context) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: cacheNamespace}}
	_, err := c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("cache: ensuring namespace %q: %w", cacheNamespace, err)
	}
	return nil
}

func configMapName(depName string) string {
	// Hash the dep name to produce a valid DNS-label ConfigMap name.
	sum := sha256.Sum256([]byte(depName))
	return fmt.Sprintf("vigie-dep-%x", sum[:8])
}
