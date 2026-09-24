package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// AdminOrderSummary adds safe display metadata, not current prices/entitlements.
// Names loaded from related records are explicitly marked as current in the UI.
type AdminOrderSummary struct {
	Currency       string `json:"currency"`
	PaymentMode    string `json:"payment_mode,omitempty"`
	PlanName       string `json:"plan_name,omitempty"`
	PlanNameSource string `json:"plan_name_source,omitempty"`
	GroupName      string `json:"group_name,omitempty"`
	ProviderName   string `json:"provider_name,omitempty"`
}

func (s *PaymentService) GetAdminOrderSummary(ctx context.Context, order *dbent.PaymentOrder) (*AdminOrderSummary, error) {
	summary := &AdminOrderSummary{Currency: PaymentOrderCurrency(order)}
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil {
		summary.PaymentMode = snapshot.PaymentMode
	}
	if order.OrderType == payment.OrderTypeSubscription {
		if name := strings.TrimSpace(order.PresalePlanName); name != "" {
			summary.PlanName, summary.PlanNameSource = name, "snapshot"
		} else if order.PlanID != nil {
			plan, err := s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.IDEQ(*order.PlanID)).Select(subscriptionplan.FieldID, subscriptionplan.FieldName).Only(ctx)
			if err != nil && !dbent.IsNotFound(err) {
				return nil, fmt.Errorf("load order plan name: %w", err)
			}
			if plan != nil {
				summary.PlanName, summary.PlanNameSource = plan.Name, "current"
			}
		}
		groupID := order.SubscriptionSourceGroupID
		if groupID == nil {
			groupID = order.SubscriptionGroupID
		}
		if groupID != nil {
			g, err := s.entClient.Group.Query().Where(group.IDEQ(*groupID)).Select(group.FieldID, group.FieldName).Only(ctx)
			if err != nil && !dbent.IsNotFound(err) {
				return nil, fmt.Errorf("load order group name: %w", err)
			}
			if g != nil {
				summary.GroupName = g.Name
			}
		}
	}
	if id, err := strconv.ParseInt(psStringValue(order.ProviderInstanceID), 10, 64); err == nil && id > 0 {
		provider, err := s.entClient.PaymentProviderInstance.Query().Where(paymentproviderinstance.IDEQ(id)).Select(paymentproviderinstance.FieldID, paymentproviderinstance.FieldName).Only(ctx)
		if err != nil && !dbent.IsNotFound(err) {
			return nil, fmt.Errorf("load order provider name: %w", err)
		}
		if provider != nil {
			summary.ProviderName = provider.Name
		}
	}
	return summary, nil
}
