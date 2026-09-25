//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestPresalePostgresOfflineRefundIdempotency(t *testing.T) {
	ctx := context.Background()
	s, u, p := newPresaleSafetyPostgresFixture(t)
	o := newPresaleSafetyPostgresOrder(t, u, p, service.NextPresalePeriod(time.Now()))
	c := integrationEntClient
	c.PaymentOrder.UpdateOneID(o.ID).SetStatus(service.OrderStatusPaid).SetPaidAt(time.Now()).ExecX(ctx)
	require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
	o = c.PaymentOrder.GetX(ctx, o.ID)
	req := service.PresaleOfflineRequest{Mode: "refund", Amount: 80, Reason: "completed external refund", Reference: "receipt", Confirmed: true, ExpectedUpdatedAt: o.UpdatedAt}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, errs[i] = s.ProcessPresaleOffline(ctx, o.ID, 99, req) }()
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 20.0, c.User.GetX(ctx, u.ID).TotalRecharged)
	require.Equal(t, service.OrderStatusPartiallyRefunded, c.PaymentOrder.GetX(ctx, o.ID).Status)
	require.Equal(t, 1, c.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(fmt.Sprint(o.ID)), paymentauditlog.ActionEQ("PRESALE_OFFLINE_REFUND")).CountX(ctx))
	_, err := s.GetPresaleQuote(ctx, u.ID, p.ID)
	require.NoError(t, err)
}

func TestPresalePostgresOfflineCancellationRacesActivation(t *testing.T) {
	ctx := context.Background()
	c := integrationEntClient
	// The user lock serializes activation and cancellation; stale admin review
	// cannot silently cancel a term that became active after it was reviewed.
	for range 4 {
		s, u, p := newPresaleSafetyPostgresFixture(t)
		period := service.NextPresalePeriod(time.Now())
		period.StartsAt = time.Now().Add(-time.Minute).Truncate(time.Second)
		period.ExpiresAt = period.StartsAt.Add(31 * 24 * time.Hour)
		o := newPresaleSafetyPostgresOrder(t, u, p, period)
		c.PaymentOrder.UpdateOneID(o.ID).SetStatus(service.OrderStatusPaid).SetPaidAt(time.Now()).ExecX(ctx)
		require.NoError(t, s.ExecuteSubscriptionFulfillment(ctx, o.ID))
		o = c.PaymentOrder.GetX(ctx, o.ID)
		req := service.PresaleOfflineRequest{Mode: "cancel", Reason: "customer request", Confirmed: true, ExpectedUpdatedAt: o.UpdatedAt}
		var wg sync.WaitGroup
		start := make(chan struct{})
		var cancelErr, activationErr error
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _, cancelErr = s.ProcessPresaleOffline(ctx, o.ID, 99, req) }()
		go func() { defer wg.Done(); <-start; _, activationErr = s.ActivateDuePresales(ctx, time.Now()) }()
		close(start)
		wg.Wait()
		require.NoError(t, activationErr)
		final := c.PaymentOrder.GetX(ctx, o.ID)
		if cancelErr == nil {
			require.Equal(t, service.OrderStatusPresaleCancelled, final.Status)
			require.Nil(t, final.PresaleActivatedAt)
		} else {
			require.Equal(t, service.OrderStatusCompleted, final.Status)
			require.NotNil(t, final.PresaleActivatedAt)
			req.ExpectedUpdatedAt = final.UpdatedAt
			_, err := s.ProcessPresaleOffline(ctx, o.ID, 99, req)
			require.NoError(t, err)
			require.Equal(t, service.OrderStatusPresaleCancelled, c.PaymentOrder.GetX(ctx, o.ID).Status)
		}
	}
}
