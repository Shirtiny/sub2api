package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const streamRetryMessage = "Our servers are currently overloaded. Please try again later."
const streamRetryError = `{"type":"response.failed","response":{"id":"resp-failed","status":"failed","error":{"code":"503","message":"Our servers are currently overloaded. Please try again later."}}}`
const streamRetryCreated = `{"type":"response.created","response":{"id":"resp-failed","status":"in_progress","output":[]}}`
const streamRetryDelta = `{"type":"response.output_text.delta","delta":"hello"}`

func streamRetryContext() (*gin.Context, *httptest.ResponseRecorder, *Account) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"stream":true}`))
	return c, rec, &Account{ID: 17, Platform: PlatformOpenAI}
}

func writeRetrySSE(w io.Writer, payload string) { _, _ = fmt.Fprintf(w, "data: %s\n\n", payload) }

func TestStreamRetryOpeningClassifier(t *testing.T) {
	for _, payload := range []string{streamRetryCreated,
		`{"type":"response.queued","response":{"output":[]}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`,
		`{"type":"response.output_item.added","item":{"type":"message","content":[]}}`,
		`{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`,
		`{"type":"message_start","message":{"id":"hidden","content":[]}}`,
		`{"type":"content_block_start","content_block":{"type":"thinking","thinking":""}}`,
		`{"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
	} {
		require.Equal(t, streamOpeningPending, classifyOpenAIStreamOpening([]byte(payload), ""), payload)
	}
	for _, payload := range []string{streamRetryDelta,
		`{"type":"response.reasoning_text.delta","delta":"thinking"}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","arguments":""}}`,
		`{"type":"response.output_item.added","item":{"type":"web_search_call"}}`,
		`{"type":"response.output_item.added","item":{"type":"image_generation_call"}}`,
		`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"text":"thought"}]}}`,
		`{"type":"content_block_start","content_block":{"type":"tool_use","input":{}}}`,
		`{"choices":[{"delta":{"content":" "}}]}`,
		`{"choices":[{"delta":{"reasoning_content":"think"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{}]}}]}`,
		`{"type":"unknown"}`, `invalid json`,
	} {
		require.Equal(t, streamOpeningContent, classifyOpenAIStreamOpening([]byte(payload), ""), payload)
	}
}

func TestStreamRetryErrorClassification(t *testing.T) {
	for _, payload := range []string{streamRetryError,
		`{"error":{"type":"overloaded_error","message":"overloaded"}}`,
		`{"type":"error","error":{"code":503,"message":"server overloaded"}}`,
	} {
		require.NotNil(t, openAIOverloadError(200, []byte(payload)), payload)
	}
	for _, status := range []int{400, 401, 403, 429} {
		require.Nil(t, openAIOverloadError(status, []byte(streamRetryError)))
	}
	for _, payload := range []string{
		`{"error":{"code":401,"message":"Our servers are currently overloaded. Please try again later."}}`,
		`{"error":{"code":503,"type":"authentication_error","message":"Our servers are currently overloaded. Please try again later."}}`,
		`{"error":{"code":"invalid_api_key","message":"Our servers are currently overloaded. Please try again later."}}`,
		`{"error":{"code":503,"message":"content policy violation: overloaded"}}`,
		`{"error":{"code":503,"type":"invalid_request_error","message":"overloaded"}}`,
		`{"type":"response.output_text.delta","delta":"Our servers are currently overloaded. Please try again later."}`,
		`{"type":"response.output_text.delta","error":{"type":"overloaded_error","message":"overloaded"}}`,
	} {
		require.Nil(t, openAIOverloadError(200, []byte(payload)), payload)
	}
}

func TestStreamRetryOpeningHeartbeatDoesNotFlushPreamble(t *testing.T) {
	for _, heartbeat := range []string{":\n\n", "event: ping\ndata: {\"type\":\"ping\"}\n\n", "event: response.aether_keepalive\ndata: {}\n\n"} {
		c, rec, _ := streamRetryContext()
		w := &openAIStreamOpeningWriter{ResponseWriter: c.Writer, status: 200}
		w.Header().Set("Content-Type", "text/event-stream")
		buffer := bufio.NewWriterSize(w, 4096)
		fmt.Fprintf(buffer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"%s\",\"output\":[]}}\n\n", strings.Repeat("x", 9000))
		require.NoError(t, buffer.Flush())
		w.Flush()
		require.Empty(t, rec.Body.String(), "4 KiB auto-flush cannot publish the opening")
		_, err := buffer.WriteString(heartbeat)
		require.NoError(t, err)
		require.NoError(t, buffer.Flush())
		w.Flush()
		require.Equal(t, ":\n\n", rec.Body.String())
		require.False(t, w.committed)
		require.True(t, w.Written(), "HTTP state remains truthful even before content")
		_, err = fmt.Fprintf(w, "data: %s\n", streamRetryDelta)
		require.NoError(t, err)
		require.True(t, w.committed)
		require.Contains(t, rec.Body.String(), "hello", "do not wait for the trailing blank line")
	}
}

func TestStreamRetryOpeningFragmentedSSE(t *testing.T) {
	wire := "event: response.created\r\ndata: {\"response\":{\"id\":\"中文\",\"output\":[]}}\r\n\r\nevent: error\r\ndata: {\"error\":{\r\ndata: \"code\":503,\"message\":\"Our servers are currently overloaded. Please try again later.\"}}\r\n\r\n"
	for size := 1; size <= len(wire); size++ {
		c, rec, _ := streamRetryContext()
		w := &openAIStreamOpeningWriter{ResponseWriter: c.Writer, status: 200}
		w.Header().Set("Content-Type", "text/event-stream")
		for start := 0; start < len(wire); start += size {
			_, err := w.Write([]byte(wire[start:min(start+size, len(wire))]))
			require.NoError(t, err)
		}
		require.NotNil(t, w.retryFailure, "chunk size %d", size)
		require.False(t, w.committed)
		require.Empty(t, rec.Body.String())
	}
}

func TestStreamRetryOpeningBoundsAndUnsafeEventsCommit(t *testing.T) {
	for _, wire := range []string{"id: upstream-id\ndata: {}\n\n", "data: broken\n\n", strings.Repeat("x", openAIStreamOpeningLimit+1), "data: " + streamRetryDelta + "\n\n"} {
		c, rec, _ := streamRetryContext()
		w := &openAIStreamOpeningWriter{ResponseWriter: c.Writer, status: 200}
		w.Header().Set("Content-Type", "text/event-stream")
		_, err := w.Write([]byte(wire))
		require.NoError(t, err)
		require.True(t, w.committed)
		require.Equal(t, wire, rec.Body.String())
		writeRetrySSE(w, streamRetryError)
		require.Contains(t, rec.Body.String(), streamRetryMessage, "committed errors must not be hidden")
	}
}

func TestStreamRetryOneRescueOnlyAndNoFailedOutputOrUsage(t *testing.T) {
	c, rec, account := streamRetryContext()
	calls := 0
	result, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		c.Header("Content-Type", "text/event-stream")
		if calls == 1 {
			writeRetrySSE(c.Writer, streamRetryCreated)
			c.Writer.Flush()
			_, _ = c.Writer.WriteString(":\n\n")
			c.Writer.Flush()
			writeRetrySSE(c.Writer, streamRetryError)
			return &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 999}}, errors.New("upstream response failed")
		}
		writeRetrySSE(c.Writer, streamRetryDelta)
		c.Writer.Flush()
		return &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 1}}, nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, 1, result.Usage.OutputTokens)
	require.Contains(t, rec.Body.String(), "hello")
	require.NotContains(t, rec.Body.String(), "resp-failed")
	require.NotContains(t, rec.Body.String(), streamRetryMessage)
	require.GreaterOrEqual(t, *result.FirstTokenMs, 500)
	require.GreaterOrEqual(t, result.Duration, 500*time.Millisecond)
	// The retry budget belongs to the inbound request, not to an account or
	// an outer candidate. Calling again cannot add two more network attempts.
	calls = 0
	_, err = ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError), RetryableOnSameAccount: true}
	})
	var exhausted *UpstreamFailoverError
	require.ErrorAs(t, err, &exhausted)
	require.True(t, exhausted.StopLocalRetry)
	require.True(t, exhausted.ResponseUncommitted)
	require.Equal(t, 1, calls)
}

