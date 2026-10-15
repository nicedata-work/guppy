package templates

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"gotest.tools/v3/assert"
)

func TestIdentityKeyStableAcrossRoutes(t *testing.T) {
	v := *semver.MustParse("1.2.3")
	a := Identity{Scope: ScopeUser, Kind: SourceRegistry, ID: "go-cli", Version: v, Pin: "1.2.3"}
	b := Identity{Scope: ScopeCommunity, Kind: SourceRegistry, ID: "go-cli", Version: v, Pin: "1.2.3"}

	assert.Equal(t, a.Key(), b.Key())
}

func TestIdentityKeyUnresolvedFallback(t *testing.T) {
	v := *semver.MustParse("0.3.1")
	id := Identity{Kind: SourceGit, ID: "base", Version: v}
	assert.Equal(t, id.Key(), "git:base@0.3.1")
}

func TestScopeString(t *testing.T) {
	for s, want := range map[Scope]string{
		ScopeBuiltin: "builtin", ScopeUser: "user", ScopeLocal: "local", ScopeCommunity: "community",
	} {
		assert.Equal(t, s.String(), want)
	}
}
