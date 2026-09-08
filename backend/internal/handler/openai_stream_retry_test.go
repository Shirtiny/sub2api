package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamRetryFailoverGuardDistinguishesHeartbeatsFromContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	before := c.Writer.Size()
	_, err := c.Writer.WriteString(":\n\n")
	require.NoError(t, err)
	c.Writer.Flush()
	require.True(t, stopOpenAIStreamFailover(c, &service.UpstreamFailoverError{}, before), "legacy errors remain conservative")
	require.False(t, stopOpenAIStreamFailover(c, &service.UpstreamFailoverError{ResponseUncommitted: true}, before), "only the gate can certify heartbeat-only writes")
	require.True(t, stopOpenAIStreamFailover(c, &service.UpstreamFailoverError{ResponseUncommitted: true, StopLocalRetry: true}, before), "exhaustion cannot multiply outer retries")
	c, _ = gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, stopOpenAIStreamFailover(c, &service.UpstreamFailoverError{StopLocalRetry: true}, c.Writer.Size()))
}

func TestStreamRetryExhaustionAfterHeartbeatWritesOneNativeTerminalError(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		c.Header("Content-Type", "text/event-stream")
		_, err := c.Writer.WriteString(":\n\n")
		require.NoError(t, err)
		c.Writer.Flush()
		failover := &service.UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(`{"error":{"message":"overloaded"}}`), ResponseUncommitted: true, StopLocalRetry: true}
		h := &OpenAIGatewayHandler{}
		if path == "/v1/messages" {
			h.handleAnthropicFailoverExhausted(c, failover, c.Writer.Written())
		} else {
			h.handleFailoverExhausted(c, failover, c.Writer.Written())
		}
		require.Equal(t, 200, rec.Code)
		require.NotContains(t, rec.Body.String(), "response.completed")
		require.NotContains(t, rec.Body.String(), "data: [DONE]")
		if path == "/v1/responses" {
			require.Contains(t, rec.Body.String(), "event: response.failed")
		} else {
			require.Contains(t, rec.Body.String(), "event: error")
		}
	}
}
