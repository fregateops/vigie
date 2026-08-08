package engine

import (
	"fmt"
	"regexp"
	"strings"
)

var msgPattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// interpolate replaces {{ a.b.c }} patterns in msg with values resolved
// from the bindings map (dotted-path lookup through nested map[string]any).
func interpolate(msg string, bindings map[string]any) string {
	return msgPattern.ReplaceAllStringFunc(msg, func(match string) string {
		path := strings.TrimSpace(msgPattern.FindStringSubmatch(match)[1])
		val := resolvePath(path, bindings)
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	})
}

func resolvePath(path string, bindings map[string]any) any {
	parts := strings.Split(path, ".")
	var cur any = bindings
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		v, exists := m[p]
		if !exists {
			return nil
		}
		cur = v
	}
	return cur
}
