package config

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingsPathHelpers(t *testing.T) {
	require.Equal(t, "", getProjectSettingsPath(""))
	require.Equal(t, "", getLocalSettingsPath(""))

	managed := getManagedSettingsPath()
	require.NotEmpty(t, managed)
	switch runtime.GOOS {
	case "darwin":
		require.Contains(t, managed, "/Library/Application Support")
	case "windows":
		require.Contains(t, managed, `ProgramData`)
	default:
		require.Contains(t, managed, "/etc/")
	}
}
