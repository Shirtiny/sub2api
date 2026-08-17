package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func ordinaryRemoteCompactionV2SSEFixture() []byte {
	return []byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_bad","status":"in_progress","output":[]}}`,
		"",
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","summary":[{"type":"summary_text","text":"reason"}]}}`,
		"",
		`data: {"type":"response.output_item.done","output_index":1,"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"summary text"}]}}`,
		"",
		`data: {"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","name":"followup_task","arguments":"{}"}}`,
		"",
		`data: {"type":"response.output_item.done","output_index":3,"item":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_bad","object":"response","model":"gpt-5.6-sol","status":"completed","output":[],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
}

func validRemoteCompactionV2SSEFixture() []byte {
	return []byte(strings.Join([]string{
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"cmp_1","type":"compaction","encrypted_content":"ciphertext"}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_ok","object":"response","status":"completed","output":[{"id":"cmp_1","type":"compaction","encrypted_content":"ciphertext"}]}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
}

func TestValidateOpenAICompactionV2ResponseBody(t *testing.T) {
	t.Run("rejects ordinary output items", func(t *testing.T) {
		err := validateOpenAICompactionV2ResponseBody(ordinaryRemoteCompactionV2SSEFixture(), true)
		require.EqualError(t, err, "remote compaction v2 expected exactly one compaction output item, got 0 from 4 output items")
	})

	t.Run("accepts exactly one compaction item", func(t *testing.T) {
		require.NoError(t, validateOpenAICompactionV2ResponseBody(validRemoteCompactionV2SSEFixture(), true))
	})

	t.Run("rejects stream without completion", func(t *testing.T) {
		body := []byte(`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}` + "\n\n")
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.EqualError(t, err, "remote compaction v2 response closed before response.completed")
	})

	t.Run("does not use the SSE event name as a missing payload type", func(t *testing.T) {
		body := []byte(strings.Join([]string{
			`event: response.output_item.done`,
			`data: {"item":{"type":"compaction","encrypted_content":"ciphertext"}}`,
			"",
			`event: response.completed`,
			`data: {"response":{"id":"resp_missing_types"}}`,
			"",
		}, "\n"))
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.ErrorContains(t, err, "event type is required")
	})

	t.Run("rejects completed event without response id", func(t *testing.T) {
		body := []byte(strings.Join([]string{
			`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}`,
			"",
			`data: {"type":"response.completed","response":{"status":"completed"}}`,
			"",
		}, "\n"))
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.ErrorContains(t, err, "response.completed.response.id is required")
	})

	t.Run("rejects completed event with malformed usage", func(t *testing.T) {
		body := []byte(strings.Join([]string{
			`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_bad_usage","usage":{"input_tokens":1,"output_tokens":2}}}`,
			"",
		}, "\n"))
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.ErrorContains(t, err, "must include input_tokens, output_tokens, and total_tokens")
	})

	t.Run("accepts compaction summary alias", func(t *testing.T) {
		body := bytes.ReplaceAll(validRemoteCompactionV2SSEFixture(), []byte(`"type":"compaction"`), []byte(`"type":"compaction_summary"`))
		require.NoError(t, validateOpenAICompactionV2ResponseBody(body, true))
	})

	t.Run("rejects multiple compaction items", func(t *testing.T) {
		body := []byte(strings.Join([]string{
			`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"one"}}`,
			"",
			`data: {"type":"response.output_item.done","item":{"type":"compaction_summary","encrypted_content":"two"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_multiple"}}`,
			"",
		}, "\n"))
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.EqualError(t, err, "remote compaction v2 expected exactly one compaction output item, got 2 from 2 output items")
	})

	t.Run("does not normalize event type whitespace", func(t *testing.T) {
		body := []byte(strings.Join([]string{
			`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}`,
			"",
			`data: {"type":" response.completed ","response":{"id":"resp_padded"}}`,
			"",
		}, "\n"))
		err := validateOpenAICompactionV2ResponseBody(body, true)
		require.EqualError(t, err, "remote compaction v2 response closed before response.completed")
	})

	t.Run("accepts a string response id exactly as Codex deserializes it", func(t *testing.T) {
		body := bytes.Replace(validRemoteCompactionV2SSEFixture(), []byte(`"id":"resp_ok"`), []byte(`"id":""`), 1)
		require.NoError(t, validateOpenAICompactionV2ResponseBody(body, true))
	})
}

