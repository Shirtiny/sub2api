package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type streamBillingFailWriter struct {
	gin.ResponseWriter
	fail     bool
	attempts int
}

func (w *streamBillingFailWriter) Write(p []byte) (int, error) {
	w.attempts++
	if w.fail {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(p)
}
func (w *streamBillingFailWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

type streamBillingReadStage struct {
	opening  *strings.Reader
	terminal *strings.Reader
	between  func()
	switched bool
}

func (r *streamBillingReadStage) Read(p []byte) (int, error) {
	if !r.switched {
		n, err := r.opening.Read(p)
		if err != io.EOF {
			return n, err
		}
		r.switched = true
		if r.between != nil {
			r.between()
		}
	}
	return r.terminal.Read(p)
}

func streamBillingWire(mode string) (string, string) {
	switch mode {
	case "raw-chat":
		return "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n",
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20}}\n\ndata: [DONE]\n\n"
	case "raw-messages":
		return "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-billing\",\"content\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":20}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	default:
		return "data: " + streamRetryCreated + "\n\ndata: " + streamRetryDelta + "\n\n",
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-billing\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}}\n\n"
	}
}

func forwardStreamBillingProcessor(svc *OpenAIGatewayService, mode string, ctx context.Context, c *gin.Context, account *Account, response *http.Response) (*OpenAIForwardResult, error) {
	switch mode {
	case "responses":
		r, e := svc.handleStreamingResponse(ctx, response, c, account, time.Now(), "model", "model")
		if r == nil {
			return nil, e
		}
		return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
	case "responses-passthrough":
		r, e := svc.handleStreamingResponsePassthrough(ctx, response, c, account, time.Now(), "model", "model")
		if r == nil {
			return nil, e
		}
		return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
	case "chat":
		return svc.handleChatStreamingResponse(response, c, account, "model", "model", "model", time.Now(), 10)
	case "messages":
		return svc.handleAnthropicStreamingResponse(response, c, account, "model", "model", "model", time.Now())
	case "raw-chat":
		return svc.streamRawChatCompletions(c, response, account, "model", "model", "model", nil, nil, time.Now(), 10)
	default:
		r, e := svc.handleOpenAIMessagesPassthroughStreamingResponse(ctx, response, c, account, time.Now())
		if r == nil {
			return nil, e
		}
		return &OpenAIForwardResult{Usage: *r.usage, FirstTokenMs: r.firstTokenMs}, e
	}
}

func TestStreamRetryBillingPreservesDrainedUsageAcrossProcessors(t *testing.T) {
	for _, mode := range []string{"responses", "responses-passthrough", "chat", "messages", "raw-chat", "raw-messages"} {
		for _, failure := range []string{"cancel", "first_content_write", "after_content_write", "heartbeat_write"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				c, rec, account := streamRetryContext()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c.Request = c.Request.WithContext(ctx)
				writer := &streamBillingFailWriter{ResponseWriter: c.Writer, fail: failure == "first_content_write" || failure == "heartbeat_write"}
				c.Writer = writer
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				opening, terminal := streamBillingWire(mode)
				calls := 0
				result, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
					calls++
					c.Header("Content-Type", "text/event-stream")
					if failure == "heartbeat_write" {
						_, _ = c.Writer.WriteString(":\n\n")
					}
					reader := &streamBillingReadStage{opening: strings.NewReader(opening), terminal: strings.NewReader(terminal), between: func() {
						if failure == "cancel" {
							cancel()
						}
						if failure == "after_content_write" {
							writer.fail = true
						}
					}}
					response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(reader)}
					return forwardStreamBillingProcessor(svc, mode, ctx, c, account, response)
				})
				require.NoError(t, err, "client delivery must not discard processor-approved usage")
				require.NotNil(t, result)
				require.Equal(t, 10, result.Usage.InputTokens)
				require.Equal(t, 20, result.Usage.OutputTokens)
				require.Equal(t, 1, calls, "a disconnect must never dispatch another generation")
				require.False(t, c.GetBool(openAIStreamOverloadRetriedKey))
				if failure == "cancel" {
					require.ErrorIs(t, ctx.Err(), context.Canceled)
					require.True(t, result.ClientDisconnect)
					require.Contains(t, rec.Body.String(), "hello")
				}
			})
		}
	}
}

func TestStreamRetryBillingNeverConvertsUpstreamFailuresToSuccess(t *testing.T) {
	for _, kind := range []string{"missing_result", "read_error", "overload", "hidden_overload", "filtered_overload", "hidden_business_error", "hidden_terminal_failure", "http_error"} {
		t.Run(kind, func(t *testing.T) {
			c, _, account := streamRetryContext()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			_, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
				calls++
				cancel()
				result := &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 20}}
				switch kind {
				case "missing_result":
					return nil, nil
				case "read_error":
					return result, fmt.Errorf("stream read error: %w", io.ErrUnexpectedEOF)
				case "overload":
					return result, &UpstreamFailoverError{StatusCode: 503, ResponseBody: []byte(streamRetryError)}
				case "hidden_overload":
					c.Header("Content-Type", "text/event-stream")
					writeRetrySSE(c.Writer, streamRetryError)
				case "filtered_overload":
					require.Nil(t, captureOpenAIStreamRetryableError(c, []byte(streamRetryError)), "overloads remain non-retryable")
				case "hidden_business_error":
					observeOpenAIStreamRetrySource(c, []byte(`{"error":{"type":"permission_denied","message":"denied"}}`))
				case "hidden_terminal_failure":
					c.Header("Content-Type", "text/event-stream")
					writeRetrySSE(c.Writer, `{"type":"response.failed","response":{"status":"failed"}}`)
				case "http_error":
					c.JSON(403, gin.H{"error": gin.H{"message": "denied"}})
				}
				return result, nil
			})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.False(t, c.GetBool(openAIStreamOverloadRetriedKey))
		})
	}
}

