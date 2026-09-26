package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// These routes live under the admin users group and share its admin middleware.
func (h *SettingHandler) GetUserConcurrencyRules(c *gin.Context) {
	rules, err := h.settingService.GetUserConcurrencyRules(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, rules)
}

func (h *SettingHandler) UpdateUserConcurrencyRules(c *gin.Context) {
	var input struct {
		BalanceTiers []struct {
			MinBalance  *float64 `json:"min_balance"`
			Concurrency int      `json:"concurrency"`
		} `json:"balance_tiers"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.BadRequest(c, "Invalid concurrency rules")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Invalid concurrency rules")
		return
	}
	rules := service.UserConcurrencyRules{BalanceTiers: make([]service.BalanceConcurrencyRule, 0, len(input.BalanceTiers))}
	for _, tier := range input.BalanceTiers {
		if tier.MinBalance == nil {
			response.BadRequest(c, "Every tier requires a balance threshold")
			return
		}
		rules.BalanceTiers = append(rules.BalanceTiers, service.BalanceConcurrencyRule{MinBalance: *tier.MinBalance, Concurrency: tier.Concurrency})
	}
	if err := h.settingService.SetUserConcurrencyRules(c.Request.Context(), rules); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	slog.InfoContext(c.Request.Context(), "user concurrency rules updated", "user_id", subject.UserID, "balance_tiers", rules.BalanceTiers)
	response.Success(c, rules)
}
