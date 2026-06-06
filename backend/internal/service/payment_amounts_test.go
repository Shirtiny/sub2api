package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaidRechargePointsFactor(t *testing.T) {
	t.Parallel()

	require.InDelta(t, 0.25, paidRechargePointsFactor(40, 10), 1e-12)
	require.Zero(t, paidRechargePointsFactor(0, 10))
	require.Zero(t, paidRechargePointsFactor(40, 0))
}
