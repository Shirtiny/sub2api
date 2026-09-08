package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const openAIStreamOpeningLimit = 256 << 10
const openAIStreamOverloadRetriedKey = "openai_stream_overload_retried"

// ForwardWithStreamRetry runs inside the existing account concurrency slot. It
// buffers only the uncommitted opening, not the response, and never changes
// accounts or lets HTTP heartbeat writes consume the safe replay window.
func ForwardWithStreamRetry(ctx context.Context, c *gin.Context, account *Account, stream bool, forward func() (*OpenAIForwardResult, error)) (*OpenAIForwardResult, error) {
	if !stream || c == nil || c.Writer == nil || account == nil {
		return forward()
	}
	original := c.Writer
	defer func() { c.Writer = original }()
	started := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		headers := original.Header().Clone()
		gate := &openAIStreamOpeningWriter{ResponseWriter: original, status: http.StatusOK}
		c.Writer = gate
		result, err := forward()
		c.Writer = original
		if gate.bypass {
			return result, err
		}
		if gate.writeErr != nil {
			return result, gate.writeErr
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var failover *UpstreamFailoverError
		errors.As(err, &failover)
		if gate.retryFailure == nil && failover != nil {
			gate.retryFailure = openAIStreamRetryableError(failover.StatusCode, failover.ResponseBody)
		}
		if gate.retryFailure == nil && !gate.httpErrorAfterHeartbeat && !gate.committed && !gate.replayUnsafe {
			if interrupted := openAIStreamReadInterruption(err, failover); interrupted != nil {
				if len(bytes.TrimSpace(gate.pending[gate.recordStart:])) != 0 {
					gate.replayUnsafe = true // a partial/unknown frame is not a known empty opening
				} else {
					gate.retryFailure = interrupted
				}
			}
		}
		// Policy denials and image/tool results are never replayed, even if their
		// service path chose to return them before writing the wire response.
		canReplay := !gate.committed && !gate.replayUnsafe && GetOpsCyberPolicy(c) == nil && (result == nil || result.ImageCount == 0)
		if gate.retryFailure != nil && canReplay {
			retryEvent := "openai.stream_overload_retry"
			if gjson.GetBytes(gate.retryFailure.ResponseBody, "error.code").String() == "stream_interrupted" {
				retryEvent = "openai.stream_interruption_retry"
			}

			gate.retryFailure.StopLocalRetry = true
			gate.retryFailure.ResponseUncommitted = true
			setOpsUpstreamError(c, gate.retryFailure.StatusCode, extractUpstreamErrorMessage(gate.retryFailure.ResponseBody), "")
			retryAfter := gate.retryAfter
			if retryAfter == "" {
				retryAfter = original.Header().Get("Retry-After")
			}
			delay, allowed := openAIStreamOverloadRetryDelay(retryAfter)
			if failover != nil && failover.ResponseHeaders.Get("Retry-After") != "" {
				delay, allowed = openAIStreamOverloadRetryDelay(failover.ResponseHeaders.Get("Retry-After"))
			}
			if c.GetBool(openAIStreamOverloadRetriedKey) || !allowed {
				logger.FromContext(ctx).Warn(retryEvent+"_exhausted", zap.Int64("account_id", account.ID))
				return nil, gate.retryFailure
			}
			c.Set(openAIStreamOverloadRetriedKey, true)
			c.Set("openai_stream_rescue_event", retryEvent)
			logger.FromContext(ctx).Warn(retryEvent, zap.Int64("account_id", account.ID), zap.Duration("delay", delay))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			// Only response headers are reset; c.Request, body and the selected
			// account belong to the original forward closure and stay unchanged.
			for key := range original.Header() {
				delete(original.Header(), key)
			}
			for key, values := range headers {
				original.Header()[key] = values
			}
			continue
		}
		if failover != nil && canReplay {
			copy := *failover
			copy.ResponseUncommitted = true
			copy.StopLocalRetry = copy.StopLocalRetry || c.GetBool(openAIStreamOverloadRetriedKey)
			return result, &copy // discard failed preambles, preserve normal failover
		}
		if failover != nil && gate.replayUnsafe {
			copy := *failover
			copy.StopLocalRetry = true // tool/unknown activity may be invisible after conversion
			err = &copy
		}
		if gate.httpErrorAfterHeartbeat {
			// HTTP status/headers are already SSE on the wire. Let the handler
			// send a native terminal event instead of inserting bare JSON/HTML.
			return nil, &UpstreamFailoverError{StatusCode: gate.status, ResponseBody: append([]byte(nil), gate.pending...), StopLocalRetry: true, ResponseUncommitted: true}
		}
		if gate.retryFailure != nil && err == nil {
			err = gate.retryFailure
		}
		if finishErr := gate.commit(); finishErr != nil {
			return result, finishErr
		}
		if result != nil {
			result.Duration = time.Since(started)
			if !gate.firstContentAt.IsZero() {
				ms := int(gate.firstContentAt.Sub(started).Milliseconds())
				result.FirstTokenMs = &ms
			}
		}
		if err == nil && c.GetBool(openAIStreamOverloadRetriedKey) {
			logger.FromContext(ctx).Info(c.GetString("openai_stream_rescue_event")+"_recovered", zap.Int64("account_id", account.ID))
		}
		return result, err
	}
}

