package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	requestControlSnapshotMaxHeaders      = 64
	requestControlSnapshotMaxHeaderValues = 8
	requestControlSnapshotMaxHeaderRunes  = 1024
	requestControlSnapshotMaxBodyBytes    = 256 * 1024
)

type RequestControlRequestSnapshot struct {
	Available         bool                `json:"available"`
	Method            string              `json:"method"`
	Host              string              `json:"host"`
	Path              string              `json:"path"`
	RawQuery          string              `json:"raw_query"`
	ClientIP          string              `json:"client_ip"`
	RemoteAddr        string              `json:"remote_addr"`
	ContentLength     int64               `json:"content_length"`
	Headers           map[string][]string `json:"headers"`
	Body              string              `json:"body"`
	BodyBytes         int                 `json:"body_bytes"`
	BodyCapturedBytes int                 `json:"body_captured_bytes"`
	BodyTruncated     bool                `json:"body_truncated"`
	BodySHA256        string              `json:"body_sha256"`
	BodyCaptureMode   string              `json:"body_capture_mode"`
	CapturedAt        time.Time           `json:"captured_at"`
}

func buildRequestControlRequestSnapshot(input RequestControlCheckInput) RequestControlRequestSnapshot {
	headers := input.MetadataHeaders
	if headers == nil {
		headers = input.Headers
	}
	capturedBody, truncated, _ := captureRequestControlBody(input.Body)
	redactedBody := redactRequestControlBody([]byte(capturedBody))
	body, redactionTruncated, _ := captureRequestControlBody(redactedBody)
	truncated = truncated || redactionTruncated
	digest := sha256.Sum256(input.Body)
	return RequestControlRequestSnapshot{
		Available:         true,
		Method:            truncateRequestControlValue(input.RequestMethod, 32),
		Host:              truncateRequestControlValue(input.RequestHost, 512),
		Path:              truncateRequestControlValue(input.RequestPath, 2048),
		RawQuery:          requestControlSnapshotQuery(input.RequestQuery),
		ClientIP:          truncateRequestControlValue(input.ClientIP, 128),
		RemoteAddr:        truncateRequestControlValue(input.RemoteAddr, 256),
		ContentLength:     input.ContentLength,
		Headers:           requestControlSnapshotHeaders(headers),
		Body:              body,
		BodyBytes:         len(input.Body),
		BodyCapturedBytes: len(body),
		BodyTruncated:     truncated,
		BodySHA256:        hex.EncodeToString(digest[:]),
		BodyCaptureMode:   requestControlBodyCaptureMode(truncated),
	}
}

func requestControlSnapshotQuery(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return truncateRequestControlValue(logredact.RedactText(raw,
			"authorization", "api_key", "apikey", "key", "token", "secret", "cookie"), 4096)
	}
	for key := range values {
		if requestControlSnapshotSensitiveQueryKey(key) {
			values[key] = []string{"[redacted]"}
		}
	}
	return truncateRequestControlValue(values.Encode(), 4096)
}

func redactRequestControlBody(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	if !requestControlSnapshotBodyMayContainCredential(raw) {
		return append([]byte(nil), raw...)
	}
	// Keep prompts and tool arguments intact while removing credentials that
	// occasionally appear in compatibility payloads before persistence.
	redacted := logredact.RedactText(string(raw),
		"api_key", "apikey", "x-api-key", "access_token", "refresh_token", "id_token",
		"apiKey", "accessToken", "refreshToken", "idToken", "clientSecret", "sessionToken",
		"session_token", "bearer", "credential", "secret", "client_secret", "password",
		"authorization", "cookie", "private_key")
	return []byte(redacted)
}

