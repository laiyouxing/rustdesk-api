package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"net/http"
)

type Dashboard struct{}

func (d *Dashboard) Page(c *gin.Context) {
	c.HTML(http.StatusOK, "dashboard_admin.html", gin.H{})
}

func (d *Dashboard) Stats(c *gin.Context) {
	u := service.AllService.UserService.CurUser(c)
	if service.AllService.UserService.IsAdmin(u) {
		stats := service.AllService.DashboardService.Stats()
		response.Success(c, stats)
	} else {
		stats := service.AllService.DashboardService.UserStats(u.Id)
		response.Success(c, stats)
	}
}