func TestStreamRetryPreservesHTTPErrorStatusUntilExhaustion(t *testing.T) {
	c, rec, account := streamRetryContext()
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		c.JSON(503, gin.H{"error": gin.H{"message": streamRetryMessage}})
		return nil, errors.New("HTTP 503")
	})
	var exhausted *UpstreamFailoverError
	require.ErrorAs(t, err, &exhausted)
	require.True(t, exhausted.StopLocalRetry)
	require.Equal(t, 2, calls)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String(), "handler owns the one final error")
}

func TestStreamRetryDoesNotReplayAfterTextReasoningOrTools(t *testing.T) {
	for _, payload := range []string{streamRetryDelta, `{"type":"response.reasoning_text.delta","delta":"thinking"}`, `{"type":"response.output_item.added","item":{"type":"function_call","arguments":""}}`, `{"type":"content_block_start","content_block":{"type":"tool_use","input":{}}}`} {
		c, rec, account := streamRetryContext()
		calls := 0
		_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			c.Header("Content-Type", "text/event-stream")
			writeRetrySSE(c.Writer, payload)
			writeRetrySSE(c.Writer, streamRetryError)
			return nil, errors.New("upstream response failed")
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Contains(t, rec.Body.String(), streamRetryMessage)
	}
}

func TestStreamRetryDoesNotReplayUnforwardedUpstreamToolActivity(t *testing.T) {
	c, _, account := streamRetryContext()
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		observeOpenAIStreamRetrySource(c, []byte(`{"type":"response.output_item.added","item":{"type":"web_search_call"}}`))
		return nil, captureOpenAIStreamRetryableError(c, []byte(streamRetryError))
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestStreamRetryCancellationAndRetryAfter(t *testing.T) {
	for _, value := range []string{"6", "900", "tomorrow", "-1"} {
		_, ok := openAIStreamOverloadRetryDelay(value)
		require.False(t, ok)
	}
	d, ok := openAIStreamOverloadRetryDelay("2")
	require.True(t, ok)
	require.Equal(t, 2*time.Second, d)
	c, _, account := streamRetryContext()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		cancel()
		return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
	c, _, account = streamRetryContext()
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	calls = 0
	_, err = ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, calls)
}

