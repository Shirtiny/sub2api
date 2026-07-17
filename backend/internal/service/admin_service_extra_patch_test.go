package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyAccountExtraPatchPreservesUnknownAndDeepMergesAetherWS(t *testing.T) {
	current := map[string]any{
		"runtime_sample": "keep",
		"legacy":         true,
		"aether_ws": map[string]any{
			"schema_version": 1,
			"enabled":        false,
			"future_field":   "keep",
		},
	}
	patch := &AccountExtraPatch{
		Set: map[string]any{
			"legacy": "set-wins",
			"aether_ws": map[string]any{
				"enabled":                   true,
				"required_control_protocol": "route-v1",
			},
		},
		Delete: []string{"legacy"},
	}

	got, err := applyAccountExtraPatch(current, patch)
	require.NoError(t, err)
	require.Equal(t, "keep", got["runtime_sample"])
	require.Equal(t, "set-wins", got["legacy"])
	require.Equal(t, map[string]any{
		"schema_version":            1,
		"enabled":                   true,
		"future_field":              "keep",
		"required_control_protocol": "route-v1",
	}, got["aether_ws"])
}

func TestApplyAccountExtraPatchDeleteThenSetReplacesAetherWS(t *testing.T) {
	current := map[string]any{
		"aether_ws": map[string]any{
			"enabled":      false,
			"future_field": "must-not-survive",
		},
	}
	requested := &AccountExtraPatch{
		Delete: []string{"aether_ws"},
		Set: map[string]any{
			"aether_ws": map[string]any{"enabled": true},
		},
	}

	got, err := applyAccountExtraPatch(current, requested)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"enabled": true}, got["aether_ws"])

	persisted := buildPersistedAccountExtraPatch(current, got, requested)
	require.Equal(t, []string{"aether_ws"}, persisted.Delete)
	require.Equal(t, map[string]any{"enabled": true}, persisted.Set["aether_ws"])
}

func TestValidateAccountAetherWSExtraRejectsNonObject(t *testing.T) {
	for _, value := range []any{nil, true, "invalid", []any{}} {
		err := ValidateAccountAetherWSExtra(map[string]any{"aether_ws": value})
		require.Error(t, err)
	}
	require.NoError(t, ValidateAccountAetherWSExtra(map[string]any{"aether_ws": map[string]any{"enabled": true}}))
}

func TestBuildAccountColumnPatchDoesNotEchoRuntimeSnapshot(t *testing.T) {
	lastUsed := time.Now().UTC()
	account := &Account{
		ID:           9,
		Name:         "snapshot-name",
		Status:       StatusError,
		Schedulable:  false,
		LastUsedAt:   &lastUsed,
		Credentials:  map[string]any{"runtime": "keep"},
		ErrorMessage: "runtime-error",
	}

	patch := buildAccountColumnPatch(&UpdateAccountInput{
		ExtraPatch: &AccountExtraPatch{Set: map[string]any{"aether_ws": map[string]any{"enabled": true}}},
	}, account)

	require.Nil(t, patch.Name)
	require.Nil(t, patch.Status)
	require.Nil(t, patch.Schedulable)
	require.Nil(t, patch.Credentials)
	require.False(t, patch.NotesSet)
	require.False(t, patch.ProxyIDSet)
	require.False(t, patch.ExpiresAtSet)
}

func TestDiffAccountExtraDoesNotEchoUnchangedRuntimeKeys(t *testing.T) {
	before := map[string]any{
		"runtime_sample": "keep",
		"mode":           "off",
		"legacy":         true,
		"aether_ws": map[string]any{
			"enabled":      false,
			"future_field": "keep",
		},
	}
	after := map[string]any{
		"runtime_sample": "keep",
		"mode":           "passthrough",
		"aether_ws": map[string]any{
			"enabled":      true,
			"future_field": "keep",
		},
	}

	patch := diffAccountExtra(before, after)
	require.Equal(t, map[string]any{
		"mode":      "passthrough",
		"aether_ws": map[string]any{"enabled": true},
	}, patch.Set)
	require.Equal(t, []string{"legacy"}, patch.Delete)
}
