// `guppy template list` and `guppy template search` (design §6).
//
// list shows what is resolvable locally: built-ins + user scope by
// default, community with --all-scopes (the trust-gated scope is opt-in
// visibility, and shadowing warnings surface §4.2 collisions).
// search queries the registry index — it always means the remote
// catalog, so there is no scope qualifier; results print both add forms
// (first-party and community:).
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nicedata-work/guppy/internal/dist"
	"github.com/nicedata-work/guppy/internal/templates"
)

func newTemplateListCmd(rt *runtime, out *output) *cobra.Command {
	var allScopes bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed templates (built-in, user, community)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTemplateList(cmd.Context(), rt, out, allScopes)
		},
	}
	cmd.Flags().BoolVar(&allScopes, "all-scopes", false, "include the trust-gated community scope")
	return cmd
}

func runTemplateList(ctx context.Context, rt *runtime, out *output, allScopes bool) error {
	res := rt.resolver()

	var warnings []string
	res.Warn = func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}
	list, err := res.List(ctx, allScopes)
	if err != nil {
		return Fail(err)
	}
	for _, w := range warnings {
		out.warnf("%s", w)
	}

	records, err := rt.distStore().Load()
	if err != nil {
		return Fail(err)
	}

	w := newTable(out.stdout, "ID", "VERSION", "KIND", "LANG", "SCOPE", "SOURCE").styled(out.outP())
	for _, t := range list {
		source := sourceLabel(t.Identity, records)
		w.row(paintByScope(out.outP(), t.Manifest.ID, t.Identity.Scope), versionOrDash(t),
			orDash(t.Manifest.Kind), orDash(t.Manifest.Language),
			paintByScope(out.outP(), t.Identity.Scope.String(), t.Identity.Scope), paintSource(out.outP(), source))
	}
	w.flush()
	return nil
}

// paintScope colours the SCOPE column: builtin is dim (always there),
// community yellow (trust-gated, §4.1), user plain. Decoration only —
// the plain text is unchanged when colour is off.
func paintByScope(p painter, id string, scope templates.Scope) string {
	switch scope {
	case templates.ScopeBuiltin:
		return p.paint(dim, id)
	case templates.ScopeCommunity:
		return p.paint(yellow, id)
	default:
		return id
	}
}

// paintSource flags untrusted provenance in red.
func paintSource(p painter, source string) string {
	if strings.Contains(source, "untrusted") {
		return p.paint(red, source)
	}
	return source
}

// versionOrDash prefers the identity version (from the manifest) —
// builtin manifests carry one too.
func versionOrDash(t templates.Template) string {
	if v := t.Identity.Version.String(); v != "" {
		return v
	}
	return "-"
}

// sourceLabel renders the provenance column from the records ledger,
// falling back to the on-disk/embedded truth.
func sourceLabel(id templates.Identity, records []dist.Record) string {

	scope := dist.ScopeName(id.Scope.String())
	if r, ok := dist.Find(records, id.ID, scope); ok {
		return r.Source
	}
	switch id.Scope {
	case templates.ScopeBuiltin:
		return "embedded"
	case templates.ScopeCommunity:
		return "(unrecorded — untrusted)"
	default:
		return "local"
	}
}

func newTemplateSearchCmd(rt *runtime, out *output) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Search the configured registries",
		Long: `Search the configured registries (named team registries first, then
the default) for templates carrying the guppy-template keyword. With no
query, lists every indexed template. The registry is the remote catalog —
search is never local. Install a hit with "guppy template add <id>"
(first-party) or "guppy template add community:<id>" (community,
review-gated).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			hits, err := rt.searchRegistries(cmd.Context(), query)
			if err != nil {
				return err
			}
			if len(hits) == 0 {
				_, _ = fmt.Fprintf(out.stdout, "no templates match %q\n", query)
				return nil
			}
			w := newTable(out.stdout, "ID", "VERSION", "KIND", "LANG", "DESCRIPTION").styled(out.outP())
			for _, h := range hits {
				id := h.ID
				if h.Registry != "" {
					id = fmt.Sprintf("%s (%s)", id, h.Registry)
				}
				w.row(id, h.Version, orDash(h.Kind), orDash(h.Language), orDash(h.Description))
			}
			w.flush()
			_, _ = fmt.Fprintf(out.stdout, "\ninstall: guppy template add <id>  ·  guppy template add community:<id> (review-gated)\n")
			return nil
		},
	}
}

// table is a minimal aligned-column writer (ADR 0009: stdlib plain
// text, no TUI dependency). Widths use visibleLen, so cells may carry
// ANSI styles; headers print bold when colour is on.
type table struct {
	out     io.Writer
	p       painter
	columns []string
	rows    [][]string
	widths  []int
}

func newTable(out io.Writer, cols ...string) *table {
	return &table{out: out, columns: cols, widths: make([]int, len(cols))}
}

// styled binds a painter to the table so rows and headers may carry
// colour; chain it after newTable.
func (t *table) styled(p painter) *table {
	t.p = p
	return t
}

func (t *table) row(cells ...string) {
	t.rows = append(t.rows, cells)
	for i, c := range cells {
		if i < len(t.widths) && visibleLen(c) > t.widths[i] {
			t.widths[i] = visibleLen(c)
		}
	}
}

func (t *table) flush() {
	for i, c := range t.columns {
		if len(c) > t.widths[i] {
			t.widths[i] = len(c)
		}
	}
	header := upperAll(t.columns)
	for i, c := range header {
		header[i] = t.p.paint(bold, c)
	}
	t.printRow(header)
	for _, r := range t.rows {
		t.printRow(r)
	}
}

func (t *table) printRow(cells []string) {
	for i, c := range cells {
		if i > 0 {
			_, _ = fmt.Fprint(t.out, "  ")
		}
		if i == len(cells)-1 {
			_, _ = fmt.Fprint(t.out, c)
		} else {
			_, _ = fmt.Fprint(t.out, padTo(c, t.widths[i]))
		}
	}
	_, _ = fmt.Fprintln(t.out)
}

func upperAll(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = strings.ToUpper(c)
	}
	return out
}
