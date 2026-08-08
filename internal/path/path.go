package path

import (
	"fmt"
	"strconv"
	"strings"
)

// Get resolves a dotted/bracketed path expression against a document.
// Returns (value, found, error). Supports: spec.replicas, containers[0].image
func Get(doc any, expr string) (any, bool, error) {
	if expr == "" || expr == "." {
		return doc, true, nil
	}

	segments, err := parse(expr)
	if err != nil {
		return nil, false, err
	}

	cur := doc
	for _, seg := range segments {
		if seg.index >= 0 {
			// array index
			slice, ok := toSlice(cur)
			if !ok {
				return nil, false, nil
			}
			if seg.index >= len(slice) {
				return nil, false, nil
			}
			cur = slice[seg.index]
		} else {
			// map key
			m, ok := toMap(cur)
			if !ok {
				return nil, false, nil
			}
			v, exists := m[seg.key]
			if !exists {
				return nil, false, nil
			}
			cur = v
		}
	}
	return cur, true, nil
}

type segment struct {
	key   string
	index int // -1 means key lookup
}

func parse(expr string) ([]segment, error) {
	var segments []segment
	// Split on dots first, then handle [N] within each part.
	parts := strings.Split(expr, ".")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Handle bracket notation: foo[0][1]
		for {
			ob := strings.IndexByte(part, '[')
			if ob < 0 {
				if part != "" {
					segments = append(segments, segment{key: part, index: -1})
				}
				break
			}
			if ob > 0 {
				segments = append(segments, segment{key: part[:ob], index: -1})
			}
			cb := strings.IndexByte(part, ']')
			if cb < 0 {
				return nil, fmt.Errorf("path: unmatched '[' in %q", expr)
			}
			idxStr := part[ob+1 : cb]
			idx, err := strconv.Atoi(idxStr)
			if err != nil {
				return nil, fmt.Errorf("path: non-integer index %q in %q", idxStr, expr)
			}
			segments = append(segments, segment{index: idx})
			part = part[cb+1:]
		}
	}
	return segments, nil
}

func toMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func toSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}
