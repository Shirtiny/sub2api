package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type concurrencyRulesHandlerRepo struct {
	service.SettingRepository
	value string
}

func (r *concurrencyRulesHandlerRepo) GetValue(context.Context, string) (string, error) {
	if r.value == "" {
		return "", service.ErrSettingNotFound
	}
	return r.value, nil
}
func (r *concurrencyRulesHandlerRepo) Set(_ context.Context, _, value string) error {
	r.value = value
	return nil
}

func TestConcurrencyRulesHandlerValidatesAndRoundTrips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &concurrencyRulesHandlerRepo{}
	h := &SettingHandler{settingService: service.NewSettingService(repo, nil)}
	router := gin.New()
	router.GET("/rules", h.GetUserConcurrencyRules)
	router.PUT("/rules", h.UpdateUserConcurrencyRules)
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, "/rules", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	valid := `{"balance_tiers":[{"min_balance":0,"concurrency":4},{"min_balance":25,"concurrency":6}]}`
	require.Equal(t, http.StatusOK, request(http.MethodPut, valid).Code)
	for _, body := range []string{
		`{}`, `null`, `{"balance_tiers":null}`, `{"balance_tiers":[]}`,
		`{"balance_tiers":[{"concurrency":4}]}`,
		`{"balance_tiers":[{"min_balance":null,"concurrency":4}]}`,
		`{"balance_tiers":[{"min_balance":0,"concurrency":1.5}]}`,
		`{"balance_tiers":[{"min_balance":0,"concurrency":0}]}`,
		`{"balance_tiers":[{"min_balance":10,"concurrency":1}]}`,
		`{"balance_tiers":[{"min_balance":0,"concurrency":1},{"min_balance":0,"concurrency":2}]}`,
		strings.Replace(valid, `"min_balance":25`, `"min_balance":"25"`, 1),
		strings.Replace(valid, `"min_balance":25`, `"min_balance":1e999`, 1),
		strings.Replace(valid, `"min_balance":25`, `"min_balance":25,"override":true`, 1),
		valid + `{}`, valid + strings.Repeat(" ", 17<<10),
	} {
		require.Equal(t, http.StatusBadRequest, request(http.MethodPut, body).Code, body[:min(len(body), 200)])
	}
	response := request(http.MethodGet, "")
	require.Equal(t, http.StatusOK, response.Code)
	var out struct {
		Data service.UserConcurrencyRules `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &out))
	require.Equal(t, []service.BalanceConcurrencyRule{{MinBalance: 0, Concurrency: 4}, {MinBalance: 25, Concurrency: 6}}, out.Data.BalanceTiers)
	var persisted service.UserConcurrencyRules
	require.NoError(t, json.Unmarshal([]byte(repo.value), &persisted))
	require.Equal(t, out.Data, persisted, "invalid writes do not alter saved rules")
}
