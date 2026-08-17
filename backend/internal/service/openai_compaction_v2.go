package service

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAICompactionV2ResponseStats mirrors the part of Codex's remote
// compaction v2 collector that matters here. Codex counts every
// response.output_item.done event, but only accepts exactly one compaction
// item among them, followed by response.completed.
type openAICompactionV2ResponseStats struct {
	OutputItemCount int
	CompactionCount int
	SawCompleted    bool
	SSE             bool
}

func inspectOpenAICompactionV2SSE(body []byte) openAICompactionV2ResponseStats {
	stats := openAICompactionV2ResponseStats{SSE: true}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), defaultMaxLineSize)
	for scanner.Scan() {
		data, ok := extractOpenAISSEDataLine(scanner.Text())
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" || !gjson.Valid(data) {
			continue
		}
		payload := gjson.Parse(data)
		switch payload.Get("type").String() {
		case "response.output_item.done":
			stats.OutputItemCount++
			item := payload.Get("item")
			switch item.Get("type").String() {
			case "compaction", "compaction_summary":
				if item.Get("encrypted_content").Type == gjson.String {
					stats.CompactionCount++
				}
			}
		case "response.completed":
			stats.SawCompleted = true
			return stats
		}
	}
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
	stats.SawCompleted = status == "" || status == "completed"
	return stats
}

func openAICompactionV2ResponseError(stats openAICompactionV2ResponseStats) string {
	if !stats.SawCompleted {
		return "remote compaction v2 response closed before response.completed"
	}
	return fmt.Sprintf(
		"remote compaction v2 expected exactly one compaction output item, got %d from %d output items",
		stats.CompactionCount,
		stats.OutputItemCount,
	)
}

