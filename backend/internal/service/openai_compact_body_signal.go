package service

import (
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// OpenAIRequestKind classifies the wire contract expected by the client. The
// classification is stored on the request context so every account attempt and
// forwarding branch uses the same decision.
type OpenAIRequestKind uint8

const (
	OpenAIRequestKindStandard OpenAIRequestKind = iota
	OpenAIRequestKindRemoteCompactionV2
)

const openAIRequestKindContextKey = "openai_request_kind"

// HasCompactionTriggerInInput detects the Codex remote compact v2 body signal:
// an input item with type "compaction_trigger". Official Codex now sends this
// on a normal POST /v1/responses (plus x-codex-beta-features /
// x-codex-turn-metadata). That request must stay on /responses and must not
// be rewritten onto the legacy POST /v1/responses/compact V1 path.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}

func ClassifyOpenAIRequest(body []byte) OpenAIRequestKind {
	if HasCompactionTriggerInInput(body) {
		return OpenAIRequestKindRemoteCompactionV2
	}
	return OpenAIRequestKindStandard
}

func SetOpenAIRequestKind(c *gin.Context, kind OpenAIRequestKind) {
	if c == nil {
		return
	}
	c.Set(openAIRequestKindContextKey, kind)
}

func GetOpenAIRequestKind(c *gin.Context) OpenAIRequestKind {
	if c == nil {
		return OpenAIRequestKindStandard
	}
	value, ok := c.Get(openAIRequestKindContextKey)
	if !ok {
		return OpenAIRequestKindStandard
	}
	switch kind := value.(type) {
	case OpenAIRequestKind:
		return kind
	case int:
		return OpenAIRequestKind(kind)
	default:
		return OpenAIRequestKindStandard
	}
}

func IsOpenAIRemoteCompactionV2(c *gin.Context) bool {
	return GetOpenAIRequestKind(c) == OpenAIRequestKindRemoteCompactionV2
}
