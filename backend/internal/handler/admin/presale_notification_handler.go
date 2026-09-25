package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *PaymentHandler) PreviewPresaleNotice(c *gin.Context) {
	preview, err := h.presaleNotifications.Preview(c.Request.Context(), c.Query("include_restricted") == "true", c.DefaultQuery("locale", "zh"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, preview)
}
func (h *PaymentHandler) TestPresaleNotice(c *gin.Context)     { h.sendPresaleNotice(c, true) }
func (h *PaymentHandler) SendNextPresaleNotice(c *gin.Context) { h.sendPresaleNotice(c, false) }
func (h *PaymentHandler) UpdatePresaleNoticeConfig(c *gin.Context) {
	var req service.PresaleNoticeConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	config, err := h.presaleNotifications.UpdateConfig(c.Request.Context(), req, getAdminIDFromContext(c))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}
func (h *PaymentHandler) sendPresaleNotice(c *gin.Context, test bool) {
	var req service.PresaleNoticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	var result *service.PresaleNoticeSendResult
	var err error
	if test {
		result, err = h.presaleNotifications.SendTest(c.Request.Context(), req, getAdminIDFromContext(c))
	} else {
		result, err = h.presaleNotifications.SendNext(c.Request.Context(), req, getAdminIDFromContext(c))
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
