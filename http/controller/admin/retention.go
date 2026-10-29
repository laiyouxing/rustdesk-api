package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Retention 保留策略配置（日志 / 审计记录）
//
// 这两项原先只能改 config.yaml 并重启，现在后台可配且立即生效：
//   - 日志保留天数：保存后立刻按新值清理一次过期日志（logger.SetMaxDays）；
//   - 审计保留天数：保存后下一次巡检（最多 12 小时）生效；如需立刻生效，
//     可传 run_now=true 主动触发一次清理（会真实删除超期记录）。
//
// 生效值优先级：system_settings 表 > config.yaml > 内置默认值。
type Retention struct {
}

// 取值边界：太小会立刻删掉大量数据，太大等于不清理。
const (
	retentionMinDays = 1
	retentionMaxDays = 3650 // 10 年
)

func sourceOf(dbHas bool, configSet bool) string {
	if dbHas {
		return "database"
	}
	if configSet {
		return "config"
	}
	return "default"
}

// Info 读取当前保留策略
// @Tags 保留策略
// @Summary 读取日志/审计保留策略
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/retention/info [get]
// @Security token
func (r *Retention) Info(c *gin.Context) {
	logDays := service.EffectiveLogMaxDays()
	auditDays := service.EffectiveAuditRetentionDays()

	_, logInDB := service.AllService.SettingService.GetInt(service.SettingKeyLogMaxDays)
	_, auditInDB := service.AllService.SettingService.GetInt(service.SettingKeyAuditRetentionDays)

	configLogSet := global.Config.Logger.MaxDays > 0
	configAuditSet := global.Config.Audit.RetentionDays > 0

	response.Success(c, gin.H{
		"log_max_days":         logDays,
		"audit_retention_days": auditDays,
		"log_source":           sourceOf(logInDB, configLogSet),
		"audit_source":         sourceOf(auditInDB, configAuditSet),
		// 当前正在写入的日志文件（按天轮转后的实际路径），便于运维确认
		"log_file": logger.CurrentLogPath(),
		"log_days_in_effect": logger.CurrentMaxDays(),
	})
}

// Update 保存保留策略
// @Tags 保留策略
// @Summary 保存日志/审计保留策略
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/retention/update [post]
// @Security token
func (r *Retention) Update(c *gin.Context) {
	type Req struct {
		LogMaxDays        int  `json:"log_max_days"`
		AuditRetentionDay int  `json:"audit_retention_days"`
		RunNow            bool `json:"run_now"` // 是否立即按新策略清理一次审计记录
	}
	var req Req
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if req.LogMaxDays < retentionMinDays || req.LogMaxDays > retentionMaxDays {
		response.Fail(c, 101, "日志保留天数需在 1~3650 之间")
		return
	}
	if req.AuditRetentionDay < retentionMinDays || req.AuditRetentionDay > retentionMaxDays {
		response.Fail(c, 101, "审计保留天数需在 1~3650 之间")
		return
	}

	svc := service.AllService.SettingService
	if err := svc.Set(service.SettingKeyLogMaxDays, strconv.Itoa(req.LogMaxDays)); err != nil {
		response.Fail(c, 101, "保存日志保留天数失败："+err.Error())
		return
	}
	if err := svc.Set(service.SettingKeyAuditRetentionDays, strconv.Itoa(req.AuditRetentionDay)); err != nil {
		response.Fail(c, 101, "保存审计保留天数失败："+err.Error())
		return
	}

	// 日志立刻生效：更新写入器的保留天数，并按新值清理一次
	logger.SetMaxDays(req.LogMaxDays)
	global.Logger.Infof("[Retention] 日志保留天数已更新为 %d 天（按天轮转，已按新值清理一次）", req.LogMaxDays)

	if req.RunNow {
		// 异步执行：审计表可能很大，避免请求超时
		go func() {
			if n := service.AllService.AuditRetentionService.PruneNow(); n > 0 {
				global.Logger.Infof("[Retention] 已按新策略（%d 天）立即清理审计记录 %d 条", req.AuditRetentionDay, n)
			}
		}()
	}
	global.Logger.Infof("[Retention] 审计保留天数已更新为 %d 天（下一次巡检生效%s）",
		req.AuditRetentionDay, map[bool]string{true: "，并已触发立即清理", false: ""}[req.RunNow])

	response.Success(c, gin.H{
		"log_max_days":         req.LogMaxDays,
		"audit_retention_days": req.AuditRetentionDay,
		"run_now":              req.RunNow,
	})
}
