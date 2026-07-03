//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestValidatePlanRequired_AllValid(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_EmptyName(t *testing.T) {
	err := validatePlanRequired("", 1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_WhitespaceName(t *testing.T) {
	err := validatePlanRequired("   ", 1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_ZeroGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", 0, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_NegativeGroupID(t *testing.T) {
	err := validatePlanRequired("Pro", -1, 9.99, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanRequired_ZeroPrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 0, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_NegativePrice(t *testing.T) {
	err := validatePlanRequired("Pro", 1, -5, 30, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanRequired_ZeroValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 0, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_NegativeValidityDays(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, -7, "days", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanRequired_EmptyValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_WhitespaceValidityUnit(t *testing.T) {
	err := validatePlanRequired("Pro", 1, 9.99, 30, "   ", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanRequired_NameValidatedFirst(t *testing.T) {
	err := validatePlanRequired("", 0, 0, 0, "", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanRequired_TrimmedValidName(t *testing.T) {
	err := validatePlanRequired("  Pro  ", 1, 9.99, 30, "days", nil)
	require.NoError(t, err)
}

func TestValidatePlanRequired_NegativeOriginalPrice(t *testing.T) {
	neg := -10.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &neg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanRequired_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &zero)
	require.NoError(t, err)
}

func TestValidatePlanRequired_ValidOriginalPrice(t *testing.T) {
	op := 19.99
	err := validatePlanRequired("Pro", 1, 9.99, 30, "days", &op)
	require.NoError(t, err)
}

func TestNormalizePlanCustomMultiplierConfigAllowsOneXMinimum(t *testing.T) {
	minValue, maxValue, err := normalizePlanCustomMultiplierConfig(true, 1, 5)
	require.NoError(t, err)
	require.Equal(t, 1, minValue)
	require.Equal(t, 5, maxValue)
}

func TestNormalizePlanCustomMultiplierConfigRejectsBelowOne(t *testing.T) {
	_, _, err := normalizePlanCustomMultiplierConfig(true, 0, 5)
	require.Error(t, err)
	require.Equal(t, "PLAN_CUSTOM_MULTIPLIER_MIN_INVALID", infraerrors.Reason(err))
}

// --- validatePlanPatch tests ---

func TestValidatePlanPatch_NegativeOriginalPrice(t *testing.T) {
	neg := -5.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &neg})
	require.Error(t, err)
	require.Contains(t, err.Error(), "original price")
}

func TestValidatePlanPatch_ZeroOriginalPrice(t *testing.T) {
	zero := 0.0
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &zero})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ValidOriginalPrice(t *testing.T) {
	op := 29.99
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: &op})
	require.NoError(t, err)
}

func TestValidatePlanPatch_NilOriginalPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{OriginalPrice: nil})
	require.NoError(t, err)
}

// --- validatePlanPatch: other fields ---

func ptrStr(s string) *string     { return &s }
func ptrInt(i int) *int           { return &i }
func ptrInt64(i int64) *int64     { return &i }
func ptrFloat(f float64) *float64 { return &f }

func TestValidatePlanPatch_EmptyName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan name")
}

func TestValidatePlanPatch_ValidName(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Name: ptrStr("Basic")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroGroupID(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{GroupID: ptrInt64(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "group")
}

func TestValidatePlanPatch_NegativePrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(-1)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ZeroPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price")
}

func TestValidatePlanPatch_ValidPrice(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{Price: ptrFloat(9.99)})
	require.NoError(t, err)
}

func TestValidatePlanPatch_ZeroValidityDays(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityDays: ptrInt(0)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity days")
}

func TestValidatePlanPatch_EmptyValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validity unit")
}

func TestValidatePlanPatch_ValidValidityUnit(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{ValidityUnit: ptrStr("days")})
	require.NoError(t, err)
}

func TestValidatePlanPatch_AllNil(t *testing.T) {
	err := validatePlanPatch(UpdatePlanRequest{})
	require.NoError(t, err)
}

