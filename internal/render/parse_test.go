package render

import (
	"strings"
	"testing"
)

func TestParseTemplateGrammar(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{"plain text", "hello", ""},
		{"simple var", "hi {{ .name }}", ""},
		{"filtered var", "{{ .name | kebab | upper }}", ""},
		{"if block", "{{ if .bazel }}x{{ end }}", ""},
		{"nested if", "{{ if .a }}1{{ if .b }}2{{ end }}3{{ end }}", ""},
		{"unclosed if", "{{ if .a }}x", "never closed"},
		{"stray end", "x{{ end }}", "without matching"},
		{"unclosed action", "hi {{ .name", "unclosed"},
		{"unsupported action", "{{ with .x }}", "unsupported action"},
		{"else rejected", "{{ if .a }}x{{ else }}y{{ end }}", "unsupported action"},
		{"unknown filter", "{{ .name | shout }}", "unknown filter"},
		{"non-dot ref", "{{ name }}", "unsupported action"},
		{"bad var name", "{{ .9lives }}", "invalid variable name"},
		{"bad if ref", "{{ if bazel }}", "expected a toggle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTemplate(tt.src)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("parse(%q) = %v, want ok", tt.src, err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("parse(%q) = ok, want error containing %q", tt.src, tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("parse(%q) = %q, want error containing %q", tt.src, err, tt.wantErr)
			}
		})
	}
}

func TestApplyFilter(t *testing.T) {
	tests := []struct{ in, filter, want string }{
		{"My CLI App", "kebab", "my-cli-app"},
		{"My CLI App", "snake", "my_cli_app"},
		{"My CLI App", "upper", "MY CLI APP"},
		{"My CLI App", "lower", "my cli app"},
		{"github.com/acme/widget", "basename", "widget"},
		{"widget", "basename", "widget"},
		{"--weird__  name--", "kebab", "weird-name"},
	}
	for _, tt := range tests {
		if got := applyFilter(tt.filter, tt.in); got != tt.want {
			t.Errorf("applyFilter(%q, %q) = %q, want %q", tt.filter, tt.in, got, tt.want)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"**/*", "a.txt", true},
		{"**/*", "deep/nested/a.txt", true},
		{"**/*", "deep", true},
		{"*.go", "main.go", true},
		{"*.go", "pkg/main.go", false},
		{"**/*.go", "pkg/main.go", true},
		{"template/scratch/**", "template/scratch/x", true},
		{"template/scratch/**", "template/other/x", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
	}
	for _, tt := range tests {
		if got := matchGlob(tt.pattern, tt.name); got != tt.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}
