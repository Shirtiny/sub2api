package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// opsCyberPolicyKey carries one upstream cyber-policy hit through the request
// lifecycle. The gateway records the hit after the response has been written,
// so notification and account side effects never delay the client response.
const opsCyberPolicyKey = "ops_cyber_policy"

// CyberPolicyMark is the bounded upstream evidence used by the risk-control
// recorder. Body is deliberately capped by the caller before it is stored.
type CyberPolicyMark struct {
	Code           string
	Message        string
	Body           string
	UpstreamStatus int
	UpstreamInTok  int
	UpstreamOutTok int
}

func MarkOpsCyberPolicy(c *gin.Context, mark CyberPolicyMark) {
	if c == nil || GetOpsCyberPolicy(c) != nil {
		return
	}
	mark.Code = "cyber_policy"
	mark.Message = strings.TrimSpace(mark.Message)
	mark.Body = strings.TrimSpace(mark.Body)
	c.Set(opsCyberPolicyKey, &mark)
}

func GetOpsCyberPolicy(c *gin.Context) *CyberPolicyMark {
	if c == nil {
		return nil
	}
	if value, ok := c.Get(opsCyberPolicyKey); ok {
		if mark, ok := value.(*CyberPolicyMark); ok && mark != nil {
			return mark
		}
	}
	return nil
}

func ClearOpsCyberPolicy(c *gin.Context) {
	if c != nil {
		c.Set(opsCyberPolicyKey, (*CyberPolicyMark)(nil))
	}
}

// DetectCyberPolicyResponse recognizes the canonical error code and the
// stable message markers emitted by the upstream Codex/Cyber policy path.
// Message-only matching is restricted to HTTP 400 or a structured error
// envelope so normal successful output that mentions "cyber" is never flagged.
func DetectCyberPolicyResponse(status int, payload []byte) (bool, string, string) {
	code := strings.TrimSpace(gjson.GetBytes(payload, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String())
	}
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "code").String())
	}
	if strings.EqualFold(code, "cyber_policy") {
		message := cyberPolicyErrorMessage(payload)
		return true, "cyber_policy", message
	}
	message := cyberPolicyErrorMessage(payload)
	markerText := string(payload)
	if status != http.StatusBadRequest {
		if !cyberPolicyStructuredErrorPayload(payload) {
			return false, "", ""
		}
		markerText = message
	}
	if strings.TrimSpace(markerText) == "" {
		return false, "", ""
	}
	normalized := strings.ToLower(markerText)
	for _, marker := range []string{
		"possible cybersecurity risk",
		"trusted access for cyber",
		"chatgpt.com/cyber",
	} {
		if strings.Contains(normalized, marker) {
			return true, "cyber_policy", message
		}
	}
	return false, "", ""
}

func cyberPolicyErrorMessage(payload []byte) string {
	for _, path := range []string{"error.message", "response.error.message", "message"} {
		if message := strings.TrimSpace(gjson.GetBytes(payload, path).String()); message != "" {
			return message
		}
	}
	return ""
}

func cyberPolicyStructuredErrorPayload(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	if gjson.GetBytes(payload, "error").Exists() || gjson.GetBytes(payload, "response.error").Exists() {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "type").String())) {
	case "error", "response.failed":
		return true
	default:
		return false
	}
}

func isCyberPolicyResponse(status int, payload []byte) bool {
	hit, _, _ := DetectCyberPolicyResponse(status, payload)
	return hit
}

func cyberPolicyClientMessage(message string) string {
	if message = strings.TrimSpace(message); message != "" {
		return message
	}
	return "Request blocked by upstream cyber-security policy"
}

func markOpenAIWSCyberPolicy(c *gin.Context, payload []byte, usage OpenAIUsage) bool {
	hit, _ := markOpenAIStreamingCyberPolicy(c, payload, usage)
	return hit
}

func markOpenAIStreamingCyberPolicy(c *gin.Context, payload []byte, usage OpenAIUsage) (bool, string) {
	hit, code, message := DetectCyberPolicyResponse(http.StatusOK, payload)
	if !hit {
		return false, ""
	}
	if payloadUsage, ok := extractOpenAIUsageFromJSONBytes(payload); ok {
		usage = payloadUsage
	}
	MarkOpsCyberPolicy(c, CyberPolicyMark{
		Code:           code,
		Message:        message,
		Body:           truncateString(string(payload), 4096),
		UpstreamStatus: http.StatusOK,
		UpstreamInTok:  usage.InputTokens,
		UpstreamOutTok: usage.OutputTokens,
	})
	return true, cyberPolicyClientMessage(message)
}
