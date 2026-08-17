package service

import (
	"bytes"
	"context"
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
}

func TestReadAndValidateOpenAICompactionV2ResponseRestoresValidBody(t *testing.T) {
	original := validRemoteCompactionV2SSEFixture()
	resp := &http.Response{Body: io.NopCloser(bytes.NewReader(original))}
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	buffered, err := svc.readAndValidateOpenAICompactionV2Response(resp, true)
	require.NoError(t, err)
	require.Equal(t, original, buffered)

	restored, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, original, restored)
}

func TestOpenAIGatewayServiceForwardFailsOverInvalidRemoteCompactionV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"keep context"}]},{"type":"compaction_trigger"}]}`)

	for _, tc := range []struct {
		name        string
		passthrough bool
	}{
		{name: "normal responses", passthrough: false},
		{name: "aether passthrough", passthrough: true},
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
				openai_compat.ExtraKeyResponsesSupported: true,
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
			require.Equal(t, "compaction_trigger", gjson.GetBytes(upstream.lastBody, "input.1.type").String())
		})
	}
}
