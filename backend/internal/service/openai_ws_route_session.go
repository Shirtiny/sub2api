package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	openAIWSRouteSessionHashPrefix = "openai-ws-route-v1:"
	openAIWSRouteIdentityMaxBytes  = 512
)

// OpenAIWSRouteSessionIdentity is the authenticated, reconnect-stable identity
// for route-v1. Reliable is deliberately independent from the legacy sticky
// session heuristics: content and user-only fallbacks must never authorize a
// cross-connection migration.
type OpenAIWSRouteSessionIdentity struct {
	SessionKey       string
	SessionID        string
	ThreadID         string
	Reliable         bool
	ProjectedHeaders bool
	Reason           string
}

// ResolveOpenAIWSRouteSessionIdentity resolves the canonical Codex websocket
// identity once, after the first response.create is available. The official
// client body projection exists only for compatibility with Codex builds that
// omitted the dash-form headers. When projection succeeds, the canonical
// headers are installed on the inbound request so the Aether hop receives the
// same identity.
func ResolveOpenAIWSRouteSessionIdentity(
	c *gin.Context,
	envelope OpenAIWSClientEnvelope,
	groupID *int64,
	userID int64,
	apiKeyID int64,
) OpenAIWSRouteSessionIdentity {
	result := OpenAIWSRouteSessionIdentity{Reason: "identity_missing"}
	if c == nil || c.Request == nil {
		result.Reason = "request_missing"
		return result
	}

	sessionHeader, sessionPresent, sessionValid := singleOpenAIWSRouteIdentityHeader(c.Request.Header, "session-id")
	threadHeader, threadPresent, threadValid := singleOpenAIWSRouteIdentityHeader(c.Request.Header, "thread-id")
	clientRequestID, clientRequestPresent, clientRequestValid := singleOpenAIWSRouteIdentityHeader(c.Request.Header, "x-client-request-id")
	if !sessionValid || !threadValid || !clientRequestValid {
		result.Reason = "identity_header_invalid"
		return result
	}

	bodySessionID := envelope.ClientMetadataSessionID
	bodyThreadID := envelope.ClientMetadataThreadID
	bodyPresent := envelope.HasClientMetadataRouteIdentity
	if bodyPresent && sessionPresent && bodySessionID != sessionHeader {
		result.Reason = "session_identity_mismatch"
		return result
	}
	if bodyPresent && threadPresent && bodyThreadID != threadHeader {
		result.Reason = "thread_identity_mismatch"
		return result
	}

	projected := false
	if !sessionPresent || !threadPresent {
		if !hasPinnedCodexCLIRouteIdentityFingerprint(c.Request.Header) || !bodyPresent {
			result.Reason = "canonical_headers_missing"
			return result
		}
		if !sessionPresent {
			sessionHeader = bodySessionID
		}
		if !threadPresent {
			threadHeader = bodyThreadID
		}
		projected = true
	}

	if clientRequestPresent && clientRequestID != threadHeader {
		result.Reason = "client_request_thread_mismatch"
		return result
	}
	if projected {
		c.Request.Header.Set("session-id", sessionHeader)
		c.Request.Header.Set("thread-id", threadHeader)
	}

	result.SessionID = sessionHeader
	result.ThreadID = threadHeader
	result.SessionKey = hashOpenAIWSRouteSessionIdentity(groupID, userID, apiKeyID, sessionHeader, threadHeader)
	result.Reliable = result.SessionKey != ""
	result.ProjectedHeaders = projected
	result.Reason = "canonical_headers"
	if projected {
		result.Reason = "official_body_projection"
	}
	return result
}

func singleOpenAIWSRouteIdentityHeader(headers http.Header, name string) (value string, present bool, valid bool) {
	values := headers.Values(name)
	if len(values) == 0 {
		return "", false, true
	}
	if len(values) != 1 {
		return "", true, false
	}
	value = strings.TrimSpace(values[0])
	if value == "" || len(value) > openAIWSRouteIdentityMaxBytes {
		return "", true, false
	}
	return value, true, true
}

func hasPinnedCodexCLIRouteIdentityFingerprint(headers http.Header) bool {
	userAgents := headers.Values("user-agent")
	originators := headers.Values("originator")
	if len(userAgents) != 1 || len(originators) != 1 {
		return false
	}
	userAgent := strings.ToLower(strings.TrimSpace(userAgents[0]))
	originator := strings.ToLower(strings.TrimSpace(originators[0]))
	return strings.HasPrefix(userAgent, "codex_cli_rs/") && originator == "codex_cli_rs"
}

func hashOpenAIWSRouteSessionIdentity(groupID *int64, userID, apiKeyID int64, sessionID, threadID string) string {
	if sessionID == "" || threadID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("sub2api:openai-ws-route-session:v1"))
	var integer [8]byte
	if groupID == nil {
		_, _ = hash.Write([]byte{0})
	} else {
		_, _ = hash.Write([]byte{1})
		binary.BigEndian.PutUint64(integer[:], uint64(*groupID))
		_, _ = hash.Write(integer[:])
	}
	for _, value := range []uint64{uint64(userID), uint64(apiKeyID)} {
		binary.BigEndian.PutUint64(integer[:], value)
		_, _ = hash.Write(integer[:])
	}
	for _, value := range []string{sessionID, threadID} {
		binary.BigEndian.PutUint64(integer[:], uint64(len(value)))
		_, _ = hash.Write(integer[:])
		_, _ = hash.Write([]byte(value))
	}
	return openAIWSRouteSessionHashPrefix + hex.EncodeToString(hash.Sum(nil))
}