func TestStreamRetryNonOverloadAndNonStreamingKeepExistingBehavior(t *testing.T) {
	for _, stream := range []bool{false, true} {
		c, rec, account := streamRetryContext()
		calls := 0
		_, err := ForwardWithStreamRetry(context.Background(), c, account, stream, func() (*OpenAIForwardResult, error) {
			calls++
			c.JSON(400, gin.H{"error": gin.H{"message": "invalid input"}})
			return nil, errors.New("bad request")
		})
		require.Error(t, err)
		require.Equal(t, 1, calls)
		require.Equal(t, 400, rec.Code)
		require.Contains(t, rec.Body.String(), "invalid input")
	}
}

func TestStreamRetryGenericFailoverAfterHeartbeatDiscardsOnlyPreamble(t *testing.T) {
	c, rec, account := streamRetryContext()
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		c.Header("Content-Type", "text/event-stream")
		writeRetrySSE(c.Writer, streamRetryCreated)
		_, _ = c.Writer.WriteString(":\n\n")
		c.Writer.Flush()
		return nil, &UpstreamFailoverError{StatusCode: 502, ResponseBody: []byte(`{"error":{"message":"upstream unavailable"}}`)}
	})
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.True(t, failover.ResponseUncommitted)
	require.False(t, failover.StopLocalRetry)
	require.Equal(t, ":\n\n", rec.Body.String())
}

