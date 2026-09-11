package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type wsHTTPFallbackHandlerCache struct {
	service.GatewayCache
	mu     sync.Mutex
	marked map[string]bool
	err    error
}

func (c *wsHTTPFallbackHandlerCache) OpenAIWSHTTPFallbackRequired(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.marked[key], c.err
}

func (c *wsHTTPFallbackHandlerCache) MarkOpenAIWSHTTPFallback(_ context.Context, key string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.marked[key] = true
	}
	return c.err
}

func (*wsHTTPFallbackHandlerCache) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}
func (*wsHTTPFallbackHandlerCache) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}
func (*wsHTTPFallbackHandlerCache) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}
func (*wsHTTPFallbackHandlerCache) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

type wsHTTPFallbackSnapshot struct {
	service.SchedulerCache
	repo *openAIWSFailoverHandlerAccountRepoStub
}

func (s *wsHTTPFallbackSnapshot) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *wsHTTPFallbackSnapshot) GetSnapshot(context.Context, service.SchedulerBucket) ([]*service.Account, bool, error) {
	accounts := make([]*service.Account, 0, len(s.repo.accounts))
	for _, account := range s.repo.accounts {
		accounts = append(accounts, &account)
	}
	return accounts, true, nil
}

func newWSHTTPFallbackHandler(t *testing.T, accounts []service.Account, cache *wsHTTPFallbackHandlerCache) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 2
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 2
	repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
	snapshot := service.NewSchedulerSnapshotService(&wsHTTPFallbackSnapshot{repo: repo}, nil, repo, nil, cfg)
	svc := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, cache, cfg, snapshot, nil,
		service.NewBillingService(cfg, nil), nil, nil, nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	h := newOpenAIHandlerForPreviousResponseIDValidation(t, nil)
	h.cfg = cfg
	h.gatewayService = svc
	h.maxAccountSwitches = 2
	return h
}

func wsHTTPFallbackTestAccount(id int64, url string, websocket bool) service.Account {
	mode := service.OpenAIWSIngressModeOff
	if websocket {
		mode = service.OpenAIWSIngressModePassthrough
	}
	return service.Account{
		ID: id, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: int(id), GroupIDs: []int64{2},
		Credentials: map[string]any{"api_key": "mock-key", "base_url": url},
		Extra: map[string]any{
			"openai_apikey_responses_websockets_v2_enabled": websocket,
			"openai_apikey_responses_websockets_v2_mode":    mode,
		},
	}
}

func dialWSHTTPFallback(t *testing.T, server *httptest.Server, thread string) (*coderws.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", &coderws.DialOptions{
		HTTPHeader: http.Header{"Session-Id": {"session-a"}, "Thread-Id": {thread}},
	})
}

func TestOpenAIWSHTTPFallbackRejectsHTTPOnlyBeforeUpgrade(t *testing.T) {
	cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool)}
	h := newWSHTTPFallbackHandler(t, []service.Account{wsHTTPFallbackTestAccount(1, "http://127.0.0.1:1", false)}, cache)
	server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer server.Close()
	conn, response, err := dialWSHTTPFallback(t, server, "thread-a")
	require.Error(t, err)
	require.Nil(t, conn)
	require.Equal(t, http.StatusUpgradeRequired, response.StatusCode)
	require.Empty(t, cache.marked, "static HTTP-only admission needs no session state")
}

func TestOpenAIWSHTTPFallbackAfterHandshakeFailure(t *testing.T) {
	for _, status := range []int{426, 401, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.WriteHeader(status)
			}))
			defer upstream.Close()
			cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool)}
			h := newWSHTTPFallbackHandler(t, []service.Account{wsHTTPFallbackTestAccount(1, upstream.URL, true)}, cache)
			server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
			defer server.Close()
			conn, _, err := dialWSHTTPFallback(t, server, "thread-a")
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-sol"}`)))
			_, _, err = conn.Read(ctx)
			require.Error(t, err)
			_ = conn.CloseNow()
			require.Equal(t, int32(1), attempts.Load())
			next, response, err := dialWSHTTPFallback(t, server, "thread-a")
			if status == 426 {
				require.Error(t, err)
				require.Equal(t, 426, response.StatusCode)
			} else {
				require.NoError(t, err)
				_ = next.CloseNow()
			}
			other, _, err := dialWSHTTPFallback(t, server, "thread-b")
			require.NoError(t, err, "another thread must still be allowed to use WS")
			_ = other.CloseNow()
		})
	}
}

func TestOpenAIWSHTTPFallbackTriesRemainingProvider(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(426) }))
	defer first.Close()
	received := make(chan struct{}, 1)
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
		received <- struct{}{}
		_ = conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.created","response":{"id":"resp_mock"}}`))
		_, _, _ = conn.Read(ctx)
	}))
	defer second.Close()
	cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool)}
	h := newWSHTTPFallbackHandler(t, []service.Account{wsHTTPFallbackTestAccount(1, first.URL, true), wsHTTPFallbackTestAccount(2, second.URL, true)}, cache)
	server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer server.Close()
	conn, _, err := dialWSHTTPFallback(t, server, "thread-a")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-sol"}`)))
	select {
	case <-received:
	case <-ctx.Done():
		t.Fatal("remaining provider was not attempted")
	}
	cache.mu.Lock()
	require.Empty(t, cache.marked)
	cache.mu.Unlock()
}