func TestCreatePlanValidatesGroupExistsActiveAndSubscriptionType(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	svc := &PaymentConfigService{entClient: client}

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{
		GroupID:      404,
		Name:         "Missing Group Plan",
		Price:        10,
		ValidityDays: 30,
		ValidityUnit: "days",
	})
	require.Error(t, err)
	require.Equal(t, "PLAN_GROUP_NOT_FOUND", infraerrors.Reason(err))

	inactiveGroup, err := client.Group.Create().SetName("inactive-plan-group").SetStatus(StatusDisabled).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	_, err = svc.CreatePlan(ctx, CreatePlanRequest{GroupID: inactiveGroup.ID, Name: "Inactive Group Plan", Price: 10, ValidityDays: 30, ValidityUnit: "days"})
	require.Error(t, err)
	require.Equal(t, "PLAN_GROUP_INACTIVE", infraerrors.Reason(err))

	standardGroup, err := client.Group.Create().SetName("standard-plan-group").SetStatus(payment.EntityStatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeStandard).Save(ctx)
	require.NoError(t, err)
	_, err = svc.CreatePlan(ctx, CreatePlanRequest{GroupID: standardGroup.ID, Name: "Standard Group Plan", Price: 10, ValidityDays: 30, ValidityUnit: "days"})
	require.Error(t, err)
	require.Equal(t, "PLAN_GROUP_TYPE_MISMATCH", infraerrors.Reason(err))

	subscriptionGroup, err := client.Group.Create().SetName("subscription-plan-group").SetStatus(payment.EntityStatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	plan, err := svc.CreatePlan(ctx, CreatePlanRequest{GroupID: subscriptionGroup.ID, Name: "Valid Group Plan", Price: 10, ValidityDays: 30, ValidityUnit: "days"})
	require.NoError(t, err)
	require.Equal(t, subscriptionGroup.ID, plan.GroupID)
}

func TestUpdatePlanValidatesNewGroupBeforeSaving(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	svc := &PaymentConfigService{entClient: client}

	subscriptionGroup, err := client.Group.Create().SetName("update-source-subscription").SetStatus(payment.EntityStatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeSubscription).Save(ctx)
	require.NoError(t, err)
	standardGroup, err := client.Group.Create().SetName("update-target-standard").SetStatus(payment.EntityStatusActive).SetPlatform(PlatformOpenAI).SetSubscriptionType(SubscriptionTypeStandard).Save(ctx)
	require.NoError(t, err)
	plan, err := client.SubscriptionPlan.Create().SetGroupID(subscriptionGroup.ID).SetName("Update Plan").SetDescription("").SetPrice(10).SetValidityDays(30).SetValidityUnit("days").SetForSale(true).Save(ctx)
	require.NoError(t, err)

	_, err = svc.UpdatePlan(ctx, plan.ID, UpdatePlanRequest{GroupID: &standardGroup.ID})
	require.Error(t, err)
	require.Equal(t, "PLAN_GROUP_TYPE_MISMATCH", infraerrors.Reason(err))

	updated, err := client.SubscriptionPlan.Get(ctx, plan.ID)
	require.NoError(t, err)
	require.Equal(t, subscriptionGroup.ID, updated.GroupID, "failed update must not rebind plan to a non-subscription group")
}

func TestValidateSubscriptionPlanGroupRejectsCustomSubscriptionGroup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)

	customGroup, err := client.Group.Create().
		SetName("plan-custom-target").
		SetStatus(payment.EntityStatusActive).
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetIsCustomSubscriptionGroup(true).
		SetCustomOwnerUserID(1).
		SetCustomSourcePlanID(1).
		SetCustomSourceGroupID(1).
		SetCustomMultiplier(2).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentConfigService{entClient: client}
	err = svc.validateSubscriptionPlanGroup(ctx, customGroup.ID)
	require.Error(t, err)
	require.Equal(t, "PLAN_GROUP_CUSTOM_NOT_ALLOWED", infraerrors.Reason(err))
}