// collectOpenAICompactionV2OutputItems gets the complete output represented by
// a stream. The terminal response is preferred because it contains the
// provider's final item state; done events are the fallback for providers that
// leave response.completed.response.output empty.
func collectOpenAICompactionV2OutputItems(body string, finalResponse []byte) []json.RawMessage {
	if output := gjson.GetBytes(finalResponse, "output"); output.IsArray() {
		items := output.Array()
		result := make([]json.RawMessage, 0, len(items))
		for _, item := range items {
			if item.IsObject() && item.Raw != "" {
				result = append(result, json.RawMessage(item.Raw))
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	result := make([]json.RawMessage, 0, 4)
	forEachOpenAISSEDataPayload(body, func(data []byte) {
		if gjson.GetBytes(data, "type").String() != "response.output_item.done" {
			return
		}
		item := gjson.GetBytes(data, "item")
		if item.IsObject() && item.Raw != "" {
			result = append(result, json.RawMessage(item.Raw))
		}
	})
	return result
}

// buildOpenAICompactionV2CompatItem makes a dynamic opaque payload from the
// provider's ordinary output. This is a wire-compatibility fallback only: it
// is not an OpenAI-generated encrypted compaction state.
func buildOpenAICompactionV2CompatItem(body string, finalResponse []byte) ([]byte, error) {
	items := collectOpenAICompactionV2OutputItems(body, finalResponse)
	if len(items) == 0 {
		return nil, fmt.Errorf("remote compaction v2 response has no output items to wrap")
	}
	material, err := json.Marshal(map[string]any{
		"format":      "sub2api-remote-compaction-v2-compat",
		"response_id": strings.TrimSpace(gjson.GetBytes(finalResponse, "id").String()),
		"output":      items,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal compaction compatibility payload: %w", err)
	}
	item, err := json.Marshal(map[string]string{
		"type":              "compaction",
		"encrypted_content": base64.RawStdEncoding.EncodeToString(material),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal compaction compatibility item: %w", err)
	}
	return item, nil
}

func decodeOpenAICompactionV2CompatItem(item gjson.Result) ([]gjson.Result, bool) {
	if !item.IsObject() {
		return nil, false
	}
	switch item.Get("type").String() {
	case "compaction", "compaction_summary":
	default:
		return nil, false
	}
	encoded := strings.TrimSpace(item.Get("encrypted_content").String())
	if encoded == "" {
		return nil, false
	}
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(encoded)
	}
	if err != nil || !gjson.ValidBytes(decoded) {
		return nil, false
	}
	payload := gjson.ParseBytes(decoded)
	if payload.Get("format").String() != "sub2api-remote-compaction-v2-compat" {
		return nil, false
	}
	output := payload.Get("output")
	if !output.IsArray() {
		return nil, false
	}
	items := output.Array()
	if len(items) == 0 {
		return nil, false
	}
	for _, outputItem := range items {
		if !outputItem.IsObject() || outputItem.Raw == "" {
			return nil, false
		}
	}
	return items, true
}

// expandOpenAICompactionV2CompatInput restores locally wrapped output before
// forwarding the next turn. Official encrypted compaction items do not carry
// the local format marker and remain byte-for-byte unchanged.
func expandOpenAICompactionV2CompatInput(body []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	items := input.Array()
	var expanded bytes.Buffer
	expanded.WriteByte('[')
	wrote := 0
	changed := false
	writeItem := func(raw string) {
		if wrote > 0 {
			expanded.WriteByte(',')
		}
		expanded.WriteString(raw)
		wrote++
	}
	for _, item := range items {
		if restored, ok := decodeOpenAICompactionV2CompatItem(item); ok {
			for _, restoredItem := range restored {
				writeItem(restoredItem.Raw)
			}
			changed = true
			continue
		}
		writeItem(item.Raw)
	}
	expanded.WriteByte(']')
	if !changed {
		return body, false, nil
	}
	updated, err := sjson.SetRawBytes(body, "input", expanded.Bytes())
	if err != nil {
		return nil, false, fmt.Errorf("expand remote compaction v2 compatibility input: %w", err)
	}
	return updated, true, nil
}

func normalizeOpenAICompactionV2ResponseBody(body []byte, downstreamStream bool) ([]byte, bool, error) {
	// Providers occasionally label a completed JSON response as event-stream.
	// The actual framing is authoritative here because a mislabeled JSON body
	// can still be adapted into the SSE shape Codex requested.
	isSSE := bodyHasSSEFraming(body)
	bodyText := string(body)
	finalResponse, ok := extractCodexFinalResponse(bodyText)
	if !ok {
		return nil, false, fmt.Errorf("remote compaction v2 response has no response.completed payload")
	}
	stats := inspectOpenAICompactionV2JSON(finalResponse)
	if isSSE {
		stats = inspectOpenAICompactionV2SSE(body)
	}
	if !stats.SawCompleted {
		return nil, false, fmt.Errorf("%s", openAICompactionV2ResponseError(stats))
	}
	if stats.CompactionCount == 1 {
		if downstreamStream {
			if isSSE {
				return body, false, nil
			}
			converted, convertErr := openAIResponsesSSEBytesFromJSON(finalResponse)
			if convertErr != nil {
				return nil, false, fmt.Errorf("convert remote compaction v2 response to SSE: %w", convertErr)
			}
			return converted, true, nil
		}
		return finalResponse, isSSE, nil
	}

	item, err := buildOpenAICompactionV2CompatItem(bodyText, finalResponse)
	if err != nil {
		return nil, false, err
	}
	patched, err := sjson.SetRawBytes(finalResponse, "output", []byte("["+string(item)+"]"))
	if err != nil {
		return nil, false, fmt.Errorf("patch remote compaction v2 response output: %w", err)
	}
	if downstreamStream {
		converted, convertErr := openAIResponsesSSEBytesFromJSON(patched)
		if convertErr != nil {
			return nil, false, fmt.Errorf("convert remote compaction v2 compatibility response to SSE: %w", convertErr)
		}
		return converted, true, nil
	}
	return patched, true, nil
}

func (s *OpenAIGatewayService) normalizeOpenAICompactionV2Response(resp *http.Response, downstreamStream bool) error {
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("remote compaction v2 upstream response has no body")
	}
	body, err := readUpstreamResponseBodyLimited(resp.Body, resolveUpstreamResponseReadLimit(s.cfg))
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read remote compaction v2 upstream response: %w", err)
	}
	normalized, changed, err := normalizeOpenAICompactionV2ResponseBody(body, downstreamStream)
	if err != nil {
		return err
	}
	if changed {
		resp.Header.Del("Content-Length")
		resp.Header.Del("Content-Encoding")
		resp.ContentLength = -1
	}
	if bodyHasSSEFraming(normalized) {
		resp.Header.Set("Content-Type", "text/event-stream")
	} else {
		resp.Header.Set("Content-Type", "application/json")
	}
	resp.Body = io.NopCloser(bytes.NewReader(normalized))
	return nil
}

// openAIResponsesSSEBytesFromJSON is the in-memory counterpart of
// writeOpenAIResponsesSSEFromJSON, used before the normal streaming handler
// starts writing to the client.
func openAIResponsesSSEBytesFromJSON(body []byte) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid OpenAI response JSON for SSE framing")
	}
	eventType := openAIResponsesTerminalEventForBody(body)
	terminalPayload, err := json.Marshal(struct {
		Type     string          `json:"type"`
		Response json.RawMessage `json:"response"`
	}{Type: eventType, Response: json.RawMessage(body)})
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for i, item := range openAIResponsesOutputItemsFromBody(body) {
		payload, marshalErr := json.Marshal(struct {
			Type        string          `json:"type"`
			OutputIndex int             `json:"output_index"`
			Item        json.RawMessage `json:"item"`
		}{Type: "response.output_item.done", OutputIndex: i, Item: item})
		if marshalErr != nil {
			return nil, marshalErr
		}
		fmt.Fprint(&out, "event: response.output_item.done\n")
		fmt.Fprintf(&out, "data: %s\n\n", payload)
	}
	fmt.Fprintf(&out, "event: %s\n", eventType)
	fmt.Fprintf(&out, "data: %s\n\n", terminalPayload)
	fmt.Fprint(&out, "data: [DONE]\n\n")
	return out.Bytes(), nil
}
