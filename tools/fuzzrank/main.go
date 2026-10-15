// Command fuzzrank prioritizes fuzz-corpus entries with Okapi BM25.
//
// Rationale: our fuzz budget is time-boxed (CI runs
// 30s per target), so which seeds the mutator starts from matters. A
// grown go-fuzz cache contains hundreds of inputs; BM25 ranks them by
// lexical distinctiveness against the fuzzed package's own source, so
// inputs rich in the identifiers and syntax the target actually reasons
// about (var names, filters, `{{`…`end` markers) are distilled first.
// Common boilerplate inputs score low via the inverse-document-frequency
// term and get dropped by -top.
//
// This is a dev/CI tool, not part of the guppy binary (no new runtime
// dependency; AGENTS.md). Usage:
//
//	fuzzrank -query internal/render -corpus $GOCACHE/fuzz -top 64
//
// Output is a deterministic ranking (score desc, path tie-break):
// one "<score>\t<path>" line per kept corpus file, or just paths with
// -paths for scripting (scripts/fuzz.sh consumes that).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	var (
		query   = flag.String("query", "", "package source dir (or file) that defines the query terms")
		corpus  = flag.String("corpus", "", "corpus root(s): comma-separated dirs, searched recursively")
		top     = flag.Int("top", 0, "keep only the N highest-scoring entries (0 = all)")
		paths   = flag.Bool("paths", false, "print only the ranked paths (one per line)")
		verbose = flag.Bool("v", false, "print the full score table")
	)
	flag.Parse()
	if *query == "" || *corpus == "" {
		fmt.Fprintln(os.Stderr, "usage: fuzzrank -query DIR [-corpus DIR,...] [-top N] [-paths] [-v]")
		os.Exit(2)
	}

	queryTokens, err := readQueryTokens(*query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuzzrank: read query: %v\n", err)
		os.Exit(1)
	}

	docs, err := readCorpus(*corpus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fuzzrank: read corpus: %v\n", err)
		os.Exit(1)
	}

	ranked := scoreAll(queryTokens, docs)
	if *top > 0 && len(ranked) > *top {
		ranked = ranked[:*top]
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for _, r := range ranked {
		if *paths {
			fmt.Fprintln(out, r.Path)
		} else if *verbose {
			fmt.Fprintf(out, "%.4f\t%s\n", r.Score, r.Path)
		} else {
			fmt.Fprintf(out, "%.4f\t%s\n", r.Score, filepath.Base(r.Path))
		}
	}
}

// readQueryTokens tokenizes every .go file in dir (recursively; one
// level is the normal case) — the fuzzed package's own identifiers and
// syntax are the query. A single file path is accepted too.
func readQueryTokens(dir string) ([]string, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	if !st.IsDir() {
		files = []string{dir}
	} else {
		if err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") {
				files = append(files, p)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	slices.Sort(files)
	var tokens []string
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tokenize(string(body))...)
	}
	return tokens, nil
}

// readCorpus loads every file under the comma-separated roots (skipping
// unreadable/unremarkable dirs) as one document set. Corpus files are
// `go test fuzz v1` entries; tokenizing their raw text is exactly right
// — the encoding wrappers ("string(", "go test fuzz v1") are the same
// boilerplate in every document and IDF pushes them out.
func readCorpus(roots string) ([]doc, error) {
	var docs []doc
	for root := range strings.SplitSeq(roots, ",") {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {

				return nil
			}
			if d.IsDir() {
				return nil
			}
			body, err := os.ReadFile(p)
			if err != nil || len(body) == 0 {
				return nil
			}
			docs = append(docs, doc{Path: p, tokens: tokenize(string(body))})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no corpus files under %s", roots)
	}
	return docs, nil
}

// doc is one scored corpus entry.
type doc struct {
	Path   string
	tokens []string
}

// tokenize lowercases s into a deterministic token stream: maximal
// runs of letters/digits/_/. are single tokens; every other
// non-space character is its own token (so `{{`, `}}`, `|`, `=` —
// the renderer grammar's punctuation — are first-class query terms).
func tokenize(s string) []string {
	var (
		out   []string
		build strings.Builder
	)
	flush := func() {
		if build.Len() > 0 {
			out = append(out, strings.ToLower(build.String()))
			build.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '.':
			build.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			flush()
			out = append(out, string(r))
		}
	}
	flush()
	return out
}

// BM25 parameters (the classic defaults; Okapi BM25 with the +1
// smoothing form of IDF, which keeps scores non-negative even when a
// term appears in every document).
const (
	k1 = 1.2
	b  = 0.75
)

// score is one ranked result.
type score struct {
	Path  string
	Score float64
}

// scoreAll ranks docs by BM25 against queryTokens. Deterministic: ties
// break on path, and corpus iteration is sorted.
func scoreAll(queryTokens []string, docs []doc) []score {
	n := len(docs)
	if n == 0 {
		return nil
	}

	queryTerms := map[string]bool{}
	for _, t := range queryTokens {
		queryTerms[t] = true
	}

	var (
		df    = map[string]int{}
		tf    = make([]map[string]int, n)
		dl    = make([]int, n)
		total int
	)
	for i, d := range docs {
		tf[i] = map[string]int{}
		for _, t := range d.tokens {
			tf[i][t]++
			dl[i]++
		}
		total += dl[i]
		for term := range queryTerms {
			if tf[i][term] > 0 {
				df[term]++
			}
		}
	}
	avgdl := float64(total) / float64(n)

	out := make([]score, 0, n)
	for i, d := range docs {
		var s float64
		for term := range queryTerms {
			f := float64(tf[i][term])
			if f == 0 {
				continue
			}
			idf := math.Log((float64(n-df[term])+0.5)/(float64(df[term])+0.5) + 1)
			norm := f * (k1 + 1) / (f + k1*(1-b+b*float64(dl[i])/avgdl))
			s += idf * norm
		}
		out = append(out, score{Path: d.Path, Score: s})
	}
	slices.SortFunc(out, func(a, b score) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}
