package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"hello world", []string{"hello", "world"}},
		{"{{ .Name | kebab }}", []string{"{", "{", ".name", "|", "kebab", "}", "}"}},
		{"kebab-case", []string{"kebab", "-", "case"}},
		{"MODULE.bazel", []string{"module.bazel"}},
		{"a\tb\nc", []string{"a", "b", "c"}},

		{"{{", []string{"{", "{"}},
	}
	for _, c := range cases {
		got := tokenize(c.in)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("tokenize(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestScoreAllRanksRelevantSeedsFirst(t *testing.T) {
	query := tokenize("func parseTemplate(src string) ([]node, error) { if varNode toggle }")
	docs := []doc{
		{Path: "boilerplate", tokens: tokenize("go test fuzz v1 string(\"aaaa\")")},
		{Path: "relevant", tokens: tokenize("go test fuzz v1 string(\"{{ if .toggle }}{{ .varNode }}\")")},
		{Path: "half", tokens: tokenize("go test fuzz v1 string(\"{{ end }}\")")},
	}
	ranked := scoreAll(query, docs)
	if len(ranked) != 3 {
		t.Fatalf("ranked %d docs, want 3", len(ranked))
	}
	if ranked[0].Path != "relevant" {
		t.Errorf("top-ranked = %q, want relevant (scores: %+v)", ranked[0].Path, ranked)
	}
	if ranked[0].Score <= ranked[len(ranked)-1].Score {
		t.Errorf("expected strict score ordering, got %+v", ranked)
	}
}

func TestScoreAllIsDeterministic(t *testing.T) {
	query := tokenize("parse template")
	docs := []doc{
		{Path: "b", tokens: tokenize("template")},
		{Path: "a", tokens: tokenize("template")},
		{Path: "c", tokens: tokenize("parse")},
	}
	first := scoreAll(query, docs)
	second := scoreAll(query, docs)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("ranking not deterministic at %d: %+v vs %+v", i, first[i], second[i])
		}
	}

	if first[1].Path != "a" || first[2].Path != "b" {
		t.Errorf("ties not path-ordered: %+v", first)
	}
}

func TestScoreAllIDFPushesOutBoilerplate(t *testing.T) {

	query := tokenize("go string uniqueMarker")
	docs := []doc{
		{Path: "boilerplate", tokens: tokenize("go string go string go string")},
		{Path: "with-marker", tokens: tokenize("go string uniqueMarker uniqueMarker")},
	}
	ranked := scoreAll(query, docs)
	if ranked[0].Path != "with-marker" {
		t.Errorf("with-marker should rank first, got %+v", ranked)
	}
}

func TestReadCorpusAndQueryTokens(t *testing.T) {

	corpus := t.TempDir()
	if err := os.MkdirAll(filepath.Join(corpus, "FuzzX"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(corpus, "FuzzX", "aa"), []byte("go test fuzz v1\n"), 0o644)
	os.WriteFile(filepath.Join(corpus, "FuzzX", "bb"), []byte("go test fuzz v1\nstring(\"x\")\n"), 0o644)
	os.WriteFile(filepath.Join(corpus, "empty"), nil, 0o644)

	docs, err := readCorpus(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("readCorpus = %d docs, want 2 (empty file skipped)", len(docs))
	}

	pkg := t.TempDir()
	os.WriteFile(filepath.Join(pkg, "parse.go"), []byte("func parseTemplate() {}"), 0o644)
	os.WriteFile(filepath.Join(pkg, "notes.md"), []byte("ignored"), 0o644)
	tokens, err := readQueryTokens(pkg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) == 0 || tokens[0] != "func" {
		t.Fatalf("query tokens = %v, want Go source tokens", tokens)
	}

	fileOnly := filepath.Join(pkg, "parse.go")
	tokens, err = readQueryTokens(fileOnly)
	if err != nil || len(tokens) == 0 {
		t.Fatalf("single-file query failed: %v %v", tokens, err)
	}
}
