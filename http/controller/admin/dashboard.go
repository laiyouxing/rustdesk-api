package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

type Dashboard struct {
}

func (d *Dashboard) Stats(c *gin.Context) {
	stats := service.AllService.DashboardService.Stats()
	response.Success(c, stats)
}
