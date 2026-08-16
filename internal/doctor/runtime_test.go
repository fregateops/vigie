package doctor

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestDetectContainerRuntime(t *testing.T) {
	t.Run("docker found and version succeeds", func(t *testing.T) {
		stubExec(t,
			func(name string) (string, error) {
				if name == "docker" {
					return "/usr/bin/docker", nil
				}
				return "", exec.ErrNotFound
			},
			func(name string, _ ...string) ([]byte, error) {
				if name == "/usr/bin/docker" {
					return []byte("24.0.7\n"), nil
				}
				return nil, errors.New("unexpected exec")
			},
		)

		c := DetectContainerRuntime()
		if c.Name != "docker" {
			t.Errorf("Name = %q, want %q", c.Name, "docker")
		}
		if c.Status != StatusOK {
			t.Errorf("Status = %q, want %q", c.Status, StatusOK)
		}
		if !strings.Contains(c.Detail, "24.0.7") {
			t.Errorf("Detail %q missing version", c.Detail)
		}
	})

	t.Run("docker found but daemon down", func(t *testing.T) {
		stubExec(t,
			func(name string) (string, error) {
				if name == "docker" {
					return "/usr/bin/docker", nil
				}
				return "", exec.ErrNotFound
			},
			func(_ string, _ ...string) ([]byte, error) { return nil, errors.New("daemon not responding") },
		)

		c := DetectContainerRuntime()
		if c.Name != "docker" {
			t.Errorf("Name = %q, want %q", c.Name, "docker")
		}
		if c.Status != StatusWarning {
			t.Errorf("Status = %q, want %q", c.Status, StatusWarning)
		}
		if !strings.Contains(c.Detail, "daemon not responding") {
			t.Errorf("Detail %q missing daemon message", c.Detail)
		}
	})

	t.Run("docker missing, podman OK", func(t *testing.T) {
		stubExec(t,
			func(name string) (string, error) {
				if name == "podman" {
					return "/usr/bin/podman", nil
				}
				return "", exec.ErrNotFound
			},
			func(name string, _ ...string) ([]byte, error) {
				if name == "/usr/bin/podman" {
					return []byte("4.7.0\n"), nil
				}
				return nil, errors.New("unexpected exec")
			},
		)

		c := DetectContainerRuntime()
		if c.Name != "podman" {
			t.Errorf("Name = %q, want %q", c.Name, "podman")
		}
		if c.Status != StatusOK {
			t.Errorf("Status = %q, want %q", c.Status, StatusOK)
		}
		if !strings.Contains(c.Detail, "4.7.0") {
			t.Errorf("Detail %q missing version", c.Detail)
		}
	})

	t.Run("podman found but not responding", func(t *testing.T) {
		stubExec(t,
			func(name string) (string, error) {
				if name == "podman" {
					return "/usr/bin/podman", nil
				}
				return "", exec.ErrNotFound
			},
			func(_ string, _ ...string) ([]byte, error) { return nil, errors.New("podman exit 1") },
		)

		c := DetectContainerRuntime()
		if c.Name != "podman" {
			t.Errorf("Name = %q, want %q", c.Name, "podman")
		}
		if c.Status != StatusWarning {
			t.Errorf("Status = %q, want %q", c.Status, StatusWarning)
		}
	})

	t.Run("both missing", func(t *testing.T) {
		stubExec(t,
			func(_ string) (string, error) { return "", exec.ErrNotFound },
			nil,
		)

		c := DetectContainerRuntime()
		if c.Name != "docker" {
			t.Errorf("Name = %q, want %q", c.Name, "docker")
		}
		if c.Status != StatusWarning {
			t.Errorf("Status = %q, want %q", c.Status, StatusWarning)
		}
		if !strings.Contains(c.Detail, "not found") {
			t.Errorf("Detail %q missing 'not found'", c.Detail)
		}
	})
}
