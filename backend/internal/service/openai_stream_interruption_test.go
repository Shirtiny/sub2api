package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestStreamRetryInterruptionAfterHeartbeatUsesSameBudget(t *testing.T) {
	for _, firstOverload := range []bool{false, true} {
		c, rec, account := streamRetryContext()
		calls := 0
		_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			c.Header("Content-Type", "text/event-stream")
			writeRetrySSE(c.Writer, streamRetryCreated)
			_, _ = c.Writer.WriteString(":\n\n")
			c.Writer.Flush()
			if (calls == 1) == firstOverload {
				return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
			}
			return nil, errors.New("stream usage incomplete: missing terminal event")
		})
		var final *UpstreamFailoverError
		require.ErrorAs(t, err, &final)
		require.True(t, final.StopLocalRetry)
		require.True(t, final.ResponseUncommitted)
		require.Equal(t, 2, calls)
		require.NotContains(t, rec.Body.String(), "resp-failed")
		require.NotContains(t, rec.Body.String(), "response.completed")
		require.Contains(t, rec.Body.String(), ":\n\n")
	}
}

func TestStreamRetryInterruptionRecoversPlainReadFailures(t *testing.T) {
	for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF, fmt.Errorf("stream read error: %w", io.ErrUnexpectedEOF), errors.New("stream usage incomplete: missing terminal event"), errors.New("stream data interval timeout")} {
		c, rec, account := streamRetryContext()
		calls := 0
		result, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			c.Header("Content-Type", "text/event-stream")
			if calls == 1 {
				writeRetrySSE(c.Writer, streamRetryCreated)
				_, _ = c.Writer.WriteString(":\n\n")
				return nil, cause
			}
			writeRetrySSE(c.Writer, streamRetryDelta)
			return &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 1}}, nil
		})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Equal(t, 1, result.Usage.OutputTokens)
		require.Contains(t, rec.Body.String(), "hello")
		require.NotContains(t, rec.Body.String(), "resp-failed")
	}
}

func TestStreamRetryInterruptionClassifiesAetherNativeErrorsButNotBusinessErrors(t *testing.T) {
	for _, payload := range []string{
		`{"error":{"type":"stream_missing_terminal_event","code":502,"message":"execution runtime stream ended before provider terminal event"}}`,
		`{"type":"response.failed","response":{"status":"failed","error":{"type":"stream_missing_terminal_event","code":"stream_missing_terminal_event","message":"execution runtime stream ended before provider terminal event"}}}`,
		`{"error":{"code":"stream_read_error","message":"stream_read_error"}}`,
		`{"type":"error","error":{"type":"api_error","code":"stream_missing_terminal_event","message":"execution runtime stream ended before provider terminal event"}}`,
	} {
		require.NotNil(t, openAIStreamRetryableError(200, []byte(payload)), payload)
	}
	for _, payload := range []string{
		`{"error":{"type":"authentication_error","code":"stream_read_error","message":"stream_read_error"}}`,
		`{"error":{"code":401,"message":"OpenAI stream ended before a terminal event"}}`,
		`{"error":{"type":"content_policy_violation","message":"stream_read_error"}}`,
		`{"type":"response.output_text.delta","delta":"stream_read_error"}`,
		`{"error":{"code":"invalid_request_error","message":"stream_read_error"}}`,
	} {
		require.Nil(t, openAIStreamRetryableError(200, []byte(payload)), payload)
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("invalid API key"), errors.New("JSON decode failed")} {
		require.Nil(t, openAIStreamReadInterruption(cause, nil))
	}
}

func TestStreamRetryInterruptionPreservesGatewayTimeout(t *testing.T) {
	for _, payload := range []string{
		`{"error":{"type":"stream_idle_timeout","code":504,"message":"provider stream produced no upstream frame for 300000 ms"}}`,
		`{"type":"response.failed","response":{"error":{"code":"stream_progress_timeout","message":"provider stream produced no client-visible data for 300000 ms"}}}`,
	} {
		failure := openAIStreamRetryableError(200, []byte(payload))
		require.NotNil(t, failure)
		require.Equal(t, http.StatusGatewayTimeout, failure.StatusCode)
	}
}

func TestStreamRetryInterruptionNeverReplaysContentToolsOrAmbiguousTail(t *testing.T) {
	for _, wire := range []string{"data: " + streamRetryCreated + "\n\ndata: " + streamRetryDelta + "\n\n", "data: " + streamRetryDelta + "\n\n", `data: {"type":"response.output_text.delta","delta":"partial`, "data: {\"type\":\"unknown\"}\n\n"} {
		c, _, account := streamRetryContext()
		calls := 0
		_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.WriteString(wire)
			return nil, errors.New("stream usage incomplete: missing terminal event")
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
	}
	c, _, account := streamRetryContext()
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		observeOpenAIStreamRetrySource(c, []byte(`{"type":"response.output_item.added","item":{"type":"web_search_call"}}`))
		return nil, newOpenAIStreamInterruption("OpenAI stream ended before a terminal event")
	})
	var final *UpstreamFailoverError
	require.ErrorAs(t, err, &final)
	require.True(t, final.StopLocalRetry, "outer handler cannot replay unforwarded tool activity")
	require.Equal(t, 1, calls)
}

