package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// Increment when the canonical profile below changes. Existing rows remain
// readable; new rows simply use a new unique fingerprint.
const requestControlDedupFingerprintVersion = 4

var requestControlDedupSessionHeaders = []string{
	"x-aether-session-id",
	"session-id",
	"session_id",
	"conversation-id",
	"conversation_id",
	"thread-id",
	"thread_id",
	"x-claude-code-session-id",
	"x-opencode-session-id",
}

// requestControlDedupHashes is a small test/helper entry point. Production
// logging uses requestControlDedupBodyHash with the same canonicalization.
func requestControlDedupHashes(input RequestControlCheckInput) (string, string) {
	_, body := buildRequestControlMetadata(input)
	return requestControlDedupHeaderHash(input), requestControlDedupBodyHash(input, body)
}

// requestControlDedupHeaderHash intentionally fingerprints the client profile,
// not concrete request IDs. The full redacted headers are still retained for
// detail inspection, but session/turn/proxy values must not create one row per
// turn.
func requestControlDedupHeaderHash(input RequestControlCheckInput) string {
	source := input.Headers
	if len(source) == 0 && len(input.MetadataHeaders) > 0 {
		source = input.MetadataHeaders
	}
	headers := cloneRequestControlHeaders(source)
	userAgent := firstRequestControlHeader(headers, "User-Agent")
	if userAgent == "" {
		userAgent = strings.TrimSpace(input.UserAgent)
	}
	originator := firstRequestControlHeader(headers, "originator")
	if originator == "" {
		originator = strings.TrimSpace(input.Originator)
	}

	profile := map[string]any{
		"fingerprint_version": requestControlDedupFingerprintVersion,
		"client_family":       requestControlDedupClientFamily(userAgent),
		"originator_family":   requestControlDedupClientFamily(originator),
		"transport":           requestControlDedupTransport(input.WebSocket),
		"content_type":        requestControlDedupMediaType(firstRequestControlHeader(headers, "Content-Type")),
		"accept_event_stream": requestControlHeaderTokenContains(firstRequestControlHeader(headers, "Accept"), "text/event-stream"),
		"responses_websocket": requestControlHeaderTokenContains(firstRequestControlHeader(headers, "OpenAI-Beta"), "responses_websockets=2026-02-06"),
		"codex_turn":          requestControlDedupHeaderValueShape(headers, "x-codex-turn-metadata"),
		"codex_installation":  requestControlDedupHeaderValueShape(headers, "x-codex-installation-id"),
		"codex_window":        requestControlDedupHeaderValueShape(headers, "x-codex-window-id"),
		"client_request_id":   requestControlDedupHeaderValueShape(headers, "x-client-request-id"),
		"session_signal":      requestControlDedupSessionSignal(headers),
		"claude_app":          strings.ToLower(firstRequestControlHeader(headers, "X-App")),
		"anthropic_version":   strings.ToLower(firstRequestControlHeader(headers, "anthropic-version")),
		"claude_code_beta":    requestControlHeaderTokenContains(firstRequestControlHeader(headers, "anthropic-beta"), "claude-code-20250219"),
	}
	return requestControlMetadataHash(profile)
}

func cloneRequestControlHeaders(source http.Header) http.Header {
	headers := make(http.Header, len(source))
	for key, values := range source {
		headers[key] = append([]string(nil), values...)
	}
	return headers
}

func firstRequestControlHeader(headers http.Header, name string) string {
	values := requestControlHeaderValues(headers, name)
	if len(values) != 1 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func requestControlDedupClientFamily(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "missing"
	}
	switch {
	case strings.HasPrefix(value, "codex desktop"), strings.HasPrefix(value, "codex_work_desktop"):
		return "codex_desktop"
	case strings.HasPrefix(value, "codex-tui/"):
		return "codex_tui"
	case strings.HasPrefix(value, "codex_cli_rs/"):
		return "codex_cli"
	case strings.HasPrefix(value, "codex_vscode/"):
		return "codex_vscode"
	case strings.HasPrefix(value, "codex_exec/"):
		return "codex_exec"
	case strings.HasPrefix(value, "codex_sdk_ts/"):
		return "codex_sdk"
	case strings.HasPrefix(value, "claude-code/"), strings.HasPrefix(value, "claude-cli/"), strings.HasPrefix(value, "claude code"):
		return "claude_code"
	}
	family := strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == ' ' || r == '('
	})
	if len(family) == 0 {
		return "unknown"
	}
	return truncateRequestControlValue(family[0], 64)
}

