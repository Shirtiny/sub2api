package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Raw/converted paths that used to finish successfully on EOF must report the
// protocol cutoff while the opening gate is active. Existing non-gated callers
// retain their legacy behavior; a completed terminal followed by a read error
// is not a failed generation.
func openAIStreamEndError(c *gin.Context, terminal bool, scanErr error) error {
	if c == nil {
		return nil
	}
	gate, ok := c.Writer.(*openAIStreamOpeningWriter)
	if !ok || terminal {
		return nil
	}
	if scanErr != nil {
		if errors.Is(scanErr, bufio.ErrTooLong) {
			gate.replayUnsafe = true
			return scanErr
		}
		return fmt.Errorf("stream read error: %w", scanErr)
	}
	return errors.New("stream usage incomplete: missing terminal event")
}

// Only known stream-transport/termination failures are eligible. These are not
// general 5xx or arbitrary error-string retries, and the caller must additionally
// prove that no content/tool/unknown event has been committed.
func knownOpenAIStreamInterruption(message string) bool {
	switch strings.TrimSpace(message) {
	case "stream usage incomplete: missing terminal event",
		"OpenAI stream ended before a terminal event",
		"OpenAI messages stream ended before a terminal event",
		"OpenAI stream disconnected before completion",
		"execution runtime stream ended before provider terminal event",
		"stream data interval timeout",
		"stream_read_error", "stream_timeout":
		return true
	}
	for _, prefix := range []string{"stream read error:", "OpenAI stream disconnected before completion:", "stream usage incomplete: unexpected EOF"} {
		if strings.HasPrefix(strings.TrimSpace(message), prefix) {
			return true
		}
	}
	return false
}

func newOpenAIStreamInterruption(message string) *UpstreamFailoverError {
	if strings.TrimSpace(message) == "" {
		message = "OpenAI stream disconnected before completion"
	}
	body, _ := json.Marshal(gin.H{"error": gin.H{
		"type": "upstream_error", "code": "stream_interrupted",
		"message": sanitizeUpstreamErrorMessage(message),
	}})
	status := http.StatusBadGateway
	if message == "stream data interval timeout" || message == "stream_timeout" {
		status = http.StatusGatewayTimeout
	}
	return &UpstreamFailoverError{StatusCode: status, ResponseBody: body}
}

func openAIStreamRetryableError(status int, payload []byte) *UpstreamFailoverError {
	// HTTP 503/529 overloaded responses are intentionally not retried on the
	// same account/request. They are passed through to the normal failover
	// policy; only explicit stream transport interruptions remain retryable.
	if (status >= 400 && status != 502 && status != 504) || !gjson.ValidBytes(payload) {
		return nil
	}
	root := gjson.ParseBytes(payload)
	if event := root.Get("type").String(); event != "" && event != "error" && event != "response.failed" {
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
	if !openAIStreamFailedEventShouldFailover(payload, message) {
		return nil
	}
	kind := strings.ToLower(e.Get("type").String() + " " + e.Get("code").String())
	if code := e.Get("code").Int(); code >= 400 && code < 500 {
		return nil
	}
	for _, marker := range []string{"authentication", "unauthorized", "permission", "invalid_api_key", "insufficient_quota", "rate_limit"} {
		if strings.Contains(kind, marker) {
			return nil
		}
	}
	// Responses/Messages use symbolic codes; Aether's Chat errors carry a
	// numeric HTTP code and the interruption kind in type. Accept either.
	for _, code := range []string{e.Get("code").String(), e.Get("type").String()} {
		switch code {
		case "stream_timeout", "stream_idle_timeout", "stream_progress_timeout":
			failure := newOpenAIStreamInterruption(message)
			failure.StatusCode = http.StatusGatewayTimeout
			return failure
		case "stream_interrupted", "stream_missing_terminal_event", "stream_missing_terminal", "stream_read_error":
			failure := newOpenAIStreamInterruption(message)
			if status == http.StatusGatewayTimeout || e.Get("code").Int() == http.StatusGatewayTimeout {
				failure.StatusCode = http.StatusGatewayTimeout
			}
			return failure
		}
	}
	if knownOpenAIStreamInterruption(message) {
		return newOpenAIStreamInterruption(message)
	}
	return nil
}

func openAIStreamReadInterruption(err error, failover *UpstreamFailoverError) *UpstreamFailoverError {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	if failover != nil {
		return openAIStreamRetryableError(failover.StatusCode, failover.ResponseBody)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || knownOpenAIStreamInterruption(err.Error()) {
		return newOpenAIStreamInterruption(err.Error())
	}
	return nil
}
