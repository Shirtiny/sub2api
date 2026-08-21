package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration185ExtendsMembershipBenefitsThroughLevel5(t *testing.T) {
	content, err := FS.ReadFile("185_extend_membership_levels.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.NotContains(t, sql, "INSERT INTO settings")
	require.NotContains(t, sql, "UPDATE settings")
	require.Contains(t, sql, "CHECK (membership_level BETWEEN 0 AND 5)")
}
