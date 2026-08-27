package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	requestControlMetadataMaxHeaders      = 32
	requestControlMetadataMaxHeaderRunes  = 512
	requestControlMetadataMaxFieldRunes   = 128
	requestControlMetadataMaxArraySamples = 32
	requestControlMetadataMaxFields       = 32
	requestControlMetadataMaxJSONBytes    = 8192
	requestControlMetadataMaxBodyBytes    = 2 * 1024 * 1024
)

func buildRequestControlMetadata(input RequestControlCheckInput) (map[string]string, map[string]any) {
	headers := input.MetadataHeaders
	if headers == nil {
		headers = input.Headers
	}
	metadata := requestControlBodyMetadata(input.Protocol, input.Body)
	if input.Protocol == RequestControlProtocolResponse {
		inspection := inspectRequestControlResponseSessionDetails(input)
		metadata["client_session_present"] = inspection.SessionPresent
		sessionSource := inspection.SessionSource
		if sessionSource == "" {
			sessionSource = "none"
		}
		metadata["client_session_source"] = sessionSource
		kind, confidence, evidence := requestControlResponseRequestKind(input, inspection.Body, inspection.BodyParsed, inspection.BodyErr)
		metadata["response_request_kind"] = kind
		metadata["response_request_kind_confidence"] = confidence
		if len(evidence) > 0 {
			metadata["response_request_kind_evidence"] = evidence
		}
	}
	return requestControlHeaderMetadata(headers), boundRequestControlBodyMetadata(metadata)
}

func requestControlHeaderMetadata(headers http.Header) map[string]string {
	out := make(map[string]string)
	keys := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for key := range headers {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		keys = append(keys, normalized)
	}
	sort.Strings(keys)
	truncated := len(keys) > requestControlMetadataMaxHeaders
	limit := requestControlMetadataMaxHeaders
	if truncated {
		limit--
	}
	for _, key := range keys {
		originalKeys := make([]string, 0, len(headers))
		for original := range headers {
			if strings.EqualFold(original, key) {
				originalKeys = append(originalKeys, original)
			}
		}
		sort.Strings(originalKeys)
		values := make([]string, 0, 1)
		for _, original := range originalKeys {
			for _, candidate := range headers[original] {
				values = append(values, truncateRequestControlValue(candidate, requestControlMetadataMaxHeaderRunes))
			}
		}
		if len(values) > 8 {
			values = values[:8]
		}
		sort.Strings(values)
		value := "[redacted]"
		if !requestControlSensitiveHeader(key) {
			if summary, ok := requestControlHeaderTurnMetadataSummary(key, headers); ok {
				value = summary
			} else {
				rawValue := strings.TrimSpace(strings.Join(values, ", "))
				if strings.HasPrefix(strings.ToLower(rawValue), "bearer ") || strings.HasPrefix(strings.ToLower(rawValue), "basic ") {
					value = "[redacted]"
				} else {
					value = requestControlMetadataString(logredact.RedactText(rawValue))
				}
			}
		}
		out[key] = truncateRequestControlValue(value, requestControlMetadataMaxHeaderRunes)
		if len(out) >= limit {
			break
		}
	}
	if truncated {
		out["x-request-control-metadata-truncated"] = "true"
	}
	return out
}

