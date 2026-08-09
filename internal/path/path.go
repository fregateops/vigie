package path

import (
	"fmt"
	"strconv"
	"strings"
)

// Get resolves a dotted/bracketed path expression against a document.
// Returns (value, found, error). Supports dotted keys (spec.replicas), integer
// indexing (containers[0].image), and quoted bracket keys for map keys that
// contain dots or slashes (metadata.labels["app.kubernetes.io/name"]).
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

// parse tokenises a path into segments. It scans character by character so that
// quoted bracket keys (e.g. ["app.kubernetes.io/name"]) keep their dots and
// slashes instead of being split on ".". Bare `.` separates keys; `[…]` holds
// either an integer index or a quoted string key.
func parse(expr string) ([]segment, error) {
	var segments []segment
	var key strings.Builder
	flushKey := func() {
		if key.Len() > 0 {
			segments = append(segments, segment{key: key.String(), index: -1})
			key.Reset()
		}
	}

	for idx := 0; idx < len(expr); {
		switch expr[idx] {
		case '.':
			flushKey()
			idx++
		case '[':
			flushKey()
			if idx+1 >= len(expr) {
				return nil, fmt.Errorf("path: unmatched '[' in %q", expr)
			}
			if quote := expr[idx+1]; quote == '"' || quote == '\'' {
				end := strings.IndexByte(expr[idx+2:], quote)
				if end < 0 {
					return nil, fmt.Errorf("path: unterminated quoted key in %q", expr)
				}
				end += idx + 2
				if end+1 >= len(expr) || expr[end+1] != ']' {
					return nil, fmt.Errorf("path: expected ']' after quoted key in %q", expr)
				}
				segments = append(segments, segment{key: expr[idx+2 : end], index: -1})
				idx = end + 2
				continue
			}
			close := strings.IndexByte(expr[idx:], ']')
			if close < 0 {
				return nil, fmt.Errorf("path: unmatched '[' in %q", expr)
			}
			close += idx
			idxStr := expr[idx+1 : close]
			n, err := strconv.Atoi(strings.TrimSpace(idxStr))
			if err != nil {
				return nil, fmt.Errorf("path: non-integer index %q in %q", idxStr, expr)
			}
			if n < 0 {
				return nil, fmt.Errorf("path: negative index %q in %q", idxStr, expr)
			}
			segments = append(segments, segment{index: n})
			idx = close + 1
		default:
			key.WriteByte(expr[idx])
			idx++
		}
	}
	flushKey()
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