func TestReadAndValidateOpenAICompactionV2ResponseRestoresValidBody(t *testing.T) {
	original := validRemoteCompactionV2SSEFixture()
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(original))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	buffered, err := svc.readAndValidateOpenAICompactionV2Response(context.Background(), resp, true)
	require.NoError(t, err)
	require.NotEmpty(t, buffered)
	require.NotContains(t, string(buffered), "[DONE]")
	require.Less(t, len(buffered), len(original))

	restored, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, buffered, restored)
}

func TestReadAndValidateOpenAICompactionV2ResponseStopsAtCompletedWithoutEOF(t *testing.T) {
	reader, writer := io.Pipe()
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   reader,
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	type result struct {
		body []byte
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		body, err := svc.readAndValidateOpenAICompactionV2Response(context.Background(), resp, true)
		resultCh <- result{body: body, err: err}
	}()
	writeDone := make(chan error, 1)
	go func() {
		_, err := writer.Write(validRemoteCompactionV2SSEFixture())
		writeDone <- err
	}()

	select {
	case got := <-resultCh:
		require.NoError(t, got.err)
		require.Contains(t, string(got.body), `"type":"response.completed"`)
		require.NotContains(t, string(got.body), "[DONE]")
	case <-time.After(time.Second):
		t.Fatal("remote compaction reader waited for EOF after response.completed")
	}
	_ = writer.Close()
	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("fixture writer did not unblock")
	}
}

func TestReadAndValidateOpenAICompactionV2ResponseClosesBodyOnCancel(t *testing.T) {
	reader, writer := io.Pipe()
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   reader,
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.readAndValidateOpenAICompactionV2Response(ctx, resp, true)
		resultCh <- err
	}()
	cancel()

	select {
	case err := <-resultCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("remote compaction reader ignored request cancellation")
	}
	_, writeErr := writer.Write([]byte("data: later\n\n"))
	require.Error(t, writeErr)
	_ = writer.Close()
}

func TestOpenAICompactionV2AttemptWriterCommitsAtomically(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Writer.Header().Set("X-Base", "base")
	attempt := newOpenAICompactionV2AttemptWriter(c.Writer, 1024)
	attempt.Header().Set("Content-Type", "text/event-stream")
	attempt.Header().Set("X-Upstream", "selected")
	_, err := attempt.Write(validRemoteCompactionV2SSEFixture())
	require.NoError(t, err)

	require.Empty(t, recorder.Body.Bytes())
	require.Empty(t, recorder.Header().Get("X-Upstream"))
	require.NoError(t, attempt.Commit())
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "base", recorder.Header().Get("X-Base"))
	require.Equal(t, "selected", recorder.Header().Get("X-Upstream"))
	require.Equal(t, validRemoteCompactionV2SSEFixture(), recorder.Body.Bytes())
}

func TestOpenAICompactionV2AttemptWriterKeepaliveDoesNotCommitAttemptBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	attempt := newOpenAICompactionV2AttemptWriter(c.Writer, 1024*1024)
	_, err := attempt.Write(ordinaryRemoteCompactionV2SSEFixture())
	require.NoError(t, err)
	require.NoError(t, attempt.writeKeepalive())

	require.Equal(t, ":\n\n", recorder.Body.String())
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.NotContains(t, recorder.Body.String(), "summary text")
	require.True(t, attempt.ParentWritten())
	// Discarding this attempt requires no cleanup: only the protocol-neutral
	// comment has reached the real writer.
}

func TestOpenAICompactionV2BodyHasTerminalEvent(t *testing.T) {
	require.False(t, openAICompactionV2BodyHasTerminalEvent([]byte(`{"error":"not sse"}`)))
	require.False(t, openAICompactionV2BodyHasTerminalEvent([]byte("data: {\"type\":\"response.created\"}\n\n")))
	require.True(t, openAICompactionV2BodyHasTerminalEvent([]byte("data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\"}}\n\n")))
}

func TestOpenAIGatewayServiceForwardSkipsChatCompletionsOnlyAccountForRemoteCompactionV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))

	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       202,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
		},
	}

	result, err := svc.Forward(context.Background(), c, account, requestBody)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.DoNotPenalizeAccount)
	require.Contains(t, gjson.GetBytes(failoverErr.ResponseBody, "error.message").String(), "requires a Responses API upstream")
	require.Nil(t, upstream.lastReq)
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.Bytes())
}

