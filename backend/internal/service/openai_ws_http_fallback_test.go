package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSHTTPFallbackKeyIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "session-a")
	c.Request.Header.Set("thread-id", "thread-a")
	group := int64(1)
	key := ResolveOpenAIWSHTTPFallbackKey(c, &group, 2, 3)
	require.NotEmpty(t, key)
	require.NotContains(t, key, "session-a")
	require.NotEqual(t, key, ResolveOpenAIWSHTTPFallbackKey(c, nil, 2, 3))
	require.NotEqual(t, key, ResolveOpenAIWSHTTPFallbackKey(c, &group, 4, 3))
	require.NotEqual(t, key, ResolveOpenAIWSHTTPFallbackKey(c, &group, 2, 4))
	c.Request.Header.Set("thread-id", "thread-b")
	require.NotEqual(t, key, ResolveOpenAIWSHTTPFallbackKey(c, &group, 2, 3))
	c.Request.Header.Add("thread-id", "thread-a")
	require.Empty(t, ResolveOpenAIWSHTTPFallbackKey(c, &group, 2, 3))
}

func TestOpenAIWSHTTPFallbackKeyOfficialHeaderOnly(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("x-client-request-id", "thread-a")
	require.Empty(t, ResolveOpenAIWSHTTPFallbackKey(c, nil, 2, 3))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.154.0")
	c.Request.Header.Set("originator", "codex_cli_rs")
	key := ResolveOpenAIWSHTTPFallbackKey(c, nil, 2, 3)
	require.NotEmpty(t, key)
	require.Empty(t, ResolveOpenAIWSHTTPFallbackKey(c, nil, 0, 3))
	c.Request.Header.Set("thread-id", "different-thread")
	require.Empty(t, ResolveOpenAIWSHTTPFallbackKey(c, nil, 2, 3))
	c.Request.Header.Del("thread-id")
	c.Request.Header.Add("x-client-request-id", "thread-a")
	require.Empty(t, ResolveOpenAIWSHTTPFallbackKey(c, nil, 2, 3))
}

func TestOpenAIWSHTTPFallbackAvailable(t *testing.T) {
	httpAccount := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}
	wsAccount := httpAccount
	wsAccount.ID = 2
	wsAccount.Extra = map[string]any{"openai_apikey_responses_websockets_v2_enabled": true}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	for _, tc := range []struct {
		name     string
		accounts []Account
		excluded map[int64]struct{}
		want     bool
	}{
		{"empty catalog is not HTTP availability", nil, nil, false},
		{"HTTP only", []Account{httpAccount}, nil, true},
		{"remaining WS", []Account{httpAccount, wsAccount}, nil, false},
		{"known unavailable WS retains HTTP", []Account{wsAccount}, map[int64]struct{}{2: {}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: tc.accounts}}
			available, err := svc.OpenAIWSHTTPFallbackAvailable(context.Background(), OpenAIAccountScheduleRequest{
				Platform: PlatformOpenAI, RequestedModel: "gpt-5.6-sol", ExcludedIDs: tc.excluded,
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, available)
		})
	}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: &fallbackErrorAccountRepo{err: errors.New("catalog unavailable")}}
	available, err := svc.OpenAIWSHTTPFallbackAvailable(context.Background(), OpenAIAccountScheduleRequest{Platform: PlatformOpenAI})
	require.False(t, available)
	require.ErrorContains(t, err, "catalog unavailable")
}

type fallbackErrorAccountRepo struct {
	AccountRepository
	err error
}

func (r *fallbackErrorAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]Account, error) {
	return nil, r.err
}

type fallbackTestCache struct {
	GatewayCache
	key string
	ttl time.Duration
	err error
}

func (c *fallbackTestCache) MarkOpenAIWSHTTPFallback(_ context.Context, key string, ttl time.Duration) error {
	c.key, c.ttl = key, ttl
	return c.err
}

func (c *fallbackTestCache) OpenAIWSHTTPFallbackRequired(_ context.Context, key string) (bool, error) {
	return key == c.key, c.err
}

