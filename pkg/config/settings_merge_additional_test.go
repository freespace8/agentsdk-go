package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeStatusLineOverrides(t *testing.T) {
	lower := &StatusLineConfig{
		Type:            "command",
		Command:         "echo low",
		Template:        "",
		IntervalSeconds: 10,
		TimeoutSeconds:  5,
	}
	higher := &StatusLineConfig{
		Type:           "template",
		Template:       "{{.User}}",
		TimeoutSeconds: 15,
	}

	out := mergeStatusLine(lower, higher)
	require.NotSame(t, lower, out)
	require.NotSame(t, higher, out)
	require.Equal(t, "template", out.Type)
	require.Equal(t, "echo low", out.Command) // higher empty should not clobber
	require.Equal(t, "{{.User}}", out.Template)
	require.Equal(t, 10, out.IntervalSeconds)
	require.Equal(t, 15, out.TimeoutSeconds)

	require.Nil(t, mergeStatusLine(nil, nil))
	copied := mergeStatusLine(lower, nil)
	require.Equal(t, lower.Command, copied.Command)
	require.NotSame(t, lower, copied)
}

func TestMergeMarketplaceConfigMap(t *testing.T) {
	lower := map[string]*MarketplaceConfig{
		"oss": {
			EnabledPlugins:         map[string]bool{"p1@oss": true},
			ExtraKnownMarketplaces: map[string]MarketplaceSource{"oss": {Source: "directory", Path: "/lower"}},
		},
	}
	higher := map[string]*MarketplaceConfig{
		"oss": {
			EnabledPlugins:         map[string]bool{"p1@oss": false, "p2@oss": true},
			ExtraKnownMarketplaces: map[string]MarketplaceSource{"oss": {Source: "directory", Path: "/higher"}},
		},
		"new": {
			EnabledPlugins: map[string]bool{"fresh@new": true},
		},
	}

	out := mergeMarketplaceConfigMap(lower, higher)
	require.Len(t, out, 2)
	require.Equal(t, map[string]bool{"p1@oss": false, "p2@oss": true}, out["oss"].EnabledPlugins)
	require.Equal(t, "/higher", out["oss"].ExtraKnownMarketplaces["oss"].Path)
	require.Equal(t, map[string]bool{"fresh@new": true}, out["new"].EnabledPlugins)

	// no aliasing
	higher["oss"].EnabledPlugins["p3@oss"] = true
	require.NotContains(t, out["oss"].EnabledPlugins, "p3@oss")
}

func TestMergeMarketplaceConfigNilHandling(t *testing.T) {
	require.Nil(t, mergeMarketplaceConfig(nil, nil))

	base := &MarketplaceConfig{EnabledPlugins: map[string]bool{"a": true}}
	require.Equal(t, base.EnabledPlugins, mergeMarketplaceConfig(base, nil).EnabledPlugins)

	higher := &MarketplaceConfig{ExtraKnownMarketplaces: map[string]MarketplaceSource{"oss": {Source: "git"}}}
	out := mergeMarketplaceConfig(base, higher)
	require.Equal(t, map[string]bool{"a": true}, out.EnabledPlugins)
	require.Equal(t, "git", out.ExtraKnownMarketplaces["oss"].Source)
}

func TestMergeMCPServerRules(t *testing.T) {
	lower := []MCPServerRule{{ServerName: "low"}}
	higher := []MCPServerRule{{ServerName: "high"}}

	require.Equal(t, higher, mergeMCPServerRules(lower, higher))
	require.Equal(t, lower, mergeMCPServerRules(lower, nil))
	require.Nil(t, mergeMCPServerRules(nil, nil))
}

func TestMergeMCPConfig(t *testing.T) {
	lower := &MCPConfig{Servers: map[string]MCPServerConfig{
		"base": {Type: "stdio", Command: "node"},
	}}
	higher := &MCPConfig{Servers: map[string]MCPServerConfig{
		"remote": {Type: "http", URL: "https://example"},
	}}

	out := mergeMCPConfig(lower, higher)
	require.Len(t, out.Servers, 2)
	require.Equal(t, "node", out.Servers["base"].Command)
	require.Equal(t, "https://example", out.Servers["remote"].URL)

	out.Servers["base"] = MCPServerConfig{}
	require.Equal(t, "node", lower.Servers["base"].Command)
}
