package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountHandlerUpdateForwardsExtraPatch(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)
	body, err := json.Marshal(map[string]any{
		"extra_patch": map[string]any{
			"set": map[string]any{
				"base_rpm":  20000,
				"aether_ws": map[string]any{"enabled": true},
			},
			"delete": []string{"legacy_key"},
		},
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, adminSvc.updatedAccounts, 1)
	patch := adminSvc.updatedAccounts[0].ExtraPatch
	require.NotNil(t, patch)
	require.Equal(t, 10000, patch.Set["base_rpm"])
	require.Equal(t, []string{"legacy_key"}, patch.Delete)
	require.Nil(t, adminSvc.updatedAccounts[0].Extra)
}

func TestAccountHandlerUpdateRejectsExtraAndPatchTogether(t *testing.T) {
	adminSvc := newStubAdminService()
	router := setupAccountMixedChannelRouter(adminSvc)
	body := []byte(`{"extra":{"a":1},"extra_patch":{"set":{"b":2},"delete":[]}}`)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/3", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, adminSvc.updatedAccounts)
}

func TestAccountHandlerRejectsNonObjectAetherWS(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/api/v1/admin/accounts",
			body:   `{"name":"invalid","platform":"openai","type":"apikey","credentials":{"api_key":"sk-test"},"extra":{"aether_ws":"invalid"}}`,
		},
		{
			name:   "single replacement",
			method: http.MethodPut,
			path:   "/api/v1/admin/accounts/3",
			body:   `{"extra":{"aether_ws":true}}`,
		},
		{
			name:   "single patch",
			method: http.MethodPut,
			path:   "/api/v1/admin/accounts/3",
			body:   `{"extra_patch":{"set":{"aether_ws":null},"delete":[]}}`,
		},
		{
			name:   "bulk",
			method: http.MethodPost,
			path:   "/api/v1/admin/accounts/bulk-update",
			body:   `{"account_ids":[1,2],"extra":{"aether_ws":[]}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adminSvc := newStubAdminService()
			router := setupAccountMixedChannelRouter(adminSvc)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Empty(t, adminSvc.updatedAccounts)
			require.Empty(t, adminSvc.createdAccounts)
		})
	}
}