func TestOpenAIWSHTTPFallbackCache(t *testing.T) {
	cache := &fallbackTestCache{}
	svc := &OpenAIGatewayService{cache: cache}
	require.Error(t, svc.MarkOpenAIWSHTTPFallback(context.Background(), ""))
	require.NoError(t, svc.MarkOpenAIWSHTTPFallback(context.Background(), "thread-a"))
	require.Equal(t, 30*time.Second, cache.ttl)
	marked, err := svc.OpenAIWSHTTPFallbackRequired(context.Background(), "thread-a")
	require.NoError(t, err)
	require.True(t, marked)
	cache.err = errors.New("cache unavailable")
	require.Error(t, svc.MarkOpenAIWSHTTPFallback(context.Background(), "thread-a"))
	_, err = svc.OpenAIWSHTTPFallbackRequired(context.Background(), "thread-a")
	require.Error(t, err)
	svc.cache = nil
	require.Error(t, svc.MarkOpenAIWSHTTPFallback(context.Background(), "thread-a"))
}

func TestAetherWSHTTPFallbackRequiresValidatedUnavailableProof(t *testing.T) {
	for _, tc := range []struct {
		reason      string
		disposition OpenAIWSMiddleRouteDisposition
		want        bool
	}{
		{"candidate_unavailable", OpenAIWSMiddleRouteDispositionExclude, true},
		{"candidate_unavailable", OpenAIWSMiddleRouteDispositionRetain, false},
		{"provider_rate_limited", OpenAIWSMiddleRouteDispositionExclude, false},
		{"account_catalog_changed_during_selection", OpenAIWSMiddleRouteDispositionRetain, false},
	} {
		t.Run(tc.reason+string(tc.disposition), func(t *testing.T) {
			consumer := newAetherWSRouteControlTestConsumer(t, false)
			_, fence := prepareAetherWSRouteControlTestStep(t, consumer)
			frame := aetherWSRouteControlTestFrame(fence, aetherWSRouteActionClientReconnect)
			frame.Reason = tc.reason
			frame.MiddleRouteDisposition = &tc.disposition
			_, decision, err := consumer.consumeUpstreamFrame(marshalAetherWSRouteControlTestFrame(t, frame))
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.WebSocketUnavailable)
			directive := buildAetherWSReconnectDirective(nil, decision, nil)
			var failover *OpenAIWSInitialStepFailoverError
			require.ErrorAs(t, directive.Err, &failover)
			require.Equal(t, tc.want, OpenAIWSFailoverRequiresHTTP(failover.FailoverError()))
		})
	}
	for _, status := range []int{401, 403, 429, 500, 502, 503} {
		require.False(t, OpenAIWSFailoverRequiresHTTP(&UpstreamFailoverError{StatusCode: status}))
	}
}

func TestAetherWSHTTPFallbackLaterTurnKeepsAdmissionFence(t *testing.T) {
	for _, denied := range []bool{false, true} {
		decision := aetherWSRouteControlDecision{
			SignalReconnect: true, WebSocketUnavailable: true,
			MiddleRouteDisposition: OpenAIWSMiddleRouteDispositionExclude,
			ControlID:              "control-a", BindingGeneration: 2,
		}
		called := false
		directive := buildAetherWSReconnectDirective(&OpenAIWSIngressHooks{
			BeforeReconnectSignal: func(control OpenAIWSReconnectControl) error {
				called = true
				require.True(t, control.WebSocketUnavailable)
				require.Equal(t, uint64(2), control.BindingGeneration)
				if denied {
					return errors.New("migration denied")
				}
				return nil
			},
		}, decision, nil)
		require.True(t, called)
		var initial *OpenAIWSInitialStepFailoverError
		require.False(t, errors.As(directive.Err, &initial), "later turns must never replay the first frame")
		if denied {
			require.Empty(t, directive.ClientPayload)
			require.False(t, directive.Exit)
		} else {
			require.NotEmpty(t, directive.ClientPayload)
			require.True(t, directive.Exit)
		}
	}
}