func TestOpenAIWSHTTPFallbackCacheFailureDoesNotDowngrade(t *testing.T) {
	cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool), err: errors.New("cache unavailable")}
	h := newWSHTTPFallbackHandler(t, []service.Account{wsHTTPFallbackTestAccount(1, "http://127.0.0.1:1", true)}, cache)
	server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer server.Close()
	conn, _, err := dialWSHTTPFallback(t, server, "thread-a")
	require.NoError(t, err)
	_ = conn.CloseNow()
}

func TestOpenAIWSHTTPFallbackOfficialHeaderOnly(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching body", true: "mismatched body"}[mismatch], func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(426) }))
			defer upstream.Close()
			cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool)}
			h := newWSHTTPFallbackHandler(t, []service.Account{wsHTTPFallbackTestAccount(1, upstream.URL, true)}, cache)
			server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
			defer server.Close()
			headers := http.Header{
				"X-Client-Request-Id": {"thread-a"},
				"User-Agent":          {"codex_cli_rs/0.154.0"}, "Originator": {"codex_cli_rs"},
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			url := "ws" + strings.TrimPrefix(server.URL, "http") + "/openai/v1/responses"
			conn, _, err := coderws.Dial(ctx, url, &coderws.DialOptions{HTTPHeader: headers})
			require.NoError(t, err)
			thread := "thread-a"
			if mismatch {
				thread = "thread-b"
			}
			payload := `{"type":"response.create","model":"gpt-5.6-sol","client_metadata":{"session_id":"session-a","thread_id":"` + thread + `"}}`
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(payload)))
			_, _, err = conn.Read(ctx)
			require.Error(t, err)
			_ = conn.CloseNow()
			next, response, err := coderws.Dial(ctx, url, &coderws.DialOptions{HTTPHeader: headers})
			if mismatch {
				require.NoError(t, err)
				_ = next.CloseNow()
			} else {
				require.Error(t, err)
				require.Equal(t, 426, response.StatusCode)
			}
		})
	}
}

func TestOpenAIWSHTTPFallbackAetherProof(t *testing.T) {
	for _, tc := range []struct {
		name, disposition, writeState string
		fallback                      bool
	}{
		{"empty WS candidates", "exclude", "not_started", true},
		{"retain is not exhausted", "retain", "not_started", false},
		{"unknown execution", "exclude", "unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-aether-ws-control", "route-v1")
				w.Header().Set("x-aether-ws-capabilities", "close-after-terminal,client-reconnect")
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = conn.CloseNow() }()
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				_, request, err := conn.Read(ctx)
				if err != nil {
					return
				}
				fence := gjson.GetBytes(request, "client_metadata.aether\\.sub2api_step_control")
				payload, err := json.Marshal(map[string]any{
					"type": "aether.route_control", "version": 1, "action": "client_reconnect",
					"control_id": "control-a", "scope": "current_step", "effective_after": "immediate",
					"reason": "candidate_unavailable", "recommended_action": "client_reconnect",
					"sub2api_step_correlation_id": fence.Get("sub2api_step_correlation_id").String(),
					"sub2api_binding_epoch_id":    fence.Get("sub2api_binding_epoch_id").String(),
					"sub2api_binding_generation":  fence.Get("sub2api_binding_generation").Uint(),
					"aether_step_id":              "step-a", "aether_attempt_id": "attempt-a",
					"current_attempt_state": "rejected_before_execution", "provider_write_state": tc.writeState,
					"provider_execution_disposition": "proven_not_executed",
					"adapter_proof_class":            service.AetherWSAdapterProofClassCodexOfficialNotExecuted,
					"adapter_proof_version":          1, "middle_route_disposition": tc.disposition,
					"retry_after_ms": 0, "provider_fallback_used": false,
				})
				if err != nil {
					return
				}
				_ = conn.Write(ctx, coderws.MessageText, payload)
				_, _, _ = conn.Read(ctx)
			}))
			defer upstream.Close()
			account := wsHTTPFallbackTestAccount(1, upstream.URL, true)
			account.Extra[service.AetherWSAccountExtraKey] = map[string]any{
				"schema_version": float64(service.AetherWSAccountSchemaVersion),
				"enabled":        true, "required_control_protocol": "route-v1",
			}
			cache := &wsHTTPFallbackHandlerCache{marked: make(map[string]bool)}
			h := newWSHTTPFallbackHandler(t, []service.Account{account}, cache)
			h.cfg.Gateway.OpenAIWS.AetherRouteControlEnabled = true
			server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
			defer server.Close()
			conn, _, err := dialWSHTTPFallback(t, server, "thread-a")
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.6-sol"}`)))
			_, _, err = conn.Read(ctx)
			require.Error(t, err)
			_ = conn.CloseNow()
			next, response, err := dialWSHTTPFallback(t, server, "thread-a")
			if tc.fallback {
				require.Error(t, err)
				require.Equal(t, 426, response.StatusCode)
			} else {
				require.NoError(t, err)
				_ = next.CloseNow()
			}
		})
	}
}
