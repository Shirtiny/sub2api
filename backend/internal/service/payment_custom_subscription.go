package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	entgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func hasCustomSubscriptionPaymentOrderFields(o *dbent.PaymentOrder) bool {
	return o != nil && o.PlanID != nil && o.SubscriptionSourceGroupID != nil && o.SubscriptionMultiplier != nil && *o.SubscriptionMultiplier >= minCustomSubscriptionMultiplier
}

func (s *PaymentService) isCustomSubscriptionPaymentOrder(ctx context.Context, o *dbent.PaymentOrder) (bool, error) {
	if !hasCustomSubscriptionPaymentOrderFields(o) {
		return false, nil
	}
	if *o.SubscriptionMultiplier > 1 {
		return true, nil
	}
	if s == nil || s.entClient == nil {
		return false, fmt.Errorf("payment service not ready")
	}
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, *o.PlanID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get subscription plan for custom order detection: %w", err)
	}
	return plan.CustomMultiplierEnabled, nil
}

func (s *PaymentService) isCustomSubscriptionPaymentOrderForRefund(ctx context.Context, o *dbent.PaymentOrder, force bool) (bool, *RefundResult) {
	customOrder, err := s.isCustomSubscriptionPaymentOrder(ctx, o)
	if err == nil {
		return customOrder, nil
	}
	if !force {
		return false, &RefundResult{Success: false, Warning: "cannot verify custom subscription order, use force", RequireForce: true}
	}
	orderID := int64(0)
	if o != nil {
		orderID = o.ID
	}
	slog.Warn("refund: custom subscription order verification failed; continuing because force=true", "orderID", orderID, "error", err)
	return false, nil
}

type virtualCustomSubscriptionEntitlement struct {
	GroupID             int64
	Multiplier          int
	SourcePlanID        int64
	SourceGroupID       int64
	DisplayName         string
	MigrateFromGroupIDs []int64
}

func (s *PaymentService) virtualCustomSubscriptionEntitlementForOrder(ctx context.Context, o *dbent.PaymentOrder) (*virtualCustomSubscriptionEntitlement, error) {
	if !hasCustomSubscriptionPaymentOrderFields(o) {
		return nil, nil
	}
	if s == nil || s.entClient == nil {
		return nil, fmt.Errorf("payment service not ready")
	}
	planID := *o.PlanID
	sourceGroupID := *o.SubscriptionSourceGroupID
	multiplier := *o.SubscriptionMultiplier
	if multiplier < minCustomSubscriptionMultiplier || multiplier > maxCustomSubscriptionMultiplier {
		return nil, fmt.Errorf("invalid custom subscription multiplier %d", multiplier)
	}
	plan, err := s.entClient.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get source subscription plan: %w", err)
	}
	if err := validateCustomSubscriptionPlanSource(plan, sourceGroupID); err != nil {
		return nil, err
	}
	source, err := s.entClient.Group.Get(ctx, sourceGroupID)
	if err != nil {
		return nil, fmt.Errorf("get source subscription group: %w", err)
	}
	if err := validateCustomSubscriptionSourceGroup(source, sourceGroupID); err != nil {
		return nil, err
	}
	migrateFromGroupIDs, err := s.expiredReusableCustomSubscriptionGroupIDs(ctx, o.UserID, planID)
	if err != nil {
		return nil, err
	}
	return &virtualCustomSubscriptionEntitlement{
		GroupID:             sourceGroupID,
		Multiplier:          multiplier,
		SourcePlanID:        planID,
		SourceGroupID:       sourceGroupID,
		DisplayName:         customSubscriptionGroupName(source.Name, multiplier),
		MigrateFromGroupIDs: migrateFromGroupIDs,
	}, nil
}

func (s *PaymentService) shouldUseLegacyCustomSubscriptionGroup(ctx context.Context, o *dbent.PaymentOrder) (bool, error) {
	if !hasCustomSubscriptionPaymentOrderFields(o) {
		return false, nil
	}
	active, err := s.findActiveCustomSubscriptionGroup(ctx, o.UserID, *o.PlanID)
	if err == nil && active != nil {
		return true, nil
	}
	if err != nil && !dbent.IsNotFound(err) {
		return false, fmt.Errorf("find active custom subscription group: %w", err)
	}
	return false, nil
}

