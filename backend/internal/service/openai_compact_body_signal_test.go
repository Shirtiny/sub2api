//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasCompactionTriggerInInput_DetectsCompactSignal(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"stream":true,
		"input":[
			{"type":"message","role":"user","content":"hello"},
			{"type":"compaction_trigger"}
		]
	}`)
	require.True(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_NoTrigger(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"type":"message","role":"user","content":"hello"}
		]
	}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_EmptyInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[]}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_NoInputField(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5"}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_EmptyBody(t *testing.T) {
	require.False(t, HasCompactionTriggerInInput(nil))
	require.False(t, HasCompactionTriggerInInput([]byte{}))
}

func TestHasCompactionTriggerInInput_StringInput(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":"compaction_trigger"}`)
	require.False(t, HasCompactionTriggerInInput(body))
}

func TestHasCompactionTriggerInInput_CompactTriggerOnly(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"compaction_trigger"}]}`)
	require.True(t, HasCompactionTriggerInInput(body))
}

func TestHasPiContextSummarizationPromptInInput_StringContent(t *testing.T) {
	body := []byte(`{"input":[{"role":"system","content":"` + strings.ReplaceAll(piContextSummarizationSystemPrompt, "\n", `\n`) + `"},{"role":"user","content":[{"type":"input_text","text":"short history"}]}]}`)
	require.True(t, hasPiContextSummarizationPromptInInput(body))
}

func TestHasPiContextSummarizationPromptInInput_DeveloperTextBlock(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"input": []any{
			map[string]any{"role": "developer", "content": []any{map[string]any{"type": "input_text", "text": piContextSummarizationSystemPrompt}}},
			map[string]any{"role": "user", "content": "short history"},
		},
	})
	require.NoError(t, err)
	require.True(t, hasPiContextSummarizationPromptInInput(body))
}

func TestHasPiContextSummarizationPromptInInput_RejectsOrdinarySummaryPrompt(t *testing.T) {
	body := []byte(`{"input":[{"role":"system","content":"Summarize this conversation."},{"role":"user","content":"short history"}]}`)
	require.False(t, hasPiContextSummarizationPromptInInput(body))
}
