package openai_ws_v2

import "bytes"

type fastUpstreamEventEnvelope struct {
	eventType      string
	responseIDRaw  []byte
	topLevelIDRaw  []byte
	nestedResponse bool
}

// inspectFastUpstreamEventEnvelope handles the canonical official event shape
// without materializing delta text. Non-canonical order/escaping falls back to
// the existing complete gjson classifier.
func inspectFastUpstreamEventEnvelope(payload []byte) (fastUpstreamEventEnvelope, bool) {
	result := fastUpstreamEventEnvelope{}
	parser := clientJSONParser{data: payload}
	parser.skipSpace()
	if !parser.consumeByte('{') {
		return result, false
	}
	parser.skipSpace()
	key, err := parser.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
	if err != nil || !rawClientJSONStringEqual(payload, key, "type") {
		return result, false
	}
	parser.skipSpace()
	if !parser.consumeByte(':') {
		return result, false
	}
	parser.skipSpace()
	typeMeta, err := parser.scanString(responseEventTypeMaxBytes, responseEventTypeMaxBytes*6)
	if err != nil || bytes.IndexByte(payload[typeMeta.contentStart:typeMeta.contentEnd], '\\') >= 0 {
		return result, false
	}
	result.eventType = canonicalUpstreamEventType(payload[typeMeta.contentStart:typeMeta.contentEnd])
	if result.eventType == "" {
		return result, false
	}

	for {
		parser.skipSpace()
		if parser.consumeByte('}') {
			break
		}
		if !parser.consumeByte(',') {
			return result, false
		}
		parser.skipSpace()
		key, err = parser.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return result, false
		}
		parser.skipSpace()
		if !parser.consumeByte(':') {
			return result, false
		}
		parser.skipSpace()
		switch {
		case rawClientJSONStringEqual(payload, key, "response_id"):
			responseID, ok := scanFastProtocolString(&parser, responseStepIDMaxBytes)
			if !ok {
				return result, false
			}
			result.responseIDRaw = responseID
			return result, true
		case rawClientJSONStringEqual(payload, key, "response"):
			responseID, found, ok := scanFastNestedResponseID(&parser)
			if !ok {
				return result, false
			}
			result.nestedResponse = true
			if found {
				result.responseIDRaw = responseID
				return result, true
			}
		case rawClientJSONStringEqual(payload, key, "id"):
			value, ok := scanFastProtocolString(&parser, responseStepIDMaxBytes)
			if !ok {
				return result, false
			}
			result.topLevelIDRaw = value
		default:
			if err := parser.scanValue(1); err != nil {
				return result, false
			}
		}
	}
	if len(result.responseIDRaw) == 0 && result.eventType != "error" && isTerminalEvent(result.eventType) {
		result.responseIDRaw = result.topLevelIDRaw
	}
	return result, true
}

func rawClientJSONStringEqual(payload []byte, meta clientJSONStringMeta, expected string) bool {
	raw := payload[meta.contentStart:meta.contentEnd]
	if bytes.IndexByte(raw, '\\') < 0 {
		return len(raw) == len(expected) && bytes.Equal(raw, []byte(expected))
	}
	if meta.decodedLen != len(expected) {
		return false
	}
	var decoded [ClientEnvelopeMaxKeyBytes]byte
	length := decodeClientJSONStringInto(payload, meta, decoded[:])
	return length == len(expected) && bytes.Equal(decoded[:length], []byte(expected))
}

func scanFastProtocolString(parser *clientJSONParser, maxBytes int) ([]byte, bool) {
	if parser == nil || parser.pos >= len(parser.data) || parser.data[parser.pos] != '"' {
		return nil, false
	}
	meta, err := parser.scanString(maxBytes, maxBytes*6)
	if err != nil {
		return nil, false
	}
	raw := parser.data[meta.contentStart:meta.contentEnd]
	if len(raw) == 0 || len(raw) > maxBytes || bytes.IndexByte(raw, '\\') >= 0 {
		return nil, false
	}
	for _, value := range raw {
		if value > 0x7f {
			return nil, false
		}
	}
	return raw, true
}

func scanFastNestedResponseID(parser *clientJSONParser) ([]byte, bool, bool) {
	if parser == nil || parser.pos >= len(parser.data) || parser.data[parser.pos] != '{' {
		if parser != nil {
			if err := parser.scanValue(1); err != nil {
				return nil, false, false
			}
		}
		return nil, false, true
	}
	parser.pos++
	for {
		parser.skipSpace()
		if parser.consumeByte('}') {
			return nil, false, true
		}
		key, err := parser.scanString(ClientEnvelopeMaxKeyBytes, clientEnvelopeMaxEncodedKeyBytes)
		if err != nil {
			return nil, false, false
		}
		parser.skipSpace()
		if !parser.consumeByte(':') {
			return nil, false, false
		}
		parser.skipSpace()
		if rawClientJSONStringEqual(parser.data, key, "id") {
			value, ok := scanFastProtocolString(parser, responseStepIDMaxBytes)
			return value, ok, ok
		}
		if err := parser.scanValue(2); err != nil {
			return nil, false, false
		}
		parser.skipSpace()
		if parser.consumeByte('}') {
			return nil, false, true
		}
		if !parser.consumeByte(',') {
			return nil, false, false
		}
	}
}

func canonicalUpstreamEventType(raw []byte) string {
	for _, candidate := range [...]string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.output_item.done",
		"response.content_part.added",
		"response.content_part.done",
		"response.output_text.delta",
		"response.output_text.done",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_part.done",
		"response.reasoning_summary_text.delta",
		"response.reasoning_summary_text.done",
		"response.reasoning_text.delta",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.custom_tool_call_input.delta",
		"response.completed",
		"response.failed",
		"response.incomplete",
		"response.done",
		"response.cancelled",
		"response.canceled",
		"error",
	} {
		if len(raw) == len(candidate) && bytes.Equal(raw, []byte(candidate)) {
			return candidate
		}
	}
	return ""
}
