package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildUsageWhereGroupFilterIncludesCustomSubscriptionGroups(t *testing.T) {
	groupID := int64(42)
	_, where, args, _ := buildUsageWhere(&service.OpsDashboardFilter{GroupID: &groupID}, time.Unix(1, 0), time.Unix(2, 0), 1)

	for _, want := range []string{
		"ul.group_id = $3",
		"custom_source_group_id = $3",
		"is_custom_subscription_group = TRUE",
	} {
		if !strings.Contains(where, want) {
			t.Fatalf("usage where missing %q\nfull: %s", want, where)
		}
	}
	if len(args) != 3 || args[2] != groupID {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildErrorWhereGroupFilterIncludesCustomSubscriptionGroups(t *testing.T) {
	groupID := int64(42)
	where, args, _ := buildErrorWhere(&service.OpsDashboardFilter{GroupID: &groupID}, time.Unix(1, 0), time.Unix(2, 0), 1)

	for _, want := range []string{
		"group_id = $3",
		"custom_source_group_id = $3",
		"is_custom_subscription_group = TRUE",
	} {
		if !strings.Contains(where, want) {
			t.Fatalf("error where missing %q\nfull: %s", want, where)
		}
	}
	if len(args) != 3 || args[2] != groupID {
		t.Fatalf("unexpected args: %#v", args)
	}
}
