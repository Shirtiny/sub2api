package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func TestDurableUsageRangeWaitsForDailySettlement(t *testing.T) {
	loc := timezone.Location()
	today := time.Date(2026, 8, 27, 0, 0, 0, 0, loc)
	start := today.AddDate(0, 0, -2)
	end := today.AddDate(0, 0, 1)

	before := newDurableUsageRangeAt(start, end, today.Add(2*time.Hour+59*time.Minute))
	require.Equal(t, today.AddDate(0, 0, -1).Format("2006-01-02"), before.endDate)

	after := newDurableUsageRangeAt(start, end, today.Add(3*time.Hour))
	require.Equal(t, today.Format("2006-01-02"), after.endDate)
}