func TestOpenAIGatewayServiceForwardDiscardsNonTerminalCompactionErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))

	upstream := &httpUpstreamRecorder{err: errors.New("dial failed")}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:          203,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-upstream", "base_url": "https://upstream.example/v1"},
		Extra: map[string]any{
			openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses),
		},
	}

	result, err := svc.Forward(context.Background(), c, account, requestBody)
	require.Nil(t, result)
	require.ErrorContains(t, err, "remote compaction v2 upstream error")
	require.Empty(t, recorder.Body.Bytes())
	require.False(t, c.Writer.Written())
}

func TestOpenAIGatewayServiceForwardFailsOverInvalidRemoteCompactionV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"keep context"}]},{"type":"compaction_trigger"}]}`)

	for _, tc := range []struct {
		name               string
		passthrough        bool
		responsesSupported bool
	}{
		{name: "normal responses", passthrough: false, responsesSupported: true},
		{name: "aether passthrough", passthrough: true, responsesSupported: true},
		{name: "aether passthrough ignores chat-only probe", passthrough: true, responsesSupported: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream"},
					"x-request-id": []string{"req_bad_compaction"},
				},
				Body: io.NopCloser(bytes.NewReader(ordinaryRemoteCompactionV2SSEFixture())),
			}}
			svc := &OpenAIGatewayService{
				cfg:          &config.Config{},
				httpUpstream: upstream,
			}
			extra := map[string]any{
				openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
				openai_compat.ExtraKeyResponsesSupported: tc.responsesSupported,
			}
			if tc.passthrough {
				extra["openai_passthrough"] = true
			}
			account := &Account{
				ID:          101,
				Name:        tc.name,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Concurrency: 1,
				Credentials: map[string]any{
					"api_key":  "sk-upstream",
					"base_url": "https://aether.example/v1",
				},
				Extra:       extra,
				Status:      StatusActive,
				Schedulable: true,
			}

			result, err := svc.Forward(context.Background(), c, account, requestBody)
			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.Contains(t, gjson.GetBytes(failoverErr.ResponseBody, "error.message").String(), "got 0 from 4 output items")
			require.False(t, c.Writer.Written())
			require.Empty(t, rec.Body.Bytes())
			require.NotNil(t, upstream.lastReq)
			require.Contains(t, upstream.lastReq.URL.Path, "/responses")
			require.Equal(t, "compaction_trigger", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
		})
	}
}

func TestOpenAIGatewayServiceRemoteCompactionV2DiscardsInvalidAttemptThenCommitsValidAttempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"compaction_trigger"}]}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
				"X-Request-Id": []string{"req_invalid"},
			},
			Body: io.NopCloser(bytes.NewReader(ordinaryRemoteCompactionV2SSEFixture())),
		},
		{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
				"X-Request-Id": []string{"req_valid"},
			},
			Body: io.NopCloser(bytes.NewReader(validRemoteCompactionV2SSEFixture())),
		},
	}}
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{},
		httpUpstream:  upstream,
		toolCorrector: NewCodexToolCorrector(),
	}
	newAccount := func(id int64) *Account {
		return &Account{
			ID:          id,
			Name:        fmt.Sprintf("account-%d", id),
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Concurrency: 1,
			Credentials: map[string]any{
				"api_key":  "sk-upstream",
				"base_url": "https://upstream.example/v1",
			},
			Extra: map[string]any{
				openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeForceResponses),
				openai_compat.ExtraKeyResponsesSupported: true,
			},
			Status:      StatusActive,
			Schedulable: true,
		}
	}

	firstResult, firstErr := svc.Forward(context.Background(), c, newAccount(301), requestBody)
	require.Nil(t, firstResult)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, firstErr, &failoverErr)
	require.Empty(t, recorder.Body.Bytes())
	require.False(t, c.Writer.Written())

	secondResult, secondErr := svc.Forward(context.Background(), c, newAccount(302), requestBody)
	require.NoError(t, secondErr)
	require.NotNil(t, secondResult)
	require.Equal(t, "req_valid", secondResult.RequestID)
	require.Contains(t, recorder.Body.String(), `"type":"compaction"`)
	require.NotContains(t, recorder.Body.String(), "summary text")
	require.NotContains(t, recorder.Body.String(), "req_invalid")
	require.Len(t, upstream.requests, 2)
}