func TestStreamRetryInterruptionCannotHideAnUnforwardedBusinessError(t *testing.T) {
	for _, payload := range []string{
		`{"type":"error","error":{"code":"invalid_api_key","message":"denied"}}`,
		`{"type":"response.failed","response":{"error":{"code":403,"message":"forbidden"}}}`,
		`{"type":"error","error":{"type":"content_policy_violation","message":"blocked"}}`,
	} {
		c, _, account := streamRetryContext()
		calls := 0
		_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			observeOpenAIStreamRetrySource(c, []byte(payload))
			return nil, errors.New("stream usage incomplete: missing terminal event")
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
	}
}

func interruptionWire(mode string, success bool) string {
	if !success {
		switch mode {
		case "raw-chat":
			return "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n"
		case "raw-messages":
			return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"resp-failed\",\"content\":[]}}\n\n"
		default:
			return "data: " + streamRetryCreated + "\n\n"
		}
	}
	switch mode {
	case "raw-chat":
		return "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	case "raw-messages":
		return "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	default:
		return "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-good\",\"output\":[]}}\n\ndata: " + streamRetryDelta + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-good\",\"status\":\"completed\",\"output\":[]}}\n\n"
	}
}

func aetherInterruptionWire(mode string) string {
	const message = "execution runtime stream ended before provider terminal event"
	switch mode {
	case "raw-chat":
		return fmt.Sprintf("data: {\"error\":{\"type\":\"stream_missing_terminal_event\",\"code\":502,\"message\":%q}}\n\ndata: [DONE]\n\n", message)
	case "raw-messages":
		return fmt.Sprintf("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"api_error\",\"code\":\"stream_missing_terminal_event\",\"message\":%q}}\n\n", message)
	default:
		return fmt.Sprintf("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"stream_missing_terminal_event\",\"code\":\"stream_missing_terminal_event\",\"message\":%q}}}\n\n", message)
	}
}

func TestStreamRetryInterruptionAllResponseProcessors(t *testing.T) {
	for _, mode := range []string{"responses", "responses-passthrough", "chat", "messages", "raw-chat", "raw-messages"} {
		for _, nativeError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/native=%v", mode, nativeError), func(t *testing.T) {
				c, rec, account := streamRetryContext()
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				calls := 0
				_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
					calls++
					c.Header("Content-Type", "text/event-stream")
					_, _ = c.Writer.WriteString(":\n\n")
					c.Writer.Flush()
					wire := interruptionWire(mode, calls > 1)
					if calls == 1 && nativeError {
						wire += aetherInterruptionWire(mode)
					}
					resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
					switch mode {
					case "responses":
						r, e := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model")
						if r == nil {
							return nil, e
						}
						return &OpenAIForwardResult{Usage: *r.usage}, e
					case "responses-passthrough":
						r, e := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "model", "model")
						if r == nil {
							return nil, e
						}
						return &OpenAIForwardResult{Usage: *r.usage}, e
					case "chat":
						return svc.handleChatStreamingResponse(resp, c, account, "model", "model", "model", time.Now(), 10)
					case "messages":
						return svc.handleAnthropicStreamingResponse(resp, c, account, "model", "model", "model", time.Now())
					case "raw-chat":
						return svc.streamRawChatCompletions(c, resp, account, "model", "model", "model", nil, nil, time.Now(), 10)
					default:
						r, e := svc.handleOpenAIMessagesPassthroughStreamingResponse(context.Background(), resp, c, account, time.Now())
						if r == nil {
							return nil, e
						}
						return &OpenAIForwardResult{Usage: *r.usage}, e
					}
				})
				require.NoError(t, err)
				require.Equal(t, 2, calls)
				require.NotContains(t, rec.Body.String(), "resp-failed")
				require.Contains(t, rec.Body.String(), "hello")
				require.NotContains(t, rec.Body.String(), "stream_missing_terminal_event")
			})
		}
	}
}

func TestStreamRetryInterruptionLoopbackHTTPMissingTypeThenEOF(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil // mimic the older hop's absent media type
		if hits.Add(1) == 1 {
			writeRetrySSE(w, streamRetryCreated)
			writeRetrySSE(w, `{"type":"response.aether_keepalive"}`)
			_ = http.NewResponseController(w).Flush()
			return
		}
		_, _ = io.WriteString(w, interruptionWire("responses", true))
	}))
	defer server.Close()
	upstream := &streamRetryHTTPClient{client: server.Client(), target: server.URL}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, rec, account := streamRetryContext()
	account.Type = AccountTypeOAuth
	account.Credentials = map[string]any{"access_token": "local-mock-token", "chatgpt_account_id": "local-account"}
	account.Extra = map[string]any{"openai_passthrough": true}
	account.Status = StatusActive
	account.Schedulable = true
	body := []byte(`{"model":"gpt-5.4","stream":true,"instructions":"local test","input":[{"role":"user","content":"hello"}]}`)
	c.Request.Header.Set("session_id", "session-preserved")
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
	c.Request.Header.Set("Content-Type", "application/json")
	_, err := ForwardWithStreamRetry(c.Request.Context(), c, account, true, func() (*OpenAIForwardResult, error) { return svc.Forward(c.Request.Context(), c, account, body) })
	require.NoError(t, err)
	require.Equal(t, int32(2), hits.Load())
	require.Equal(t, upstream.bodies[0], upstream.bodies[1])
	require.Equal(t, upstream.headers[0], upstream.headers[1])
	require.Contains(t, rec.Body.String(), "hello")
	require.NotContains(t, rec.Body.String(), "resp-failed")
}