func TestStreamRetryRealResponseProcessors(t *testing.T) {

	for _, mode := range []string{"responses", "responses-passthrough", "chat", "messages", "raw-chat", "raw-messages"} {
		t.Run(mode, func(t *testing.T) {
			c, rec, account := streamRetryContext()
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			calls := 0
			_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
				calls++
				wire := "data: " + streamRetryCreated + "\n\n"
				if calls == 1 {
					wire += "event: error\ndata: {\"error\":{\"code\":503,\"message\":\"" + streamRetryMessage + "\"}}\n\n"
				} else {
					wire = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-good\",\"output\":[]}}\n\n" + "data: " + streamRetryDelta + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-good\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n"
					if mode == "raw-chat" {
						wire = "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"
					}
					if mode == "raw-messages" {
						wire = "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
				}
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}
				switch mode {
				case "responses":
					r, e := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model")
					if r == nil {
						return nil, e
					}
					return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
				case "responses-passthrough":
					r, e := svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "model", "model")
					if r == nil {
						return nil, e
					}
					return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
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
					return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
				}
			})
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.NotContains(t, rec.Body.String(), "resp-failed")
			require.NotContains(t, rec.Body.String(), streamRetryMessage)
			require.Contains(t, rec.Body.String(), "hello")
		})
	}
}

type streamRetryHTTPClient struct {
	client   *http.Client
	target   string
	headers  []http.Header
	bodies   [][]byte
	accounts []int64
}

func (u *streamRetryHTTPClient) Do(req *http.Request, _ string, account int64, _ int) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	u.headers = append(u.headers, req.Header.Clone())
	u.bodies = append(u.bodies, body)
	u.accounts = append(u.accounts, account)
	// Test-only transport: every request goes to the loopback mock regardless
	// of the prepared upstream URL. No real provider traffic is possible.
	local, err := http.NewRequestWithContext(req.Context(), req.Method, u.target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	local.Header = req.Header.Clone()
	if local.URL.Scheme != "http" || local.URL.Hostname() != "127.0.0.1" {
		return nil, errors.New("test transport requires loopback HTTP")
	}
	return u.client.Do(local) // #nosec G704 -- Test-only transport, target is explicitly restricted to loopback above.
}
func (u *streamRetryHTTPClient) DoWithTLS(req *http.Request, proxy string, account int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, account, concurrency)
}

func TestStreamRetryForwardHTTPPreservesRequestAndRetriesAfterHeartbeat(t *testing.T) {
	for _, firstStatus := range []int{200, 503} {
		t.Run(fmt.Sprint(firstStatus), func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit := hits.Add(1)
				if hit == 1 && firstStatus == 503 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(503)
					_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, streamRetryMessage)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if hit == 1 {
					writeRetrySSE(w, streamRetryCreated)
					writeRetrySSE(w, `{"type":"response.aether_keepalive"}`)
					_ = http.NewResponseController(w).Flush()
					time.Sleep(80 * time.Millisecond)
					writeRetrySSE(w, streamRetryError)
					_ = http.NewResponseController(w).Flush()
					// The forwarder must stop on the error, not wait for upstream
					// EOF before retrying. Closing resp.Body cancels this request.
					select {
					case <-r.Context().Done():
					case <-time.After(4 * time.Second):
					}
					return
				}
				writeRetrySSE(w, `{"type":"response.created","response":{"id":"resp-good","output":[]}}`)
				writeRetrySSE(w, streamRetryDelta)
				writeRetrySSE(w, `{"type":"response.completed","response":{"id":"resp-good","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":1}}}`)
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
			started := time.Now()
			result, err := ForwardWithStreamRetry(c.Request.Context(), c, account, true, func() (*OpenAIForwardResult, error) { return svc.Forward(c.Request.Context(), c, account, body) })
			require.NoError(t, err)
			require.Less(t, time.Since(started), 3*time.Second, "retry must not wait for the failed upstream's EOF")
			require.Len(t, upstream.bodies, 2)
			require.Equal(t, upstream.bodies[0], upstream.bodies[1])
			require.Equal(t, upstream.headers[0], upstream.headers[1])
			require.Equal(t, []int64{account.ID, account.ID}, upstream.accounts)
			require.Equal(t, int64(1), int64(result.Usage.OutputTokens))
			require.NotContains(t, rec.Body.String(), "resp-failed")
			require.NotContains(t, rec.Body.String(), streamRetryMessage)
			require.Contains(t, rec.Body.String(), "hello")
		})
	}
}

