//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestCheckRecentDuplicateOrderBlocksImmediateBalanceDuplicate(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("dup-order@example.com").SetPasswordHash("hash").SetUsername("dup-order").Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(80).
		SetPayAmount(20).
		SetFeeRate(0).
		SetRechargeCode("DUP-RECENT").
		SetOutTradeNo("sub2_dup_recent").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	svc := &PaymentService{}
	err = svc.checkRecentDuplicateOrder(ctx, tx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeBalance,
	}, 80, nil)
	require.Error(t, err)
	require.Equal(t, "DUPLICATE_PAYMENT_ORDER_RECENT", infraerrors.Reason(err))
}

func TestCheckRecentDuplicateOrderAllowsOldPendingOrder(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("old-order@example.com").SetPasswordHash("hash").SetUsername("old-order").Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(80).
		SetPayAmount(20).
		SetFeeRate(0).
		SetRechargeCode("DUP-OLD").
		SetOutTradeNo("sub2_dup_old").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		SetCreatedAt(time.Now().Add(-recentDuplicateOrderWindow - time.Second)).
		Save(ctx)
	require.NoError(t, err)

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	svc := &PaymentService{}
	err = svc.checkRecentDuplicateOrder(ctx, tx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeBalance,
	}, 80, nil)
	require.NoError(t, err)
}

func TestCheckRecentDuplicateOrderBlocksJustCompletedDuplicate(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("completed-dup@example.com").SetPasswordHash("hash").SetUsername("completed-dup").Save(ctx)
	require.NoError(t, err)

	_, err = client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(80).
		SetPayAmount(20).
		SetFeeRate(0).
		SetRechargeCode("DUP-COMPLETED").
		SetOutTradeNo("sub2_dup_completed").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-completed").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("app.example.com").
		Save(ctx)
	require.NoError(t, err)

	tx, err := client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	svc := &PaymentService{}
	err = svc.checkRecentDuplicateOrder(ctx, tx, CreateOrderRequest{
		UserID:      user.ID,
		PaymentType: payment.TypeAlipay,
		OrderType:   payment.OrderTypeBalance,
	}, 80, nil)
	require.Error(t, err)
	require.Equal(t, "DUPLICATE_PAYMENT_ORDER_RECENT", infraerrors.Reason(err))
}
