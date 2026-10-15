// The template syntax of design §3.3 — a deliberately small grammar, the
// uv-side of the trade-off: no logic, no partials, no control flow beyond
// boolean blocks. This file owns parsing; lint statically verifies that
// every substitution resolves.
//
//	{{ .name }}                  required var, prompted if absent
//	{{ .module_path }}           declared var with prompt/default
//	{{ .name | kebab }}          filters: kebab, snake, upper, lower, basename
//	{{ if .bazel }}…{{ end }}    the ONLY conditional: default toggles, blocks
package render

import (
	"fmt"
	"regexp"
	"strings"
)

type nodeKind int

const (
	textNode nodeKind = iota
	varNode
	ifNode
)

// node is one piece of a parsed template: literal text, a variable
// substitution, or a toggle-gated block (nestable; there is no else).
type node struct {
	kind    nodeKind
	text    string
	name    string
	filters []string
	body    []node
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// filters are the complete, closed set of §3.3. Adding one is a design
// change, not a template feature.
var filters = map[string]bool{"kebab": true, "snake": true, "upper": true, "lower": true, "basename": true}

// parseTemplate parses one template source into a node list. Syntax errors
// are lint errors: guppy must be able to statically verify every
// substitution in the payload.
func parseTemplate(src string) ([]node, error) {
	var (
		out   []node
		stack []*ifNodeAccumulator
		rest  = src
	)
	for {
		i := strings.Index(rest, "{{")
		if i < 0 {
			text := rest
			if len(stack) > 0 {
				stack[len(stack)-1].body = append(stack[len(stack)-1].body, node{kind: textNode, text: text})
				break
			}
			if text != "" || len(out) == 0 {
				out = append(out, node{kind: textNode, text: text})
			}
			break
		}
		j := strings.Index(rest[i:], "}}")
		if j < 0 {
			return nil, fmt.Errorf("unclosed {{ at end of template")
		}
		j += i
		if i > 0 {
			text := rest[:i]
			if len(stack) > 0 {
				stack[len(stack)-1].body = append(stack[len(stack)-1].body, node{kind: textNode, text: text})
			} else {
				out = append(out, node{kind: textNode, text: text})
			}
		}
		action := strings.TrimSpace(rest[i+2 : j])
		rest = rest[j+2:]

		switch {
		case strings.HasPrefix(action, "if "):
			name, err := parseToggleRef(strings.TrimSpace(action[3:]))
			if err != nil {
				return nil, err
			}
			stack = append(stack, &ifNodeAccumulator{name: name})
		case action == "end":
			if len(stack) == 0 {
				return nil, fmt.Errorf("{{ end }} without matching {{ if }}")
			}
			finished := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			in := node{kind: ifNode, name: finished.name, body: finished.body}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.body = append(parent.body, in)
			} else {
				out = append(out, in)
			}
		case strings.HasPrefix(action, "."):
			n, err := parseVarAction(action)
			if err != nil {
				return nil, err
			}
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				top.body = append(top.body, n)
			} else {
				out = append(out, n)
			}
		default:
			return nil, fmt.Errorf("unsupported action {{ %s }}: only {{ .var }}, {{ .var | filter }}, and {{ if .toggle }}…{{ end }} are allowed", action)
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("{{ if .%s }} is never closed with {{ end }}", stack[len(stack)-1].name)
	}
	return out, nil
}

type ifNodeAccumulator struct {
	name string
	body []node
}

func parseToggleRef(s string) (string, error) {
	if !strings.HasPrefix(s, ".") {
		return "", fmt.Errorf("{{ if %s }}: expected a toggle like {{ if .bazel }}", s)
	}
	name := s[1:]
	if !namePattern.MatchString(name) {
		return "", fmt.Errorf("invalid toggle name %q", name)
	}
	return name, nil
}

func parseVarAction(action string) (node, error) {
	parts := strings.Split(action, "|")
	ref := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(ref, ".") {
		return node{}, fmt.Errorf("expected a variable like {{ .name }}, got {{ %s }}", action)
	}
	name := ref[1:]
	if !namePattern.MatchString(name) {
		return node{}, fmt.Errorf("invalid variable name %q", name)
	}
	n := node{kind: varNode, name: name}
	for _, f := range parts[1:] {
		f = strings.TrimSpace(f)
		if !filters[f] {
			return node{}, fmt.Errorf("unknown filter %q: must be one of kebab, snake, upper, lower, basename", f)
		}
		n.filters = append(n.filters, f)
	}
	return n, nil
}

// applyFilter applies one closed-set filter to a value.
func applyFilter(name, v string) string {
	switch name {
	case "kebab":
		return sepJoin(v, "-")
	case "snake":
		return sepJoin(v, "_")
	case "upper":
		return strings.ToUpper(v)
	case "lower":
		return strings.ToLower(v)
	case "basename":
		if i := strings.LastIndex(v, "/"); i >= 0 {
			return v[i+1:]
		}
		return v
	default:
		return v
	}
}

// sepJoin lowercases v and joins runs of non-alphanumeric characters with
// sep, trimming separators (kebab/snake of "My App" == "my-app"/"my_app").
func sepJoin(v, sep string) string {
	var b strings.Builder
	lastWasSep := true
	for _, r := range strings.ToLower(v) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasSep = false
		default:
			if !lastWasSep {
				b.WriteString(sep)
				lastWasSep = true
			}
		}
	}
	return strings.TrimRight(b.String(), sep)
}