func requestControlSnapshotBodyMayContainCredential(raw []byte) bool {
	// Avoid a second full JSON decode on the request-control hot path. A false
	// positive merely canonicalizes/redacts the snapshot; a false negative is
	// avoided by covering both snake_case and camelCase spellings.
	lower := bytes.ToLower(raw)
	for _, marker := range []string{
		`"authorization"`, `"api_key"`, `"apikey"`, `"x-api-key"`, `"access_token"`, `"accesstoken"`,
		`"refresh_token"`, `"refreshtoken"`, `"id_token"`, `"idtoken"`, `"client_secret"`, `"clientsecret"`,
		`"session_token"`, `"sessiontoken"`, `"password"`, `"passwd"`, `"pwd"`, `"cookie"`, `"set-cookie"`,
		`"token"`, `"secret"`, `"credential"`, `"bearer"`, `"private_key"`, `"privatekey"`,
	} {
		if bytes.Contains(lower, []byte(marker)) {
			return true
		}
	}
	return !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) &&
		(bytes.Contains(lower, []byte("authorization=")) || bytes.Contains(lower, []byte("password=")) || bytes.Contains(lower, []byte("token=")))
}

func requestControlSnapshotSensitiveQueryKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "key" || requestControlSnapshotSensitiveFieldKey(normalized)
}

func requestControlSnapshotSensitiveFieldKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	compact := strings.NewReplacer("_", "", "-", "").Replace(normalized)
	switch compact {
	case "authorization", "apikey", "xapikey", "accesstoken", "refreshtoken", "idtoken", "clientsecret", "sessiontoken", "password", "passwd", "pwd", "cookie", "setcookie", "token", "secret", "credential", "bearer", "privatekey":
		return true
	default:
		return false
	}
}

func requestControlBodyCaptureMode(truncated bool) string {
	if truncated {
		return "head_tail"
	}
	return "full"
}

func requestControlSnapshotHeaders(headers http.Header) map[string][]string {
	out := make(map[string][]string)
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
	if len(keys) > requestControlSnapshotMaxHeaders {
		keys = keys[:requestControlSnapshotMaxHeaders]
		out["x-request-control-headers-truncated"] = []string{"true"}
	}
	for _, key := range keys {
		if requestControlSensitiveHeader(key) {
			out[key] = []string{"[redacted]"}
			continue
		}
		values := make([]string, 0, 1)
		for original, candidates := range headers {
			if !strings.EqualFold(original, key) {
				continue
			}
			for _, candidate := range candidates {
				values = append(values, truncateRequestControlValue(candidate, requestControlSnapshotMaxHeaderRunes))
				if len(values) >= requestControlSnapshotMaxHeaderValues {
					break
				}
			}
			if len(values) >= requestControlSnapshotMaxHeaderValues {
				break
			}
		}
		out[key] = values
	}
	return out
}

func captureRequestControlBody(raw []byte) (string, bool, int) {
	if len(raw) <= requestControlSnapshotMaxBodyBytes {
		return strings.ToValidUTF8(string(raw), "�"), false, len(raw)
	}
	omitted := len(raw) - requestControlSnapshotMaxBodyBytes
	var marker string
	var headBytes, tailBytes int
	for range 3 {
		marker = requestControlBodyOmittedMarker(omitted)
		payloadBytes := requestControlSnapshotMaxBodyBytes - len(marker)
		headBytes = payloadBytes / 2
		tailBytes = payloadBytes - headBytes
		nextOmitted := len(raw) - headBytes - tailBytes
		if nextOmitted == omitted {
			break
		}
		omitted = nextOmitted
	}
	head := requestControlValidUTF8Prefix(raw[:headBytes])
	tail := requestControlValidUTF8Suffix(raw[len(raw)-tailBytes:])
	body := head + marker + tail
	return body, true, len(body)
}

func requestControlBodyOmittedMarker(bytes int) string {
	return "\n\n...[request-control omitted " + strconv.Itoa(bytes) + " bytes]...\n\n"
}

func requestControlValidUTF8Prefix(raw []byte) string {
	for len(raw) > 0 && !utf8.Valid(raw) {
		raw = raw[:len(raw)-1]
	}
	return string(raw)
}

func requestControlValidUTF8Suffix(raw []byte) string {
	for len(raw) > 0 && !utf8.Valid(raw) {
		raw = raw[1:]
	}
	return string(raw)
}