func TestStreamRetryHonorsUpstreamRetryAfterEvenWhenNotForwarded(t *testing.T) {
	c, rec, account := streamRetryContext()
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		rememberOpenAIStreamRetryHeaders(c, http.Header{"Retry-After": []string{"60"}})
		return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
	})
	var exhausted *UpstreamFailoverError
	require.ErrorAs(t, err, &exhausted)
	require.True(t, exhausted.StopLocalRetry)
	require.Equal(t, 1, calls)
	require.Empty(t, rec.Body.String())
}

func TestStreamRetryHTTPFailureAfterHeartbeatDoesNotCorruptSSE(t *testing.T) {
	c, rec, account := streamRetryContext()
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		if calls == 1 {
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.WriteString(":\n\n")
			writeRetrySSE(c.Writer, streamRetryError)
			return nil, errors.New("overloaded")
		}
		c.JSON(401, gin.H{"error": gin.H{"message": "unauthorized"}})
		return nil, errors.New("HTTP 401")
	})
	var final *UpstreamFailoverError
	require.ErrorAs(t, err, &final)
	require.Equal(t, 401, final.StatusCode)
	require.True(t, final.StopLocalRetry)
	require.True(t, final.ResponseUncommitted)
	require.Equal(t, 2, calls)
	require.Equal(t, ":\n\n", rec.Body.String(), "handler must emit the final SSE error, never bare JSON")
}

func TestStreamRetryStopsOnDownstreamWriteFailure(t *testing.T) {
	c, _, account := streamRetryContext()
	c.Writer = &streamRetryFailedWriter{ResponseWriter: c.Writer}
	calls := 0
	_, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		calls++
		c.Header("Content-Type", "text/event-stream")
		writeRetrySSE(c.Writer, streamRetryDelta)
		return nil, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
	})
	require.EqualError(t, err, "client disconnected")
	require.Equal(t, 1, calls)
}

func TestStreamRetryLateOverloadWithRealKeepaliveHasNoContentDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, rec, account := streamRetryContext()
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
		calls := 0
		result, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			var body io.ReadCloser
			if calls == 1 {
				r, w := io.Pipe()
				body = r
				go func() {
					defer func() { _ = w.Close() }()
					// Exercise the service's real 4 KiB buffer and heartbeat timer.
					_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"failed-%s\",\"output\":[]}}\n\n", strings.Repeat("x", 9000))
					time.Sleep(47 * time.Second)
					writeRetrySSE(w, streamRetryError)
				}()
			} else {
				body = io.NopCloser(strings.NewReader("data: " + streamRetryDelta + "\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[]}}\n\n"))
			}
			defer func() { _ = body.Close() }()
			resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body}
			r, e := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "model", "model")
			if r == nil {
				return nil, e
			}
			return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
		})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.Contains(t, rec.Body.String(), ":\n\n")
		require.NotContains(t, rec.Body.String(), "failed-")
		require.Contains(t, rec.Body.String(), "hello")
		require.Equal(t, 47500, *result.FirstTokenMs, "only upstream wait and the one backoff, no content lookahead delay")
	})
}

type streamRetryFailedWriter struct{ gin.ResponseWriter }

func (w *streamRetryFailedWriter) Write([]byte) (int, error) {
	return 0, errors.New("client disconnected")
}
func (w *streamRetryFailedWriter) WriteString(string) (int, error) {
	return 0, errors.New("client disconnected")
}
