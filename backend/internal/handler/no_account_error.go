package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type noAccountErrorClassification struct {
	Status        int
	ErrType       string
	Message       string
	ModelNotFound bool
}

// classifyNoAccountErrorFromGin keeps Grok's no-capacity response stable on
// custom builds that do not include main's optional model-availability diagnoser.
func classifyNoAccountErrorFromGin(
	_ *gin.Context,
	_ any,
	_ *service.APIKey,
	_ string,
	_ string,
	_ string,
) noAccountErrorClassification {
	return noAccountErrorClassification{
		Status:  http.StatusServiceUnavailable,
		ErrType: "api_error",
		Message: "Service temporarily unavailable",
	}
}