func (s *PaymentService) expiredReusableCustomSubscriptionGroupIDs(ctx context.Context, userID, planID int64) ([]int64, error) {
	if s == nil || s.entClient == nil || userID <= 0 || planID <= 0 {
		return nil, nil
	}
	groups, err := s.entClient.Group.Query().
		Where(
			entgroup.IsCustomSubscriptionGroupEQ(true),
			entgroup.CustomOwnerUserIDEQ(userID),
			entgroup.CustomSourcePlanIDEQ(planID),
		).
		Order(dbent.Asc(entgroup.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reusable custom subscription groups: %w", err)
	}
	ids := make([]int64, 0, len(groups))
	now := time.Now()
	for _, g := range groups {
		active, err := s.entClient.UserSubscription.Query().
			Where(
				usersubscription.GroupIDEQ(g.ID),
				usersubscription.UserIDEQ(userID),
				usersubscription.StatusEQ(SubscriptionStatusActive),
				activeUserSubscriptionExpiresAt(now),
				usersubscription.DeletedAtIsNil(),
			).
			Exist(ctx)
		if err != nil {
			return nil, fmt.Errorf("check reusable custom subscription group activity: %w", err)
		}
		if !active {
			ids = append(ids, g.ID)
		}
	}
	return ids, nil
}

func (s *PaymentService) ensureCustomSubscriptionGroupForOrder(ctx context.Context, o *dbent.PaymentOrder) (int64, error) {
	if !hasCustomSubscriptionPaymentOrderFields(o) {
		if o == nil || o.SubscriptionGroupID == nil {
			return 0, fmt.Errorf("missing subscription group")
		}
		return *o.SubscriptionGroupID, nil
	}
	if s == nil || s.entClient == nil {
		return 0, fmt.Errorf("payment service not ready")
	}

	planID := *o.PlanID
	sourceGroupID := *o.SubscriptionSourceGroupID
	multiplier := *o.SubscriptionMultiplier
	if multiplier < minCustomSubscriptionMultiplier || multiplier > maxCustomSubscriptionMultiplier {
		return 0, fmt.Errorf("invalid custom subscription multiplier %d", multiplier)
	}

	plan, err := s.entClient.SubscriptionPlan.Get(ctx, planID)
	if err != nil {
		return 0, fmt.Errorf("get source subscription plan: %w", err)
	}
	if err := validateCustomSubscriptionPlanSource(plan, sourceGroupID); err != nil {
		return 0, err
	}

	active, err := s.findActiveCustomSubscriptionGroup(ctx, o.UserID, planID)
	if err == nil && active != nil {
		if active.Status != payment.EntityStatusActive {
			return 0, fmt.Errorf("custom subscription group %d is inactive", active.ID)
		}
		if err := s.syncCustomGroupFromSource(ctx, active.ID, sourceGroupID, multiplier); err != nil {
			return 0, err
		}
		return active.ID, nil
	}
	if err != nil && !dbent.IsNotFound(err) {
		return 0, fmt.Errorf("find active custom subscription group: %w", err)
	}

	source, err := s.entClient.Group.Get(ctx, sourceGroupID)
	if err != nil {
		return 0, fmt.Errorf("get source subscription group: %w", err)
	}
	if err := validateCustomSubscriptionSourceGroup(source, sourceGroupID); err != nil {
		return 0, err
	}

	reusable, err := s.findReusableCustomSubscriptionGroup(ctx, o.UserID, planID)
	if err == nil && reusable != nil {
		if err := s.syncCustomGroupFromSource(ctx, reusable.ID, sourceGroupID, multiplier); err != nil {
			return 0, err
		}
		return reusable.ID, nil
	}
	if err != nil && !dbent.IsNotFound(err) {
		return 0, fmt.Errorf("find reusable custom subscription group: %w", err)
	}

	name, err := s.uniqueCustomSubscriptionGroupName(ctx, customSubscriptionGroupName(source.Name, multiplier))
	if err != nil {
		return 0, err
	}
	created, err := s.createCustomSubscriptionGroup(ctx, source, name, o.UserID, planID, sourceGroupID, multiplier)
	if err != nil {
		return 0, err
	}
	if err := s.copyGroupAccountBindings(ctx, sourceGroupID, created.ID); err != nil {
		return 0, err
	}
	if err := s.syncCustomGroupChannelBinding(ctx, sourceGroupID, created.ID); err != nil {
		return 0, err
	}
	if err := s.syncCustomGroupAccountStatsPricingRules(ctx, sourceGroupID, created.ID); err != nil {
		return 0, err
	}
	if err := s.notifyCustomSubscriptionGroupChanged(ctx, created.ID); err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (s *PaymentService) findReusableCustomSubscriptionGroup(ctx context.Context, userID, planID int64) (*dbent.Group, error) {
	if s == nil || s.entClient == nil || userID <= 0 || planID <= 0 {
		return nil, &dbent.NotFoundError{}
	}
	base := func() *dbent.GroupQuery {
		return s.entClient.Group.Query().
			Where(
				entgroup.IsCustomSubscriptionGroupEQ(true),
				entgroup.CustomOwnerUserIDEQ(userID),
				entgroup.CustomSourcePlanIDEQ(planID),
			)
	}
	active, err := base().
		Where(entgroup.StatusEQ(payment.EntityStatusActive)).
		Order(dbent.Desc(entgroup.FieldID)).
		First(ctx)
	if err == nil {
		return active, nil
	}
	if !dbent.IsNotFound(err) {
		return nil, err
	}
	return base().
		Order(dbent.Desc(entgroup.FieldID)).
		First(ctx)
}

func (s *PaymentService) ensureReusableCustomGroupSafety(ctx context.Context, customGroupID, sourceGroupID int64, orderMultiplier int) error {
	if s == nil || s.entClient == nil || customGroupID <= 0 {
		return fmt.Errorf("custom subscription group not found")
	}
	if orderMultiplier < minCustomSubscriptionMultiplier || orderMultiplier > maxCustomSubscriptionMultiplier {
		return fmt.Errorf("invalid custom subscription multiplier %d", orderMultiplier)
	}
	customGroup, err := s.entClient.Group.Get(ctx, customGroupID)
	if err != nil {
		return fmt.Errorf("get custom subscription group: %w", err)
	}
	if !customGroup.IsCustomSubscriptionGroup {
		return fmt.Errorf("group %d is not a custom subscription group", customGroupID)
	}
	if customGroup.Status != payment.EntityStatusActive {
		return fmt.Errorf("custom subscription group %d is inactive", customGroupID)
	}
	activeCount, err := s.entClient.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(customGroupID),
			usersubscription.StatusEQ(SubscriptionStatusActive),
			activeUserSubscriptionExpiresAt(time.Now()),
			usersubscription.DeletedAtIsNil(),
		).
		Count(ctx)
	if err != nil {
		return fmt.Errorf("check active custom subscription: %w", err)
	}
	if activeCount > 0 {
		if customGroup.CustomMultiplier == nil || *customGroup.CustomMultiplier != orderMultiplier {
			return fmt.Errorf("active custom subscription multiplier mismatch")
		}
		if customGroup.CustomSourceGroupID != nil && *customGroup.CustomSourceGroupID != sourceGroupID {
			if customGroup.CustomSourcePlanID == nil {
				return fmt.Errorf("active custom subscription source group mismatch")
			}
			plan, err := s.entClient.SubscriptionPlan.Get(ctx, *customGroup.CustomSourcePlanID)
			if err != nil {
				return fmt.Errorf("get source subscription plan: %w", err)
			}
			if plan.GroupID != sourceGroupID {
				return fmt.Errorf("active custom subscription source group mismatch")
			}
		}
	}
	return nil
}

func validateCustomSubscriptionPlanSource(plan *dbent.SubscriptionPlan, sourceGroupID int64) error {
	if plan == nil {
		return fmt.Errorf("source subscription plan not found")
	}
	if plan.GroupID != sourceGroupID {
		return fmt.Errorf("subscription plan %d source group mismatch", plan.ID)
	}
	return nil
}

func validateCustomSubscriptionSourceGroup(source *dbent.Group, sourceGroupID int64) error {
	if source == nil {
		return fmt.Errorf("source group %d no longer exists", sourceGroupID)
	}
	if source.Status != payment.EntityStatusActive {
		return fmt.Errorf("source group %d no longer active", sourceGroupID)
	}
	if source.SubscriptionType != SubscriptionTypeSubscription {
		return fmt.Errorf("source group %d is not subscription type", sourceGroupID)
	}
	return nil
}

func customSubscriptionGroupName(groupName string, multiplier int) string {
	const maxGroupNameRunes = 100
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		groupName = "Subscription"
	}
	if multiplier < 1 {
		multiplier = 1
	}
	suffix := fmt.Sprintf("-%dx", multiplier)
	if strings.HasSuffix(groupName, suffix) {
		return truncateCustomSubscriptionGroupName(groupName)
	}
	allowedNameRunes := maxGroupNameRunes - len([]rune(suffix))
	if allowedNameRunes < 0 {
		allowedNameRunes = 0
	}
	nameRunes := []rune(groupName)
	if len(nameRunes) > allowedNameRunes {
		nameRunes = nameRunes[:allowedNameRunes]
	}
	return string(nameRunes) + suffix
}

