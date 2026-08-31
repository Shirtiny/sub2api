package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const piContextSummarizationSystemPrompt = `You are a context summarization assistant. Your task is to read a conversation between a user and an AI assistant, then produce a structured summary following the exact format specified.

Do NOT continue the conversation. Do NOT respond to any questions in the conversation. ONLY output the structured summary.`

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

// hasPiContextSummarizationPromptInInput detects Pi's fixed local-compaction
// prompt. Pi sends these summaries to the normal Responses endpoint without a
// session identifier, tools, prompt_cache_key, or an explicit compaction
// header. The surrounding request-shape checks remain in request control so a
// matching prompt alone cannot classify an arbitrary request as compaction.
func hasPiContextSummarizationPromptInInput(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	items := input.Array()
	if len(items) != 2 {
		return false
	}

	instruction := items[0]
	role := instruction.Get("role").String()
	if role != "system" && role != "developer" {
		return false
	}
	if items[1].Get("role").String() != "user" {
		return false
	}
	userContent := items[1].Get("content")
	if userContent.Type != gjson.String && !userContent.IsArray() {
		return false
	}

	text, ok := openAIInputMessageText(instruction)
	return ok && strings.TrimSpace(text) == piContextSummarizationSystemPrompt
}

func openAIInputMessageText(message gjson.Result) (string, bool) {
	content := message.Get("content")
	if content.Type == gjson.String {
		return content.String(), true
	}
	if !content.IsArray() {
		return "", false
	}
	blocks := content.Array()
	if len(blocks) != 1 {
		return "", false
	}
	blockType := blocks[0].Get("type").String()
	if blockType != "input_text" && blockType != "text" {
		return "", false
	}
	text := blocks[0].Get("text")
	if text.Type != gjson.String {
		return "", false
	}
	return text.String(), true
}