func openAIStreamOverloadRetryDelay(retryAfter string) (time.Duration, bool) {
	if strings.TrimSpace(retryAfter) == "" {
		return 500 * time.Millisecond, true
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter))
	if err != nil || seconds < 0 || seconds > 5 {
		return 0, false
	}
	delay := time.Duration(seconds) * time.Second
	if delay < 500*time.Millisecond {
		delay = 500 * time.Millisecond
	}
	return delay, true
}

func openAIOverloadError(status int, payload []byte) *UpstreamFailoverError {
	if !gjson.ValidBytes(payload) {
		return nil
	}
	root := gjson.ParseBytes(payload)
	if event := root.Get("type").String(); status < 400 && event != "" && event != "error" && event != "response.failed" {
		return nil
	}
	e := root.Get("response.error")
	if !e.IsObject() {
		e = root.Get("error")
	}
	if !e.IsObject() {
		return nil
	}
	message := e.Get("message").String()
	kind := strings.ToLower(e.Get("type").String() + " " + e.Get("code").String())
	if code := e.Get("code").Int(); code >= 400 && code < 500 {
		return nil
	}
	for _, marker := range []string{"authentication", "unauthorized", "permission", "invalid_api_key", "insufficient_quota", "rate_limit"} {
		if strings.Contains(kind, marker) {
			return nil
		}
	}
	combined := strings.ToLower(message) + " " + kind
	if !strings.Contains(combined, "overload") || !openAIStreamFailedEventShouldFailover(payload, message) {
		return nil
	}
	if status >= 400 && status != 503 && status != 529 {
		return nil
	}
	if status < 400 && e.Get("code").Int() != 503 && e.Get("code").Int() != 529 && !strings.Contains(kind, "overload") && !strings.Contains(strings.ToLower(message), "our servers are currently overloaded") {
		return nil
	}
	status = http.StatusServiceUnavailable
	body, _ := json.Marshal(gin.H{"error": gin.H{"type": "overloaded_error", "code": status, "message": sanitizeUpstreamErrorMessage(message)}})
	return &UpstreamFailoverError{StatusCode: status, ResponseBody: body}
}

func rememberOpenAIStreamRetryHeaders(c *gin.Context, headers http.Header) {
	if c == nil {
		return
	}
	if gate, ok := c.Writer.(*openAIStreamOpeningWriter); ok {
		gate.retryAfter = headers.Get("Retry-After")
	}
}

// Used before Responses -> Chat/Messages conversion, where an error event may
// otherwise be dropped or turned into a successful finish by a converter.
func captureOpenAIStreamRetryableError(c *gin.Context, payload []byte) *UpstreamFailoverError {
	if c == nil {
		return nil
	}
	gate, ok := c.Writer.(*openAIStreamOpeningWriter)
	if !ok {
		return nil
	}
	err := openAIStreamRetryableError(http.StatusOK, payload)
	if err != nil {
		gate.retryFailure = err
	}
	return err
}

// Converters can omit upstream tool/unknown events. Such activity must still
// prohibit replay, even if the converted client stream has not emitted a delta.
func observeOpenAIStreamRetrySource(c *gin.Context, payload []byte) {
	if c == nil {
		return
	}
	gate, ok := c.Writer.(*openAIStreamOpeningWriter)
	if !ok {
		return
	}
	e := gjson.GetBytes(payload, "response.error")
	if !e.IsObject() {
		e = gjson.GetBytes(payload, "error")
	}
	if e.IsObject() {
		// A converter/refusal detector may omit a business error. Its later
		// EOF must not be mistaken for a retryable transport interruption.
		kind := strings.ToLower(e.Get("type").String() + " " + e.Get("code").String())
		code := e.Get("code").Int()
		denied := !openAIStreamFailedEventShouldFailover(payload, e.Get("message").String()) || (code >= 400 && code < 500)
		for _, marker := range []string{"authentication", "unauthorized", "permission", "invalid_api_key", "insufficient_quota", "rate_limit"} {
			denied = denied || strings.Contains(kind, marker)
		}
		gate.replayUnsafe = gate.replayUnsafe || denied
		return
	}
	if classifyOpenAIStreamOpening(payload, "") == streamOpeningContent {
		gate.replayUnsafe = true
	}
}