func (s *PaymentService) uniqueCustomSubscriptionGroupName(ctx context.Context, base string) (string, error) {
	return s.uniqueCustomSubscriptionGroupNameExcept(ctx, base, 0)
}

func (s *PaymentService) uniqueCustomSubscriptionGroupNameExcept(ctx context.Context, base string, exceptGroupID int64) (string, error) {
	base = truncateCustomSubscriptionGroupName(strings.TrimSpace(base))
	if base == "" {
		base = fmt.Sprintf("Custom-%d", time.Now().UnixNano())
	}
	candidate := base
	for i := 0; i < 20; i++ {
		query := s.entClient.Group.Query().Where(entgroup.NameEQ(candidate))
		if exceptGroupID > 0 {
			query = query.Where(entgroup.IDNEQ(exceptGroupID))
		}
		exists, err := query.Exist(ctx)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		suffix := fmt.Sprintf("-%d", i+2)
		candidate = truncateCustomSubscriptionGroupName(base, suffix)
	}
	suffix := fmt.Sprintf("-%d", time.Now().Unix())
	return truncateCustomSubscriptionGroupName(base, suffix), nil
}

func truncateCustomSubscriptionGroupName(base string, suffix ...string) string {
	const maxGroupNameRunes = 100
	nameSuffix := ""
	if len(suffix) > 0 {
		nameSuffix = suffix[0]
	}
	allowed := maxGroupNameRunes - len([]rune(nameSuffix))
	if allowed < 0 {
		allowed = 0
	}
	runes := []rune(base)
	if len(runes) <= allowed {
		return base + nameSuffix
	}
	if hash := strings.LastIndex(base, "#"); hash >= 0 {
		head := []rune(base[:hash])
		tail := []rune(base[hash:])
		headAllowed := allowed - len(tail)
		if headAllowed >= 0 {
			if len(head) > headAllowed {
				head = head[:headAllowed]
			}
			return string(head) + string(tail) + nameSuffix
		}
	}
	return string(runes[:allowed]) + nameSuffix
}