func requestControlDedupTransport(websocket bool) string {
	if websocket {
		return "websocket"
	}
	return "http"
}

func requestControlDedupMediaType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || strings.TrimSpace(mediaType) == "" {
		if strings.TrimSpace(value) == "" {
			return "missing"
		}
		return "invalid"
	}
	return strings.ToLower(mediaType)
}

func requestControlDedupSessionSignal(headers http.Header) string {
	for _, name := range requestControlDedupSessionHeaders {
		if len(requestControlHeaderValues(headers, name)) > 0 {
			return "present"
		}
	}
	return "missing"
}

func requestControlDedupHeaderValueShape(headers http.Header, name string) string {
	values := requestControlHeaderValues(headers, name)
	if len(values) == 0 {
		return "missing"
	}
	if len(values) != 1 {
		return "count:" + strconv.Itoa(len(values))
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return "empty"
	}
	if name == "x-codex-turn-metadata" {
		if metadata, ok := parseRequestControlTurnMetadata(value); ok {
			return "valid:" + metadata.RequestKind
		}
		if json.Valid([]byte(value)) {
			return "invalid_identity"
		}
		return "invalid_json"
	}
	if name == "x-codex-window-id" {
		root, _, _ := strings.Cut(value, ":")
		if requestControlUUID(root) {
			return "uuid_window"
		}
		return "nonempty"
	}
	if requestControlUUID(value) {
		return "uuid"
	}
	return "nonempty"
}

func requestControlDedupBodyHash(input RequestControlCheckInput, metadata map[string]any) string {
	fingerprint := requestControlDedupBodyMetadata(input, metadata)
	fingerprint["fingerprint_version"] = requestControlDedupFingerprintVersion
	return requestControlMetadataHash(fingerprint)
}

// requestControlDedupBodyMetadata keeps only the wire format. It deliberately
// drops model names, body byte size, message/tool counts, content, optional
// request fields, and concrete session/turn IDs; those are request data rather
// than client/format identity.
func requestControlDedupBodyMetadata(input RequestControlCheckInput, metadata map[string]any) map[string]any {
	profile := map[string]any{
		"protocol":  input.Protocol,
		"endpoint":  requestControlDedupEndpoint(input.Endpoint),
		"transport": requestControlDedupTransport(input.WebSocket),
		"parse":     requestControlDedupParseShape(metadata),
	}
	if requestKind, ok := metadata["response_request_kind"].(string); ok &&
		requestKind != "" && requestKind != "openai_responses_standard_or_unknown" {
		profile["response_request_kind"] = requestKind
	}
	if sessionPresent, ok := metadata["client_session_present"].(bool); ok && sessionPresent {
		profile["client_session_present"] = sessionPresent
	}
	for _, key := range []string{
		"input", "messages", "system", "reasoning", "thinking",
		"stream", "store", "parallel_tool_calls", "type",
	} {
		if value, ok := metadata[key]; ok {
			profile[key] = requestControlDedupTypeShape(value)
		}
	}
	return profile
}

func requestControlDedupEndpoint(endpoint string) string {
	path := strings.ToLower(strings.TrimRight(strings.TrimSpace(endpoint), "/"))
	switch {
	case strings.HasSuffix(path, "/responses/compact"):
		return "responses_compact"
	case strings.HasSuffix(path, "/responses"):
		return "responses"
	case strings.HasSuffix(path, "/chat/completions"):
		return "chat_completions"
	case strings.HasSuffix(path, "/messages"):
		return "messages"
	default:
		return "other"
	}
}

func requestControlDedupParseShape(metadata map[string]any) string {
	if parse, ok := metadata["parse"].(string); ok && parse != "" {
		return parse
	}
	if fields, ok := metadata["top_level_fields"].([]string); ok && len(fields) > 0 {
		return "object"
	}
	if fields, ok := metadata["top_level_fields"].([]any); ok && len(fields) > 0 {
		return "object"
	}
	return "empty_or_unknown"
}

func requestControlDedupTypeShape(value any) string {
	if summary, ok := value.(map[string]any); ok {
		if kind, exists := summary["kind"]; exists {
			return strings.TrimSpace(strings.ToLower(toRequestControlString(kind)))
		}
	}
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number, float32, float64, int, int32, int64, uint, uint32, uint64:
		return "number"
	case string:
		return "string"
	case []any, []string:
		return "array"
	case map[string]any:
		return "object"
	default:
		return "other"
	}
}

func toRequestControlString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return "other"
}

func requestControlMetadataHash(metadata any) string {
	raw, _ := json.Marshal(metadata)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
