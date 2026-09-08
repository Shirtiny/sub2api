package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// Only the semantic opening gate may declare HTTP writes to be heartbeat-only.
// Ordinary/legacy failover errors keep the existing conservative size guard.
func stopOpenAIStreamFailover(c *gin.Context, err *service.UpstreamFailoverError, sizeBefore int) bool {
	return err.StopLocalRetry || (!err.ResponseUncommitted && c.Writer.Size() != sizeBefore)
}
