//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type atomicAccountPatchRepoStub struct {
	mockAccountRepoForGemini
	account           *Account
	legacyUpdateCalls int
	patchCalls        int
	columns           AccountColumnPatch
	set               map[string]any
	deleteKeys        []string
}

func (r *atomicAccountPatchRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	return r.account, nil
}

func (r *atomicAccountPatchRepoStub) Update(_ context.Context, _ *Account) error {
	r.legacyUpdateCalls++
	return nil
}

func (r *atomicAccountPatchRepoStub) UpdateWithExtraPatch(
	_ context.Context,
	_ int64,
	columns AccountColumnPatch,
	set map[string]any,
	deleteKeys []string,
	_ []int64,
) error {
	r.patchCalls++
	r.columns = columns
	r.set = set
	r.deleteKeys = append([]string{}, deleteKeys...)
	return nil
}

func TestUpdateAccountAetherOnlyUsesExtraPatchWithoutRuntimeColumns(t *testing.T) {
	lastUsed := time.Now().UTC()
	repo := &atomicAccountPatchRepoStub{account: &Account{
		ID:           81,
		Name:         "unchanged",
		Platform:     PlatformOpenAI,
		Type:         AccountTypeAPIKey,
		Status:       StatusError,
		Schedulable:  false,
		ErrorMessage: "runtime error",
		LastUsedAt:   &lastUsed,
		Credentials:  map[string]any{"api_key": "runtime-secret"},
		Extra: map[string]any{
			"aether_ws": map[string]any{
				"schema_version":            1,
				"enabled":                   false,
				"required_control_protocol": "route-v1",
			},
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), repo.account.ID, &UpdateAccountInput{
		ExtraPatch: &AccountExtraPatch{Set: map[string]any{
			"aether_ws": map[string]any{"enabled": true},
		}},
	})

	require.NoError(t, err)
	require.Equal(t, 1, repo.patchCalls)
	require.Zero(t, repo.legacyUpdateCalls)
	require.Nil(t, repo.columns.Name)
	require.Nil(t, repo.columns.Credentials)
	require.Nil(t, repo.columns.Status)
	require.Nil(t, repo.columns.Schedulable)
	require.False(t, repo.columns.NotesSet)
	require.False(t, repo.columns.ProxyIDSet)
	require.False(t, repo.columns.ExpiresAtSet)
	require.Equal(t, map[string]any{"enabled": true}, repo.set["aether_ws"])
	require.Empty(t, repo.deleteKeys)
}

func TestUpdateAccountLegacyExtraStillUsesColumnPatchRepository(t *testing.T) {
	repo := &atomicAccountPatchRepoStub{account: &Account{
		ID:          82,
		Name:        "unchanged",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Status:      StatusError,
		Schedulable: false,
		Credentials: map[string]any{"runtime_token": "do-not-write"},
		Extra: map[string]any{
			"old_config": "remove",
			"quota_used": 12.5,
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), repo.account.ID, &UpdateAccountInput{
		Extra: map[string]any{"new_config": true},
	})

	require.NoError(t, err)
	require.Equal(t, 1, repo.patchCalls)
	require.Zero(t, repo.legacyUpdateCalls)
	require.Nil(t, repo.columns.Credentials)
	require.Nil(t, repo.columns.Status)
	require.Equal(t, true, repo.set["new_config"])
	require.NotContains(t, repo.set, "quota_used", "runtime quota keys already in the JSONB column must be left untouched")
	require.NotContains(t, repo.deleteKeys, "quota_used")
	require.Contains(t, repo.deleteKeys, "old_config")
}
