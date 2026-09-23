package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestStreamRetryBillingCancelledResultCanSubmitDetachedUsage(t *testing.T) {
	for _, withPool := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync_fallback", true: "worker_pool"}[withPool], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"stream":true}`))
			parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "billing-client-id")
			parent = context.WithValue(parent, ctxkey.RequestID, "billing-server-id")
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			calls := 0
			result, err := service.ForwardWithStreamRetry(ctx, c, &service.Account{ID: 17, Platform: service.PlatformOpenAI}, true, func() (*service.OpenAIForwardResult, error) {
				calls++
				c.Header("Content-Type", "text/event-stream")
				_, e := c.Writer.WriteString("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
				require.NoError(t, e)
				cancel()
				return &service.OpenAIForwardResult{Usage: service.OpenAIUsage{InputTokens: 10, OutputTokens: 20}}, nil
			})
			require.NoError(t, err, "the Responses handler must not take its early-error return")
			require.Equal(t, 1, calls)
			h := &OpenAIGatewayHandler{}
			if withPool {
				h.usageRecordWorkerPool = newUsageRecordTestPool(t)
			}
			type recorded struct {
				err                 error
				clientID, requestID any
				tokens              int
			}
			done := make(chan recorded, 1)
			var records atomic.Int32
			h.submitOpenAIUsageRecordTask(ctx, result, func(billingCtx context.Context) {
				records.Add(1)
				done <- recorded{billingCtx.Err(), billingCtx.Value(ctxkey.ClientRequestID), billingCtx.Value(ctxkey.RequestID), result.Usage.OutputTokens}
			})
			select {
			case got := <-done:
				require.NoError(t, got.err)
				require.Equal(t, "billing-client-id", got.clientID)
				require.Equal(t, "billing-server-id", got.requestID)
				require.Equal(t, 20, got.tokens)
			case <-time.After(time.Second):
				t.Fatal("cancelled client prevented usage submission")
			}
			require.Equal(t, int32(1), records.Load())
		})
	}
}
