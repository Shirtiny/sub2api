package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type customGroupQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type accountGroupBindingTarget struct {
	GroupID  int64
	Priority int
}

func expandGroupIDsWithCustomGroups(ctx context.Context, q customGroupQueryer, groupIDs []int64) ([]int64, error) {
	normalized := uniquePositiveInt64s(groupIDs)
	if len(normalized) == 0 {
		return nil, nil
	}
	customBySource, err := loadCustomGroupIDsBySourceGroupIDs(ctx, q, normalized)
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(normalized))
	seen := make(map[int64]struct{}, len(normalized))
	appendID := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, gid := range normalized {
		appendID(gid)
		for _, customID := range customBySource[gid] {
			appendID(customID)
		}
	}
	return out, nil
}

func expandAccountGroupBindingsWithCustomGroups(ctx context.Context, q customGroupQueryer, groupIDs []int64) ([]accountGroupBindingTarget, error) {
	normalized := uniquePositiveInt64s(groupIDs)
	if len(normalized) == 0 {
		return nil, nil
	}
	customBySource, err := loadCustomGroupIDsBySourceGroupIDs(ctx, q, normalized)
	if err != nil {
		return nil, err
	}
	out := make([]accountGroupBindingTarget, 0, len(normalized))
	seen := make(map[int64]struct{}, len(normalized))
	appendBinding := func(id int64, priority int) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, accountGroupBindingTarget{GroupID: id, Priority: priority})
	}
	for i, gid := range normalized {
		priority := i + 1
		appendBinding(gid, priority)
		for _, customID := range customBySource[gid] {
			appendBinding(customID, priority)
		}
	}
	return out, nil
}

func loadCustomGroupIDsBySourceGroupIDs(ctx context.Context, q customGroupQueryer, sourceGroupIDs []int64) (map[int64][]int64, error) {
	sourceGroupIDs = uniquePositiveInt64s(sourceGroupIDs)
	out := make(map[int64][]int64, len(sourceGroupIDs))
	if len(sourceGroupIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `
SELECT custom_source_group_id, id
FROM groups
WHERE deleted_at IS NULL
  AND status = $2
  AND is_custom_subscription_group = TRUE
  AND custom_source_group_id = ANY($1::bigint[])
ORDER BY custom_source_group_id, id`, pq.Array(sourceGroupIDs), service.StatusActive)
	if err != nil {
		return nil, fmt.Errorf("load custom groups by source group: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var sourceID int64
		var customID int64
		if err := rows.Scan(&sourceID, &customID); err != nil {
			return nil, fmt.Errorf("scan custom group by source group: %w", err)
		}
		if sourceID > 0 && customID > 0 {
			out[sourceID] = append(out[sourceID], customID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom groups by source group: %w", err)
	}
	return out, nil
}

func uniquePositiveInt64s(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func accountGroupBindingGroupIDs(bindings []accountGroupBindingTarget) []int64 {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		if binding.GroupID > 0 {
			out = append(out, binding.GroupID)
		}
	}
	return out
}