// Summarize the potentially large Codex compatibility header before applying
// the generic header-value bound. This keeps audit details useful and avoids
// treating a valid JSON blob as malformed merely because its display value was
// truncated.
func requestControlHeaderTurnMetadataSummary(name string, headers http.Header) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(name), "x-codex-turn-metadata") {
		return "", false
	}
	values := requestControlHeaderValues(headers, name)
	if len(values) != 1 {
		return "", false
	}
	raw := strings.TrimSpace(values[0])
	if strings.HasPrefix(raw, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return "", false
		}
		raw = decoded
	}
	if _, ok := parseRequestControlTurnMetadata(raw); !ok {
		return "", false
	}
	encoded, err := json.Marshal(requestControlTurnMetadataSummary(raw))
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func requestControlSensitiveHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, marker := range []string{
		"authorization",
		"auth",
		"api-key",
		"apikey",
		"key",
		"cookie",
		"password",
		"secret",
		"token",
		"credential",
		"signature",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func requestControlMetadataString(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(strings.TrimSpace(value))
	return truncateRequestControlValue(value, requestControlMetadataMaxFieldRunes)
}

func requestControlBodyMetadata(protocol string, raw []byte) map[string]any {
	metadata := map[string]any{
		"body_bytes": len(raw),
		"protocol":   protocol,
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		metadata["parse"] = "empty"
		return metadata
	}
	if len(raw) > requestControlMetadataMaxBodyBytes {
		metadata["parse"] = "too_large"
		return metadata
	}

	fields := make([]string, 0, requestControlMetadataMaxFields)
	err := openaiwsv2.VisitTopLevelObjectFields(raw, func(key, rawValue []byte) error {
		name := string(key)
		// Collect all bounded top-level keys before sorting/truncating so the
		// metadata hash is independent of JSON field order.
		fields = append(fields, truncateRequestControlValue(name, requestControlMetadataMaxFieldRunes))
		requestControlAddBodyFieldMetadata(metadata, name, rawValue)
		return nil
	})
	sort.Strings(fields)
	if len(fields) > requestControlMetadataMaxFields {
		fields = fields[:requestControlMetadataMaxFields]
	}
	metadata["top_level_fields"] = fields
	if err != nil {
		metadata["parse"] = "invalid_or_unreadable"
	}
	return boundRequestControlBodyMetadata(metadata)
}

func requestControlAddBodyFieldMetadata(metadata map[string]any, name string, raw []byte) {
	switch name {
	case "model", "service_tier", "type":
		if value, ok := requestControlJSONScalarString(raw); ok {
			metadata[name] = value
		}
	case "stream", "store", "parallel_tool_calls":
		if value, ok := requestControlJSONScalar(raw); ok {
			metadata[name] = value
		}
	case "max_tokens", "max_output_tokens", "temperature", "top_p":
		if value, ok := requestControlJSONScalar(raw); ok {
			metadata[name] = value
		}
	case "tool_choice":
		metadata[name] = requestControlJSONValueSummary(raw)
	case "reasoning", "thinking", "output_config":
		metadata[name] = requestControlJSONObjectSummary(raw, []string{"effort", "summary", "type", "max_tokens"})
	case "include", "stop", "stop_sequences":
		metadata[name] = requestControlJSONArraySummary(raw, false)
	case "input", "messages", "system":
		metadata[name] = requestControlJSONArraySummary(raw, true)
	case "tools":
		metadata[name] = requestControlJSONArraySummary(raw, true)
	case "metadata", "conversationState":
		metadata[name] = requestControlJSONObjectKeys(raw)
	case "client_metadata":
		metadata[name] = requestControlClientMetadataSummary(raw)
	case "prompt_cache_key":
		if value, ok := requestControlJSONScalarString(raw); ok && value != "" {
			metadata["prompt_cache_key_present"] = true
		}
	case "previous_response_id":
		if value, ok := requestControlJSONScalarString(raw); ok && value != "" {
			metadata["previous_response_id_present"] = true
		}
	}
}

func requestControlJSONScalarString(raw []byte) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return requestControlMetadataString(value), true
}

func requestControlJSONScalar(raw []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	switch value.(type) {
	case nil, bool, json.Number:
		return value, true
	default:
		return nil, false
	}
}

func requestControlJSONValueSummary(raw []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return map[string]any{"kind": "invalid"}
	}
	switch value := value.(type) {
	case string:
		return requestControlMetadataString(value)
	case bool, json.Number:
		return value
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, truncateRequestControlValue(key, requestControlMetadataMaxFieldRunes))
		}
		sort.Strings(keys)
		if len(keys) > requestControlMetadataMaxFields {
			keys = keys[:requestControlMetadataMaxFields]
		}
		return map[string]any{"kind": "object", "keys": keys}
	case []any:
		return map[string]any{"kind": "array", "count": len(value)}
	default:
		return map[string]any{"kind": "other"}
	}
}

func requestControlJSONObjectKeys(raw []byte) map[string]any {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"kind": "invalid"}
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, truncateRequestControlValue(key, requestControlMetadataMaxFieldRunes))
	}
	sort.Strings(keys)
	if len(keys) > requestControlMetadataMaxFields {
		keys = keys[:requestControlMetadataMaxFields]
	}
	return map[string]any{"kind": "object", "keys": keys}
}

func requestControlJSONObjectSummary(raw []byte, valueKeys []string) map[string]any {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"kind": "invalid"}
	}
	result := requestControlJSONObjectKeys(raw)
	for _, key := range valueKeys {
		if rawValue, ok := value[key]; ok {
			if scalar, ok := requestControlJSONScalar(rawValue); ok {
				result[key] = scalar
				continue
			}
			if stringValue, ok := requestControlJSONScalarString(rawValue); ok {
				result[key] = stringValue
			}
		}
	}
	return result
}