type streamOpeningDecision uint8

const (
	streamOpeningPending streamOpeningDecision = iota
	streamOpeningContent
	streamOpeningHeartbeat
)

// Empty strings are literal emptiness: whitespace is already visible content.
func emptyOpeningText(v gjson.Result) bool {
	return !v.Exists() || v.Type == gjson.Null || (v.Type == gjson.String && v.Str == "")
}
func emptyOpeningArray(v gjson.Result) bool {
	return !v.Exists() || (v.IsArray() && len(v.Array()) == 0)
}

func classifyOpenAIStreamOpening(payload []byte, event string) streamOpeningDecision {
	if !gjson.ValidBytes(payload) {
		return streamOpeningContent
	}
	v := gjson.ParseBytes(payload)
	if t := v.Get("type").String(); t != "" {
		event = t
	}
	switch event {
	case "ping", "response.aether_keepalive":
		return streamOpeningHeartbeat
	case "response.created", "response.in_progress", "response.queued":
		if emptyOpeningArray(v.Get("response.output")) {
			return streamOpeningPending
		}
	case "response.output_item.added":
		switch v.Get("item.type").String() {
		case "message":
			if emptyOpeningArray(v.Get("item.content")) {
				return streamOpeningPending
			}
		case "reasoning":
			if emptyOpeningArray(v.Get("item.summary")) && emptyOpeningArray(v.Get("item.content")) && emptyOpeningText(v.Get("item.encrypted_content")) {
				return streamOpeningPending
			}
		}
	case "response.content_part.added", "response.reasoning_summary_part.added":
		t := v.Get("part.type").String()
		if (t == "output_text" || t == "summary_text") && emptyOpeningText(v.Get("part.text")) && emptyOpeningText(v.Get("part.refusal")) && emptyOpeningArray(v.Get("part.annotations")) {
			return streamOpeningPending
		}
	case "message_start":
		if emptyOpeningArray(v.Get("message.content")) {
			return streamOpeningPending
		}
	case "content_block_start":
		t := v.Get("content_block.type").String()
		if (t == "text" || t == "thinking") && emptyOpeningText(v.Get("content_block.text")) && emptyOpeningText(v.Get("content_block.thinking")) && emptyOpeningText(v.Get("content_block.signature")) {
			return streamOpeningPending
		}
	case "":
		choices := v.Get("choices")
		if choices.IsArray() && len(choices.Array()) > 0 {
			for _, choice := range choices.Array() {
				if !emptyOpeningText(choice.Get("finish_reason")) || !choice.Get("delta").IsObject() {
					return streamOpeningContent
				}
				empty := true
				choice.Get("delta").ForEach(func(k, value gjson.Result) bool {
					switch k.Str {
					case "role":
						empty = value.Str == "assistant"
					case "content":
						empty = emptyOpeningText(value)
					default:
						empty = false
					}
					return empty
				})
				if !empty {
					return streamOpeningContent
				}
			}
			return streamOpeningPending
		}
	}
	return streamOpeningContent
}

// This writer deliberately leaves Written/Size as real HTTP state. Only the
// explicit flags on returned errors tell the handler that bytes were heartbeats.
type openAIStreamOpeningWriter struct {
	bypass                  bool
	httpErrorAfterHeartbeat bool
	retryAfter              string
	gin.ResponseWriter
	status                          int
	pending                         []byte
	scanned, lineStart, recordStart int
	committed, replayUnsafe         bool
	writeErr                        error
	retryFailure                    *UpstreamFailoverError
	firstContentAt                  time.Time
}