func (s *PaymentService) createCustomSubscriptionGroup(ctx context.Context, source *dbent.Group, name string, ownerUserID, sourcePlanID, sourceGroupID int64, multiplier int) (*dbent.Group, error) {
	if multiplier < minCustomSubscriptionMultiplier || multiplier > maxCustomSubscriptionMultiplier {
		return nil, fmt.Errorf("invalid custom subscription multiplier %d", multiplier)
	}
	m := float64(multiplier)
	builder := s.entClient.Group.Create().
		SetName(name).
		SetNillableDescription(source.Description).
		SetPlatform(source.Platform).
		SetRateMultiplier(source.RateMultiplier).
		SetSortOrder(source.SortOrder).
		SetIsExclusive(source.IsExclusive).
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetNillableDailyLimitUsd(multiplyOptionalLimit(source.DailyLimitUsd, m)).
		SetNillableWeeklyLimitUsd(multiplyOptionalLimit(source.WeeklyLimitUsd, m)).
		SetNillableMonthlyLimitUsd(multiplyOptionalLimit(source.MonthlyLimitUsd, m)).
		SetAllowImageGeneration(source.AllowImageGeneration).
		SetImageRateIndependent(source.ImageRateIndependent).
		SetImageRateMultiplier(source.ImageRateMultiplier).
		SetNillableImagePrice1k(source.ImagePrice1k).
		SetNillableImagePrice2k(source.ImagePrice2k).
		SetNillableImagePrice4k(source.ImagePrice4k).
		SetVideoRateIndependent(source.VideoRateIndependent).
		SetVideoRateMultiplier(source.VideoRateMultiplier).
		SetNillableVideoPrice480p(source.VideoPrice480p).
		SetNillableVideoPrice720p(source.VideoPrice720p).
		SetNillableVideoPrice1080p(source.VideoPrice1080p).
		SetDefaultValidityDays(source.DefaultValidityDays).
		SetClaudeCodeOnly(source.ClaudeCodeOnly).
		SetNillableFallbackGroupID(source.FallbackGroupID).
		SetNillableFallbackGroupIDOnInvalidRequest(source.FallbackGroupIDOnInvalidRequest).
		SetModelRoutingEnabled(source.ModelRoutingEnabled).
		SetMcpXMLInject(source.McpXMLInject).
		SetSupportedModelScopes(source.SupportedModelScopes).
		SetAllowMessagesDispatch(source.AllowMessagesDispatch).
		SetRequireOauthOnly(source.RequireOauthOnly).
		SetRequirePrivacySet(source.RequirePrivacySet).
		SetDefaultMappedModel(source.DefaultMappedModel).
		SetMessagesDispatchModelConfig(source.MessagesDispatchModelConfig).
		SetModelsListConfig(source.ModelsListConfig).
		SetRpmLimit(source.RpmLimit).
		SetIsCustomSubscriptionGroup(true).
		SetCustomOwnerUserID(ownerUserID).
		SetCustomSourcePlanID(sourcePlanID).
		SetCustomSourceGroupID(sourceGroupID).
		SetCustomMultiplier(multiplier)
	if source.ModelRouting != nil {
		builder = builder.SetModelRouting(source.ModelRouting)
	}
	created, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create custom subscription group: %w", err)
	}
	return created, nil
}