func requestControlClientMetadataSummary(raw []byte) map[string]any {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"kind": "invalid"}
	}
	result := requestControlJSONObjectKeys(raw)
	identity := make(map[string]any)
	for _, key := range []string{
		"x-codex-installation-id", "installation_id", "session_id", "thread_id", "turn_id", "window_id", "x-codex-window-id", "request_kind",
	} {
		if rawValue, ok := value[key]; ok {
			if stringValue, ok := requestControlJSONScalarString(rawValue); ok {
				identity[key] = stringValue
			}
		}
	}
	if rawValue, ok := value["x-codex-turn-metadata"]; ok {
		trimmed := bytes.TrimSpace(rawValue)
		var turnMetadata string
		if err := json.Unmarshal(rawValue, &turnMetadata); err == nil {
			// Parse before applying the display-length bound. The full metadata
			// blob is needed to distinguish valid Desktop payloads from malformed
			// ones; individual summary values are bounded below.
			identity["x-codex-turn-metadata"] = requestControlTurnMetadataSummary(turnMetadata)
		} else if len(trimmed) > 0 && trimmed[0] == '{' && openaiwsv2.ValidateTopLevelObject(trimmed) == nil {
			identity["x-codex-turn-metadata"] = requestControlTurnMetadataSummaryJSON(trimmed)
		}
	}
	if len(identity) > 0 {
		result["identity"] = identity
	}
	return result
}

func requestControlTurnMetadataSummary(raw string) map[string]any {
	return requestControlTurnMetadataSummaryJSON([]byte(raw))
}

func requestControlTurnMetadataSummaryJSON(raw []byte) map[string]any {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{"kind": "invalid"}
	}
	result := make(map[string]any)
	for _, key := range []string{"installation_id", "session_id", "thread_id", "turn_id", "window_id", "request_kind"} {
		if rawValue, ok := value[key]; ok {
			if stringValue, ok := requestControlJSONScalarString(rawValue); ok {
				result[key] = stringValue
			}
		}
	}
	return result
}

func requestControlJSONArraySummary(raw []byte, inspectItems bool) map[string]any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return map[string]any{"kind": "invalid"}
	}
	result := map[string]any{"kind": "array"}
	if !inspectItems {
		count := 0
		for decoder.More() {
			var item json.RawMessage
			if decoder.Decode(&item) != nil {
				return map[string]any{"kind": "invalid"}
			}
			count++
			if count >= requestControlMetadataMaxArraySamples {
				result["count_at_least"] = count
				result["truncated"] = true
				return result
			}
		}
		result["count"] = count
		return result
	}

	types := make(map[string]struct{})
	roles := make(map[string]struct{})
	count := 0
	for decoder.More() {
		var item json.RawMessage
		if decoder.Decode(&item) != nil {
			return map[string]any{"kind": "invalid"}
		}
		count++
		if count > requestControlMetadataMaxArraySamples {
			result["count_at_least"] = count
			result["truncated"] = true
			break
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(item, &object) != nil {
			continue
		}
		if value, ok := object["type"]; ok {
			if typeName, ok := requestControlJSONScalarString(value); ok && typeName != "" {
				types[typeName] = struct{}{}
			}
		}
		if value, ok := object["role"]; ok {
			if role, ok := requestControlJSONScalarString(value); ok && role != "" {
				roles[role] = struct{}{}
			}
		}
	}
	if _, truncated := result["truncated"]; !truncated {
		result["count"] = count
	}
	result["types"] = sortedMetadataSet(types)
	result["roles"] = sortedMetadataSet(roles)
	return result
}

func sortedMetadataSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, truncateRequestControlValue(value, requestControlMetadataMaxFieldRunes))
	}
	sort.Strings(result)
	if len(result) > requestControlMetadataMaxFields {
		result = result[:requestControlMetadataMaxFields]
	}
	return result
}

func boundRequestControlBodyMetadata(metadata map[string]any) map[string]any {
	raw, err := json.Marshal(metadata)
	if err == nil && len(raw) <= requestControlMetadataMaxJSONBytes {
		return metadata
	}
	return map[string]any{
		"body_bytes":                       metadata["body_bytes"],
		"protocol":                         metadata["protocol"],
		"top_level_fields":                 metadata["top_level_fields"],
		"client_session_present":           metadata["client_session_present"],
		"client_session_source":            metadata["client_session_source"],
		"response_request_kind":            metadata["response_request_kind"],
		"response_request_kind_confidence": metadata["response_request_kind_confidence"],
		"response_request_kind_evidence":   metadata["response_request_kind_evidence"],
		"metadata_truncated":               true,
	}
}
