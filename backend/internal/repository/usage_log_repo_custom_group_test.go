package repository

import (
	"strings"
	"testing"
)

func TestUsageGroupIDMatchesConditionIncludesCustomSubscriptionGroups(t *testing.T) {
	cond := usageGroupIDMatchesCondition("ul.group_id", 7)
	for _, want := range []string{
		"ul.group_id = $7",
		"ul.group_id IN",
		"is_custom_subscription_group = TRUE",
		"custom_source_group_id = $7",
	} {
		if !strings.Contains(cond, want) {
			t.Fatalf("condition missing %q\nfull: %s", want, cond)
		}
	}
}