func TestStreamRetryBillingPreservesUsageOnFinalFlushFailure(t *testing.T) {
	c, _, account := streamRetryContext()
	writer := &streamBillingFailWriter{ResponseWriter: c.Writer, fail: true}
	c.Writer = writer
	want := &OpenAIForwardResult{Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 20}}
	result, err := ForwardWithStreamRetry(context.Background(), c, account, true, func() (*OpenAIForwardResult, error) {
		c.Header("Content-Type", "text/event-stream")
		writeRetrySSE(c.Writer, streamRetryCreated)
		require.Zero(t, writer.attempts, "only an empty opening is held before final flush")
		return want, nil
	})
	require.NoError(t, err)
	require.Same(t, want, result)
	require.True(t, result.ClientDisconnect)
	require.Equal(t, 1, writer.attempts)
}

func TestStreamRetryBillingCancellationAfterRescuePreservesTiming(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _, account := streamRetryContext()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		calls := 0
		result, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
			calls++
			if calls == 1 {
				return nil, &UpstreamFailoverError{StatusCode: 502, ResponseBody: []byte(streamRetryInterruptionError)}
			}
			c.Header("Content-Type", "text/event-stream")
			writeRetrySSE(c.Writer, streamRetryDelta)
			cancel()
			return &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 20}}, nil
		})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.GreaterOrEqual(t, result.Duration, 500*time.Millisecond)
		require.NotNil(t, result.FirstTokenMs)
		require.GreaterOrEqual(t, *result.FirstTokenMs, 500)
		require.True(t, result.ClientDisconnect)
	})
}

func TestStreamRetryBillingClientDeadlineDoesNotDiscardCompletedUsage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _, account := streamRetryContext()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
			c.Header("Content-Type", "text/event-stream")
			writeRetrySSE(c.Writer, streamRetryDelta)
			time.Sleep(2 * time.Second)
			return &OpenAIForwardResult{Usage: OpenAIUsage{OutputTokens: 20}}, nil
		})
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		require.NoError(t, err)
		require.Equal(t, 20, result.Usage.OutputTokens)
	})
}

func TestStreamRetryBillingCancellationBeforeForwardDoesNotDispatch(t *testing.T) {
	c, _, account := streamRetryContext()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	result, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) { calls++; return nil, nil })
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
}

func TestStreamRetryBillingCancelledUsageReachesExistingBilling(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(fmt.Sprintf("subscription=%v", subscription), func(t *testing.T) {
			c, _, account := streamRetryContext()
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxkey.ClientRequestID, "cancelled-billing-request"))
			defer cancel()
			result, err := ForwardWithStreamRetry(ctx, c, account, true, func() (*OpenAIForwardResult, error) {
				c.Header("Content-Type", "text/event-stream")
				writeRetrySSE(c.Writer, streamRetryDelta)
				cancel()
				return &OpenAIForwardResult{Model: "gpt-5.1", Stream: true, Usage: OpenAIUsage{InputTokens: 10, OutputTokens: 20}}, nil
			})
			require.NoError(t, err)
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
			svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			account.Type = AccountTypeAPIKey
			input := &OpenAIRecordUsageInput{Result: result, APIKey: &APIKey{ID: 100, GroupID: i64p(88), Group: &Group{ID: 88, RateMultiplier: 1}}, User: &User{ID: 200}, Account: account, APIKeyService: &openAIRecordUsageAPIKeyQuotaStub{}, RequestPayloadHash: "same-payload-fingerprint"}
			if subscription {
				input.APIKey.Group.SubscriptionType = SubscriptionTypeSubscription
				input.Subscription = &UserSubscription{ID: 300}
			}
			// The real handler submits billing on a detached context; preserve client ID
			// so the existing transactional dedup key remains the original request's key.
			err = svc.RecordUsage(context.WithoutCancel(ctx), input)
			require.NoError(t, err)
			require.Equal(t, 1, billingRepo.calls)
			require.Equal(t, 1, usageRepo.calls)
			require.NoError(t, billingRepo.lastCtxErr)
			require.NoError(t, usageRepo.lastCtxErr)
			require.Equal(t, "client:cancelled-billing-request", billingRepo.lastCmd.RequestID)
			require.Equal(t, "same-payload-fingerprint", billingRepo.lastCmd.RequestPayloadHash)
			require.Equal(t, 20, billingRepo.lastCmd.OutputTokens)
			require.Positive(t, usageRepo.lastLog.ActualCost)
			if subscription {
				require.Positive(t, billingRepo.lastCmd.SubscriptionCost)
				require.Zero(t, billingRepo.lastCmd.BalanceCost)
			} else {
				require.Positive(t, billingRepo.lastCmd.BalanceCost)
				require.Zero(t, billingRepo.lastCmd.SubscriptionCost)
			}
		})
	}
}