func (w *openAIStreamOpeningWriter) WriteHeader(status int) {
	if !w.committed && status > 0 {
		w.status = status
	} else {
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *openAIStreamOpeningWriter) WriteHeaderNow() {
	if w.committed {
		w.ResponseWriter.WriteHeaderNow()
	}
}
func (w *openAIStreamOpeningWriter) Status() int {
	if w.committed {
		return w.ResponseWriter.Status()
	}
	return w.status
}
func (w *openAIStreamOpeningWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *openAIStreamOpeningWriter) Flush() {
	if w.committed {
		w.ResponseWriter.Flush()
	}
}

func (w *openAIStreamOpeningWriter) commit() error {
	if w.committed {
		return w.writeErr
	}
	w.committed = true
	w.ResponseWriter.WriteHeader(w.status)
	if len(w.pending) > 0 {
		_, w.writeErr = w.ResponseWriter.Write(w.pending)
		w.pending = nil
		w.ResponseWriter.Flush()
	}
	return w.writeErr
}

func (w *openAIStreamOpeningWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.committed {
		return w.ResponseWriter.Write(data)
	}
	if w.retryFailure != nil {
		return len(data), nil
	} // one held terminal error, not success
	if len(w.pending)+len(data) > openAIStreamOpeningLimit {
		if w.status >= 400 && w.Written() {
			w.httpErrorAfterHeartbeat = true
			w.replayUnsafe = true
			return len(data), nil
		}
		if err := w.commit(); err != nil {
			return 0, err
		}
		return w.ResponseWriter.Write(data)
	}
	w.pending = append(w.pending, data...)
	if !strings.Contains(strings.ToLower(w.Header().Get("Content-Type")), "text/event-stream") {
		if w.status >= 400 {
			w.httpErrorAfterHeartbeat = w.Written()
			if !json.Valid(w.pending) {
				return len(data), nil
			}
			w.retryFailure = openAIStreamRetryableError(w.status, w.pending)
			if w.retryFailure != nil {
				return len(data), nil
			}
			if w.httpErrorAfterHeartbeat {
				return len(data), nil
			}
		}
		return len(data), w.commit()
	}
	for w.scanned < len(w.pending) {
		i := w.scanned
		w.scanned++
		if w.pending[i] != '\n' {
			continue
		}
		line := bytes.TrimSuffix(w.pending[w.lineStart:i], []byte{'\r'})
		w.lineStart = i + 1
		if len(line) != 0 {
			// Complete content JSON is released on its data line, not after a
			// later record terminator. Errors must wait for the complete record.
			if bytes.HasPrefix(line, []byte("data:")) {
				payload := bytes.TrimSpace(line[5:])
				event, _, _ := parseStreamOpeningRecord(w.pending[w.recordStart : i+1])
				if json.Valid(payload) && openAIStreamRetryableError(200, payload) == nil && classifyOpenAIStreamOpening(payload, event) == streamOpeningContent {
					w.firstContentAt = time.Now()
					return len(data), w.commit()
				}
			}
			continue
		}
		record := w.pending[w.recordStart : i+1]
		event, payload, valid := parseStreamOpeningRecord(record)
		if !valid {
			return len(data), w.commit()
		}
		if len(payload) > 0 {
			w.retryFailure = openAIStreamRetryableError(200, payload)
			if w.retryFailure != nil {
				return len(data), nil
			}
		}
		decision := streamOpeningHeartbeat
		if len(payload) > 0 {
			decision = classifyOpenAIStreamOpening(payload, event)
		}
		switch decision {
		case streamOpeningContent:
			w.firstContentAt = time.Now()
			return len(data), w.commit()
		case streamOpeningHeartbeat:
			// Never flush the buffered preamble with a heartbeat. All heartbeat
			// forms are sent as a neutral SSE comment, with no response identity.
			w.ResponseWriter.Header().Del("Content-Length")
			w.ResponseWriter.WriteHeader(http.StatusOK)
			_, w.writeErr = io.WriteString(w.ResponseWriter, ":\n\n")
			if w.writeErr != nil {
				return 0, w.writeErr
			}
			w.ResponseWriter.Flush()
			w.pending = append(w.pending[:w.recordStart], w.pending[i+1:]...)
			w.scanned = w.recordStart
			w.lineStart = w.recordStart
			continue
		}
		w.recordStart = i + 1
	}
	return len(data), nil
}

func parseStreamOpeningRecord(record []byte) (string, []byte, bool) {
	var event string
	var data [][]byte
	for _, line := range bytes.Split(record, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 || line[0] == ':' {
			continue
		}
		field, value, _ := bytes.Cut(line, []byte{':'})
		value = bytes.TrimPrefix(value, []byte{' '})
		switch string(field) {
		case "event":
			event = string(value)
		case "data":
			data = append(data, value)
		default:
			return "", nil, false
		}
	}
	payload := bytes.Join(data, []byte{'\n'})
	if len(payload) > 0 && !json.Valid(payload) {
		return event, payload, false
	}
	return event, payload, true
}
