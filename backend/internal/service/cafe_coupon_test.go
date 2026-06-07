//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestCafeCouponClaimEligibilityAndIdempotency(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("cafe@example.com").SetPasswordHash("hash").SetUsername("cafe").Save(ctx)
	require.NoError(t, err)
	repo := cafeCouponSettingsRepo(map[string]string{
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":12.5,"period":"month"}}}`,
	})
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	svc := &PaymentService{entClient: client, userRepo: userRepo, configService: &PaymentConfigService{settingRepo: repo}}

	first, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.False(t, first.AlreadyClaimed)
	require.Equal(t, CafeCouponTypeCash, first.CouponType)
	require.Equal(t, 12.5, first.Value)

	second, err := svc.ClaimCafeCoupon(ctx, user.ID)
	require.NoError(t, err)
	require.True(t, second.AlreadyClaimed)
	require.Equal(t, first.Code, second.Code)

	count, err := client.CafeCoupon.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCafeCouponClaimRejectsIneligibleUser(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	repo := cafeCouponSettingsRepo(map[string]string{
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":12.5,"period":"month"}}}`,
	})
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: 1, Status: payment.EntityStatusActive, TotalRecharged: 0}},
		configService: &PaymentConfigService{settingRepo: repo},
	}

	_, err := svc.ClaimCafeCoupon(ctx, 1)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_NOT_ELIGIBLE", infraerrors.Reason(err))
}

func TestCafeCouponConcurrentClaimCreatesOneCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("cafe@example.com").SetPasswordHash("hash").SetUsername("cafe").Save(ctx)
	require.NoError(t, err)
	repo := cafeCouponSettingsRepo(map[string]string{
		SettingKeyCafeCouponConfig: `{"levels":{"1":{"enabled":true,"type":"cash","value":8,"period":"month"}}}`,
	})
	svc := &PaymentService{
		entClient:     client,
		userRepo:      &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}},
		configService: &PaymentConfigService{settingRepo: repo},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	codes := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := svc.ClaimCafeCoupon(ctx, user.ID)
			if err != nil {
				errCh <- err
				return
			}
			codes <- claimed.Code
		}()
	}
	wg.Wait()
	close(errCh)
	close(codes)
	for err := range errCh {
		require.NoError(t, err)
	}
	seen := map[string]struct{}{}
	for code := range codes {
		seen[code] = struct{}{}
	}
	require.Len(t, seen, 1)
	count, err := client.CafeCoupon.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestCafeCouponPreviewRejectsInvalidInputsAndOwnership(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	owner, err := client.User.Create().SetEmail("owner@example.com").SetPasswordHash("hash").SetUsername("owner").Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client, userRepo: &mockUserRepo{getByIDUser: &User{ID: 1, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}}

	_, err = svc.PreviewCafeCoupon(ctx, 1, "bad code!", 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_INVALID", infraerrors.Reason(err))

	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-OWNER").SetUserID(owner.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(5).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(time.Now().Add(-time.Hour)).SetPeriodEnd(time.Now().Add(time.Hour)).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	_, err = svc.PreviewCafeCoupon(ctx, owner.ID+1, coupon.Code, 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_FORBIDDEN", infraerrors.Reason(err))
}

func TestCafeCouponCreateOrderAppliesServerSideDiscountAndMarksCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("buyer@example.com").SetPasswordHash("hash").SetUsername("buyer").Save(ctx)
	require.NoError(t, err)

	start, end := cafeCouponPeriodWindow(time.Now(), CafeCouponPeriodMonth)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-ORDER").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(start).SetPeriodEnd(end).SetStatus(CafeCouponStatusIssued).
		Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}
	userRepo.updateBalanceFn = func(ctx context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{}}
	svc := &PaymentService{
		entClient:     client,
		configService: &PaymentConfigService{entClient: client, settingRepo: cafeCouponSettingsRepo(map[string]string{SettingPaymentEnabled: "true"})},
		userRepo:      userRepo,
		redeemService: NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil),
	}
	t.Setenv(paymentDevAutoSuccessEnv, paymentDevAutoSuccessToken)
	t.Setenv(paymentDevEnvironmentEnv, "development")

	resp, err := svc.CreateOrder(ctx, CreateOrderRequest{UserID: user.ID, Amount: 100, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, CafeCouponCode: coupon.Code, ClientIP: "127.0.0.1", SrcHost: "app.example.com"})
	require.NoError(t, err)
	require.Equal(t, 90.0, resp.PayAmount)

	order, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, 90.0, order.PayAmount)
	require.Equal(t, 10.0, order.CafeCouponDiscount)
	require.Equal(t, coupon.Code, psStringValue(order.CafeCouponCode))

	updatedCoupon, err := client.CafeCoupon.Get(ctx, coupon.ID)
	require.NoError(t, err)
	require.Equal(t, CafeCouponStatusApplied, updatedCoupon.Status)
	require.NotNil(t, updatedCoupon.OrderID)
	require.Equal(t, resp.OrderID, *updatedCoupon.OrderID)

	auditCount, err := client.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("CAFE_COUPON_APPLIED")).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, auditCount)
}

func TestCafeCouponCreateOrderRejectsUsedCoupon(t *testing.T) {
	ctx := context.Background()
	client := newCafeCouponTestClient(t)
	user, err := client.User.Create().SetEmail("used@example.com").SetPasswordHash("hash").SetUsername("used").Save(ctx)
	require.NoError(t, err)
	coupon, err := client.CafeCoupon.Create().
		SetCode("CAFE-USED").SetUserID(user.ID).SetMembershipLevel(1).SetCouponType(CafeCouponTypeCash).SetValue(10).
		SetPeriod(CafeCouponPeriodMonth).SetPeriodStart(time.Now().Add(-time.Hour)).SetPeriodEnd(time.Now().Add(time.Hour)).SetStatus(CafeCouponStatusApplied).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client, userRepo: &mockUserRepo{getByIDUser: &User{ID: user.ID, Status: payment.EntityStatusActive, TotalRecharged: MembershipLevel1Threshold + 1}}}
	_, err = svc.PreviewCafeCoupon(ctx, user.ID, coupon.Code, 100)
	require.Error(t, err)
	require.Equal(t, "CAFE_COUPON_USED", infraerrors.Reason(err))
}

func newCafeCouponTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name())
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

type cafeCouponSettingsRepo map[string]string

func (r cafeCouponSettingsRepo) Get(context.Context, string) (*Setting, error) { return nil, nil }
func (r cafeCouponSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (r cafeCouponSettingsRepo) Set(_ context.Context, key, value string) error {
	r[key] = value
	return nil
}
func (r cafeCouponSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r[key]
	}
	return out, nil
}
func (r cafeCouponSettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r[key] = value
	}
	return nil
}
func (r cafeCouponSettingsRepo) GetAll(context.Context) (map[string]string, error) { return r, nil }
func (r cafeCouponSettingsRepo) Delete(context.Context, string) error              { return nil }
