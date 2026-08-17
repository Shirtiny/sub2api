package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func ordinaryCompactionV2SSEFixture() []byte {
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
		`data: {"type":"response.completed","response":{"id":"resp_bad","object":"response","model":"gpt-5.6-sol","status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"reason"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"summary text"}]},{"type":"function_call","name":"followup_task","arguments":"{}"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":100,"output_tokens":20,"total_tokens":120}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n"))
}

func TestInspectOpenAICompactionV2SSERejectsOrdinaryOutputItems(t *testing.T) {
	stats := inspectOpenAICompactionV2SSE(ordinaryCompactionV2SSEFixture())
	require.Equal(t, 4, stats.OutputItemCount)
	require.Equal(t, 0, stats.CompactionCount)
	require.True(t, stats.SawCompleted)
	require.Contains(t, openAICompactionV2ResponseError(stats), "got 0 from 4 output items")
}

func TestNormalizeOpenAICompactionV2ResponseBodyWrapsOrdinarySSE(t *testing.T) {
	body, changed, err := normalizeOpenAICompactionV2ResponseBody(
		ordinaryCompactionV2SSEFixture(),
		true,
	)
	require.NoError(t, err)
	require.True(t, changed)

	stats := inspectOpenAICompactionV2SSE(body)
	require.Equal(t, 1, stats.OutputItemCount)
	require.Equal(t, 1, stats.CompactionCount)
	require.True(t, stats.SawCompleted)

	finalResponse, ok := extractCodexFinalResponse(string(body))
	require.True(t, ok)
	require.Equal(t, "compaction", gjson.GetBytes(finalResponse, "output.0.type").String())
	encoded := gjson.GetBytes(finalResponse, "output.0.encrypted_content").String()
	require.NotEmpty(t, encoded)
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	require.Equal(t, "sub2api-remote-compaction-v2-compat", gjson.GetBytes(decoded, "format").String())
	require.Equal(t, "resp_bad", gjson.GetBytes(decoded, "response_id").String())
	require.Len(t, gjson.GetBytes(decoded, "output").Array(), 4)
}

func TestExpandOpenAICompactionV2CompatInputRestoresWrappedOutput(t *testing.T) {
	wrapped, _, err := normalizeOpenAICompactionV2ResponseBody(ordinaryCompactionV2SSEFixture(), false)
	require.NoError(t, err)
	compaction := gjson.GetBytes(wrapped, "output.0")
	require.Equal(t, "compaction", compaction.Get("type").String())
	request := []byte(`{"model":"gpt-5.6-sol","input":[` + compaction.Raw + `,{"type":"message","role":"user","content":"next"}]}`)

	expanded, changed, err := expandOpenAICompactionV2CompatInput(request)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "reasoning", gjson.GetBytes(expanded, "input.0.type").String())
	require.Equal(t, "message", gjson.GetBytes(expanded, "input.1.type").String())
	require.Equal(t, "function_call", gjson.GetBytes(expanded, "input.2.type").String())
	require.Equal(t, "message", gjson.GetBytes(expanded, "input.3.type").String())
	require.Equal(t, "message", gjson.GetBytes(expanded, "input.4.type").String())
}

func TestExpandOpenAICompactionV2CompatInputPreservesOfficialCiphertext(t *testing.T) {
	original := []byte(`{"model":"gpt-5.6-sol","input":[{"type":"compaction","encrypted_content":"official-ciphertext"}]}`)
	expanded, changed, err := expandOpenAICompactionV2CompatInput(original)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, original, expanded)
}

func TestNormalizeOpenAICompactionV2ResponseBodyPreservesValidSSE(t *testing.T) {
	original := []byte(strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"compaction","encrypted_content":"ciphertext"}}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","output":[]}}`,
		"",
	}, "\n"))
	body, changed, err := normalizeOpenAICompactionV2ResponseBody(
		original,
		true,
	)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, original, body)
}

func TestNormalizeOpenAICompactionV2ResponseBodyConvertsOrdinaryJSONToSSE(t *testing.T) {
	original := []byte(`{"id":"resp_json","object":"response","model":"gpt-5.6-sol","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"dynamic summary"}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`)
	body, changed, err := normalizeOpenAICompactionV2ResponseBody(original, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, bodyHasSSEFraming(body))
	stats := inspectOpenAICompactionV2SSE(body)
	require.Equal(t, 1, stats.OutputItemCount)
	require.Equal(t, 1, stats.CompactionCount)
	require.True(t, stats.SawCompleted)
}

func TestNormalizeOpenAICompactionV2ResponseReplacesResponseBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"text/event-stream"},
			"Content-Length": []string{"999"},
		},
		ContentLength: 999,
		Body:          io.NopCloser(strings.NewReader(string(ordinaryCompactionV2SSEFixture()))),
	}
	svc := &OpenAIGatewayService{}
	require.NoError(t, svc.normalizeOpenAICompactionV2Response(resp, true))
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	stats := inspectOpenAICompactionV2SSE(got)
	require.Equal(t, 1, stats.OutputItemCount)
	require.Equal(t, 1, stats.CompactionCount)
	require.True(t, stats.SawCompleted)
	require.Empty(t, resp.Header.Get("Content-Length"))
	require.EqualValues(t, -1, resp.ContentLength)
}

func TestOpenAIGatewayServiceForwardWrapsOrdinaryRemoteCompactionV2Response(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"keep context"}]},{"type":"compaction_trigger"}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(string(ordinaryCompactionV2SSEFixture()))),
	}}
	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
	account := &Account{
		ID:          101,
		Name:        "aether",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-upstream",
			"base_url": "https://aether.example/v1",
		},
		Extra: map[string]any{
			"openai_passthrough":                     true,
			openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
			openai_compat.ExtraKeyResponsesSupported: true,
		},
		Status:      StatusActive,
		Schedulable: true,
	}

	result, err := svc.Forward(context.Background(), c, account, requestBody)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "compaction_trigger", gjson.GetBytes(upstream.lastBody, "input.1.type").String())

	stats := inspectOpenAICompactionV2SSE(rec.Body.Bytes())
	require.Equal(t, 1, stats.OutputItemCount)
	require.Equal(t, 1, stats.CompactionCount)
	require.True(t, stats.SawCompleted)
}
