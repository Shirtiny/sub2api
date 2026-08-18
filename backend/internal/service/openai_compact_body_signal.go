package service

import "github.com/tidwall/gjson"

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