func (s *PaymentService) syncCustomGroupFromSource(ctx context.Context, customGroupID, sourceGroupID int64, multiplier int) error {
	if err := s.ensureReusableCustomGroupSafety(ctx, customGroupID, sourceGroupID, multiplier); err != nil {
		return err
	}
	if err := s.syncCustomGroupLimits(ctx, customGroupID, sourceGroupID, multiplier); err != nil {
		return err
	}
	if err := s.syncGroupAccountBindings(ctx, sourceGroupID, customGroupID); err != nil {
		return err
	}
	if err := s.syncCustomGroupChannelBinding(ctx, sourceGroupID, customGroupID); err != nil {
		return err
	}
	if err := s.syncCustomGroupAccountStatsPricingRules(ctx, sourceGroupID, customGroupID); err != nil {
		return err
	}
	return s.notifyCustomSubscriptionGroupChanged(ctx, customGroupID)
}

func (s *PaymentService) syncCustomGroupsForSourceGroupUpdate(ctx context.Context, source *Group) ([]int64, error) {
	if s == nil || s.entClient == nil || source == nil || source.ID <= 0 || source.IsCustomSubscriptionGroup {
		return nil, nil
	}
	customGroups, err := s.entClient.Group.Query().
		Where(
			entgroup.IsCustomSubscriptionGroupEQ(true),
			entgroup.CustomSourceGroupIDEQ(source.ID),
		).
		Order(dbent.Asc(entgroup.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list custom subscription groups for source update: %w", err)
	}
	if len(customGroups) == 0 {
		return nil, nil
	}
	customGroupIDs := make([]int64, 0, len(customGroups))
	for _, customGroup := range customGroups {
		customGroupIDs = append(customGroupIDs, customGroup.ID)
		if source.Status != payment.EntityStatusActive {
			if customGroup.Status != source.Status {
				if _, err := s.entClient.Group.UpdateOneID(customGroup.ID).SetStatus(source.Status).Save(ctx); err != nil {
					return customGroupIDs, fmt.Errorf("disable custom subscription group %d after source update: %w", customGroup.ID, err)
				}
			}
			if err := s.notifyCustomSubscriptionGroupChanged(ctx, customGroup.ID); err != nil {
				return customGroupIDs, err
			}
			continue
		}
		if customGroup.Status != payment.EntityStatusActive {
			continue
		}
		if customGroup.CustomMultiplier == nil || *customGroup.CustomMultiplier < minCustomSubscriptionMultiplier || *customGroup.CustomMultiplier > maxCustomSubscriptionMultiplier {
			return customGroupIDs, fmt.Errorf("custom subscription group %d has invalid multiplier", customGroup.ID)
		}
		if err := s.syncCustomGroupFromSource(ctx, customGroup.ID, source.ID, *customGroup.CustomMultiplier); err != nil {
			return customGroupIDs, fmt.Errorf("sync custom subscription group %d after source update: %w", customGroup.ID, err)
		}
	}
	return customGroupIDs, nil
}

func (s *PaymentService) syncCustomGroupLimits(ctx context.Context, customGroupID, sourceGroupID int64, multiplier int) error {
	if multiplier < minCustomSubscriptionMultiplier || multiplier > maxCustomSubscriptionMultiplier {
		return fmt.Errorf("invalid custom subscription multiplier %d", multiplier)
	}
	source, err := s.entClient.Group.Get(ctx, sourceGroupID)
	if err != nil {
		return fmt.Errorf("get source subscription group: %w", err)
	}
	if err := validateCustomSubscriptionSourceGroup(source, sourceGroupID); err != nil {
		return err
	}
	m := float64(multiplier)
	customGroup, err := s.entClient.Group.Get(ctx, customGroupID)
	if err != nil {
		return fmt.Errorf("get custom subscription group: %w", err)
	}
	if customGroup.CustomOwnerUserID == nil || customGroup.CustomSourcePlanID == nil {
		return fmt.Errorf("custom subscription group %d missing ownership metadata", customGroupID)
	}
	name, err := s.uniqueCustomSubscriptionGroupNameExcept(ctx, customSubscriptionGroupName(source.Name, multiplier), customGroupID)
	if err != nil {
		return fmt.Errorf("generate custom subscription group name: %w", err)
	}
	update := s.entClient.Group.UpdateOneID(customGroupID).
		SetNillableDescription(source.Description).
		SetPlatform(source.Platform).
		SetRateMultiplier(source.RateMultiplier).
		SetSortOrder(source.SortOrder).
		SetIsExclusive(source.IsExclusive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetNillableDailyLimitUsd(multiplyOptionalLimit(source.DailyLimitUsd, m)).
		SetNillableWeeklyLimitUsd(multiplyOptionalLimit(source.WeeklyLimitUsd, m)).
		SetNillableMonthlyLimitUsd(multiplyOptionalLimit(source.MonthlyLimitUsd, m)).
		SetAllowImageGeneration(source.AllowImageGeneration).
		SetImageRateIndependent(source.ImageRateIndependent).
		SetImageRateMultiplier(source.ImageRateMultiplier).
		SetNillableImagePrice1k(source.ImagePrice1k).
		SetNillableImagePrice2k(source.ImagePrice2k).
		SetNillableImagePrice4k(source.ImagePrice4k).
		SetVideoRateIndependent(source.VideoRateIndependent).
		SetVideoRateMultiplier(source.VideoRateMultiplier).
		SetNillableVideoPrice480p(source.VideoPrice480p).
		SetNillableVideoPrice720p(source.VideoPrice720p).
		SetNillableVideoPrice1080p(source.VideoPrice1080p).
		SetDefaultValidityDays(source.DefaultValidityDays).
		SetClaudeCodeOnly(source.ClaudeCodeOnly).
		SetNillableFallbackGroupID(source.FallbackGroupID).
		SetNillableFallbackGroupIDOnInvalidRequest(source.FallbackGroupIDOnInvalidRequest).
		SetModelRoutingEnabled(source.ModelRoutingEnabled).
		SetMcpXMLInject(source.McpXMLInject).
		SetSupportedModelScopes(source.SupportedModelScopes).
		SetAllowMessagesDispatch(source.AllowMessagesDispatch).
		SetRequireOauthOnly(source.RequireOauthOnly).
		SetRequirePrivacySet(source.RequirePrivacySet).
		SetDefaultMappedModel(source.DefaultMappedModel).
		SetMessagesDispatchModelConfig(source.MessagesDispatchModelConfig).
		SetModelsListConfig(source.ModelsListConfig).
		SetRpmLimit(source.RpmLimit).
		SetIsCustomSubscriptionGroup(true).
		SetCustomSourceGroupID(sourceGroupID).
		SetCustomMultiplier(multiplier)
	if source.VideoPrice480p == nil {
		update = update.ClearVideoPrice480p()
	}
	if source.VideoPrice720p == nil {
		update = update.ClearVideoPrice720p()
	}
	if source.VideoPrice1080p == nil {
		update = update.ClearVideoPrice1080p()
	}
	update = update.SetName(name)
	if source.ModelRouting != nil {
		update = update.SetModelRouting(source.ModelRouting)
	} else {
		update = update.ClearModelRouting()
	}
	_, err = update.Save(ctx)
	if err != nil {
		return fmt.Errorf("sync custom group from source: %w", err)
	}
	return nil
}

func (s *PaymentService) copyGroupAccountBindings(ctx context.Context, sourceGroupID, targetGroupID int64) error {
	bindings, err := s.entClient.AccountGroup.Query().
		Where(accountgroup.GroupIDEQ(sourceGroupID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load source group accounts: %w", err)
	}
	if len(bindings) == 0 {
		return nil
	}
	builders := make([]*dbent.AccountGroupCreate, 0, len(bindings))
	for _, binding := range bindings {
		builders = append(builders, s.entClient.AccountGroup.Create().
			SetAccountID(binding.AccountID).
			SetGroupID(targetGroupID).
			SetPriority(binding.Priority))
	}
	if err := s.entClient.AccountGroup.CreateBulk(builders...).
		OnConflictColumns(accountgroup.FieldAccountID, accountgroup.FieldGroupID).
		DoNothing().
		Exec(ctx); err != nil {
		return fmt.Errorf("copy source group accounts: %w", err)
	}
	return nil
}

func (s *PaymentService) syncGroupAccountBindings(ctx context.Context, sourceGroupID, targetGroupID int64) error {
	bindings, err := s.entClient.AccountGroup.Query().
		Where(accountgroup.GroupIDEQ(sourceGroupID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("load source group accounts: %w", err)
	}
	sourceAccountIDs := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		sourceAccountIDs = append(sourceAccountIDs, binding.AccountID)
	}
	if len(sourceAccountIDs) > 0 {
		if _, err := s.entClient.AccountGroup.Delete().
			Where(accountgroup.GroupIDEQ(targetGroupID), accountgroup.AccountIDNotIn(sourceAccountIDs...)).
			Exec(ctx); err != nil {
			return fmt.Errorf("remove stale custom group accounts: %w", err)
		}
	} else {
		if _, err := s.entClient.AccountGroup.Delete().Where(accountgroup.GroupIDEQ(targetGroupID)).Exec(ctx); err != nil {
			return fmt.Errorf("remove stale custom group accounts: %w", err)
		}
		return nil
	}
	for _, binding := range bindings {
		if err := s.entClient.AccountGroup.Create().
			SetAccountID(binding.AccountID).
			SetGroupID(targetGroupID).
			SetPriority(binding.Priority).
			OnConflictColumns(accountgroup.FieldAccountID, accountgroup.FieldGroupID).
			UpdateNewValues().
			Exec(ctx); err != nil {
			return fmt.Errorf("sync source group accounts: %w", err)
		}
	}
	return nil
}

func (s *PaymentService) syncCustomGroupChannelBinding(ctx context.Context, sourceGroupID, targetGroupID int64) error {
	if s == nil || s.entClient == nil || sourceGroupID <= 0 || targetGroupID <= 0 || sourceGroupID == targetGroupID {
		return nil
	}
	deleteSQL := fmt.Sprintf(`DELETE FROM channel_groups WHERE group_id = %d`, targetGroupID)
	if _, err := s.entClient.ExecContext(ctx, deleteSQL); err != nil {
		if isMissingChannelGroupsTableError(err) {
			return nil
		}
		return fmt.Errorf("sync custom group channel binding: remove stale binding: %w", err)
	}
	insertSQL := fmt.Sprintf(`
		INSERT INTO channel_groups (channel_id, group_id)
		SELECT channel_id, %d FROM channel_groups WHERE group_id = %d`, targetGroupID, sourceGroupID)
	if _, err := s.entClient.ExecContext(ctx, insertSQL); err != nil {
		if isMissingChannelGroupsTableError(err) {
			return nil
		}
		return fmt.Errorf("sync custom group channel binding: copy source binding: %w", err)
	}
	return nil
}

func (s *PaymentService) syncCustomGroupAccountStatsPricingRules(ctx context.Context, sourceGroupID, targetGroupID int64) error {
	if s == nil || s.entClient == nil || sourceGroupID <= 0 || targetGroupID <= 0 || sourceGroupID == targetGroupID {
		return nil
	}
	if s.entClient.Driver().Dialect() != dialect.Postgres {
		return nil
	}
	syncSQL := fmt.Sprintf(`
		UPDATE channel_account_stats_pricing_rules
		SET group_ids = CASE
			WHEN %d = ANY(group_ids) AND NOT (%d = ANY(group_ids)) THEN group_ids || %d::bigint
			WHEN NOT (%d = ANY(group_ids)) AND %d = ANY(group_ids) THEN array_remove(group_ids, %d::bigint)
			ELSE group_ids
		END,
		updated_at = NOW()
		WHERE %d = ANY(group_ids) OR %d = ANY(group_ids)`,
		sourceGroupID, targetGroupID, targetGroupID,
		sourceGroupID, targetGroupID, targetGroupID,
		sourceGroupID, targetGroupID)
	if _, err := s.entClient.ExecContext(ctx, syncSQL); err != nil {
		if isMissingCustomGroupOptionalTableError(err) {
			return nil
		}
		return fmt.Errorf("sync custom group account stats pricing rules: %w", err)
	}
	return nil
}

func isMissingChannelGroupsTableError(err error) bool {
	return isMissingCustomGroupOptionalTableError(err)
}

func isMissingCustomGroupOptionalTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table: channel_groups") ||
		strings.Contains(msg, "no such table: channel_account_stats_pricing_rules") ||
		strings.Contains(msg, `relation "channel_groups" does not exist`) ||
		strings.Contains(msg, `relation "channel_account_stats_pricing_rules" does not exist`)
}

func (s *PaymentService) migrateCustomSubscriptionAPIKeys(ctx context.Context, userID, sourceGroupID, customGroupID int64) {
	if s == nil || s.entClient == nil || userID <= 0 || sourceGroupID <= 0 || customGroupID <= 0 || sourceGroupID == customGroupID {
		return
	}
	migrated, err := s.entClient.APIKey.Update().
		Where(
			apikey.UserIDEQ(userID),
			apikey.GroupIDEQ(sourceGroupID),
			apikey.DeletedAtIsNil(),
		).
		SetGroupID(customGroupID).
		Save(ctx)
	if err != nil {
		slog.Warn("custom subscription api key migration failed", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupID", customGroupID, "error", err)
		return
	}
	if migrated <= 0 {
		return
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	slog.Info("custom subscription api keys migrated", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupID", customGroupID, "count", migrated)
}

func (s *PaymentService) migrateLegacyCustomSubscriptionAPIKeysToSource(ctx context.Context, userID, sourceGroupID int64, customGroupIDs []int64) bool {
	if s == nil || s.entClient == nil || userID <= 0 || sourceGroupID <= 0 || len(customGroupIDs) == 0 {
		return false
	}
	filtered := filterLegacyCustomSubscriptionGroupIDs(sourceGroupID, customGroupIDs)
	if len(filtered) == 0 {
		return false
	}
	migrated, err := s.entClient.APIKey.Update().
		Where(
			apikey.UserIDEQ(userID),
			apikey.GroupIDIn(filtered...),
			apikey.DeletedAtIsNil(),
		).
		SetGroupID(sourceGroupID).
		Save(ctx)
	if err != nil {
		slog.Warn("legacy custom subscription api key migration to source failed", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupIDs", filtered, "error", err)
		return false
	}
	if migrated > 0 {
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		slog.Info("legacy custom subscription api keys migrated to source group", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupIDs", filtered, "count", migrated)
	}
	return true
}

func filterLegacyCustomSubscriptionGroupIDs(sourceGroupID int64, customGroupIDs []int64) []int64 {
	filtered := make([]int64, 0, len(customGroupIDs))
	seen := make(map[int64]struct{}, len(customGroupIDs))
	for _, groupID := range customGroupIDs {
		if groupID <= 0 || groupID == sourceGroupID {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		filtered = append(filtered, groupID)
	}
	return filtered
}

func (s *PaymentService) retireLegacyCustomSubscriptionGroups(ctx context.Context, userID, sourceGroupID int64, customGroupIDs []int64) {
	if s == nil || s.entClient == nil || userID <= 0 || sourceGroupID <= 0 || len(customGroupIDs) == 0 {
		return
	}
	filtered := filterLegacyCustomSubscriptionGroupIDs(sourceGroupID, customGroupIDs)
	if len(filtered) == 0 {
		return
	}
	now := time.Now()
	retirableIDs, err := s.entClient.Group.Query().
		Where(
			entgroup.IDIn(filtered...),
			entgroup.IsCustomSubscriptionGroupEQ(true),
			entgroup.CustomOwnerUserIDEQ(userID),
			entgroup.CustomSourceGroupIDEQ(sourceGroupID),
			entgroup.StatusEQ(StatusActive),
			entgroup.Not(entgroup.HasSubscriptionsWith(
				usersubscription.StatusEQ(SubscriptionStatusActive),
				usersubscription.DeletedAtIsNil(),
				activeUserSubscriptionExpiresAt(now),
			)),
		).
		IDs(ctx)
	if err != nil {
		slog.Warn("legacy custom subscription group retirement lookup failed", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupIDs", filtered, "error", err)
		return
	}
	if len(retirableIDs) == 0 {
		return
	}
	retired, err := s.entClient.Group.Update().
		Where(entgroup.IDIn(retirableIDs...)).
		SetStatus(StatusDisabled).
		Save(ctx)
	if err != nil {
		slog.Warn("legacy custom subscription group retirement failed", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupIDs", retirableIDs, "error", err)
		return
	}
	for _, groupID := range retirableIDs {
		if err := s.notifyCustomSubscriptionGroupChanged(ctx, groupID); err != nil {
			slog.Warn("legacy custom subscription group retirement notification failed", "groupID", groupID, "error", err)
		}
	}
	slog.Info("legacy custom subscription groups retired", "userID", userID, "sourceGroupID", sourceGroupID, "customGroupIDs", retirableIDs, "count", retired)
}

type groupChangeNotifier interface {
	NotifyGroupChanged(ctx context.Context, groupID int64) error
}

type channelCacheInvalidator interface {
	InvalidateCache()
}

func (s *PaymentService) notifyCustomSubscriptionGroupChanged(ctx context.Context, groupID int64) error {
	if s == nil || groupID <= 0 {
		return nil
	}
	if s.groupRepo != nil {
		if notifier, ok := any(s.groupRepo).(groupChangeNotifier); ok {
			if err := notifier.NotifyGroupChanged(ctx, groupID); err != nil {
				return fmt.Errorf("notify scheduler for custom subscription group: %w", err)
			}
		}
	}
	if s.channelCacheInvalidator != nil {
		s.channelCacheInvalidator.InvalidateCache()
	}
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
	return nil
}

func applyPaymentOrderSubscriptionGroup(order *dbent.PaymentOrder, groupID int64) *dbent.PaymentOrder {
	if order == nil {
		return nil
	}
	order.SubscriptionGroupID = &groupID
	return order
}
