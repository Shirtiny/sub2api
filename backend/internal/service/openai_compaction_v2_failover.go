package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// openAICompactionV2ResponseStats mirrors the validation Codex performs for a
// remote compaction v2 stream. Ordinary output items are not a usable compact
// result and must cause the current upstream account to fail over.
type openAICompactionV2ResponseStats struct {
	OutputItemCount int
	CompactionCount int
	SawCompleted    bool
}

func inspectOpenAICompactionV2SSE(body []byte) openAICompactionV2ResponseStats {
	stats := openAICompactionV2ResponseStats{}
	seenCompleted := false
	forEachOpenAISSEEventPayload(string(body), func(eventName string, data []byte) {
		if seenCompleted {
			return
		}
		dataType := strings.TrimSpace(gjson.GetBytes(data, "type").String())
		if dataType == "" {
			dataType = strings.TrimSpace(eventName)
		}
		switch dataType {
		case "response.output_item.done":
			stats.OutputItemCount++
			item := gjson.GetBytes(data, "item")
			switch item.Get("type").String() {
			case "compaction", "compaction_summary":
				if item.Get("encrypted_content").Type == gjson.String {
					stats.CompactionCount++
				}
			}
		case "response.completed":
			stats.SawCompleted = true
			seenCompleted = true
		}
	})
	return stats
}

func inspectOpenAICompactionV2JSON(body []byte) openAICompactionV2ResponseStats {
	stats := openAICompactionV2ResponseStats{}
	if !gjson.ValidBytes(body) {
		return stats
	}
	response := gjson.ParseBytes(body)
	output := response.Get("output")
	if output.IsArray() {
		items := output.Array()
		stats.OutputItemCount = len(items)
		for _, item := range items {
			switch item.Get("type").String() {
			case "compaction", "compaction_summary":
				if item.Get("encrypted_content").Type == gjson.String {
					stats.CompactionCount++
				}
			}
		}
	}
	status := strings.TrimSpace(response.Get("status").String())
	// A few compatible upstreams omit status on an otherwise terminal JSON
	// response. The output-item invariant remains the decisive check here.
	stats.SawCompleted = status == "" || status == "completed"
	return stats
}

func validateOpenAICompactionV2ResponseBody(body []byte, downstreamStream bool) error {
	if len(body) == 0 {
		return fmt.Errorf("remote compaction v2 response body is empty")
	}
	if bodyHasSSEFraming(body) {
		stats := inspectOpenAICompactionV2SSE(body)
		if !stats.SawCompleted {
			return fmt.Errorf("remote compaction v2 response closed before response.completed")
		}
		if stats.CompactionCount != 1 {
			return fmt.Errorf(
				"remote compaction v2 expected exactly one compaction output item, got %d from %d output items",
				stats.CompactionCount,
				stats.OutputItemCount,
			)
		}
		return nil
	}

	if downstreamStream {
		return fmt.Errorf("remote compaction v2 expected an event-stream response")
	}
	finalResponse, ok := extractCodexFinalResponse(string(body))
	if !ok {
		return fmt.Errorf("remote compaction v2 response has no response.completed payload")
	}
	stats := inspectOpenAICompactionV2JSON(finalResponse)
	if !stats.SawCompleted {
		return fmt.Errorf("remote compaction v2 response closed before response.completed")
	}
	if stats.CompactionCount != 1 {
		return fmt.Errorf(
			"remote compaction v2 expected exactly one compaction output item, got %d from %d output items",
			stats.CompactionCount,
			stats.OutputItemCount,
		)
	}
	return nil
}

// readAndValidateOpenAICompactionV2Response buffers only the compaction task.
// Buffering is required so an invalid 200 response can be rejected before any
// bytes reach the client, allowing the handler's account failover loop to run.
func (s *OpenAIGatewayService) readAndValidateOpenAICompactionV2Response(resp *http.Response, downstreamStream bool) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("remote compaction v2 upstream response has no body")
	}
	body, err := readUpstreamResponseBodyLimited(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read remote compaction v2 upstream response: %w", err)
	}
	if err := validateOpenAICompactionV2ResponseBody(body, downstreamStream); err != nil {
		return body, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func (s *OpenAIGatewayService) newOpenAICompactionV2FailoverError(
	c *gin.Context,
	account *Account,
	resp *http.Response,
	payload []byte,
	validationErr error,
	passthrough bool,
) *UpstreamFailoverError {
	message := "remote compaction v2 response is invalid"
	if validationErr != nil && strings.TrimSpace(validationErr.Error()) != "" {
		message += ": " + validationErr.Error()
	}
	requestID := ""
	if resp != nil {
		requestID = resp.Header.Get("x-request-id")
	}
	return s.newOpenAIStreamFailoverError(c, account, passthrough, requestID, payload, message)
}
