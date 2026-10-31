package admin

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/service"
)

// Retention 存储管理：各类数据的保留策略 + 占用统计。
//
// 这些项原先只能改 config.yaml 并重启，现在后台可配且立即生效：
//   - 日志保留天数：保存后立刻按新值清理一次过期日志（logger.SetMaxDays）；
//   - 审计保留天数：保存后下一次巡检（最多 12 小时）生效；如需立刻生效，
//     可传 run_now=true 主动触发一次清理（会真实删除超期记录）；
//   - 站内消息 / 登录日志 / 操作日志：0 表示不自动清理（默认 0，保持原行为）。
//
// 生效值优先级：system_settings 表 > config.yaml > 内置默认值。
type Retention struct {
}

// 取值边界：太小会立刻删掉大量数据，太大等于不清理。
const (
	retentionMinDays = 1
	retentionMaxDays = 3650 // 10 年
	// 允许 0 的项目上限（0=不自动清理）
	retentionZeroMax = 3650
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
// @Tags 存储管理
// @Summary 读取保留策略与生效来源
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/retention/info [get]
// @Security token
func (r *Retention) Info(c *gin.Context) {
	logDays := service.EffectiveLogMaxDays()
	auditDays := service.EffectiveAuditRetentionDays()

	_, logInDB := service.AllService.SettingService.GetInt(service.SettingKeyLogMaxDays)
	_, auditInDB := service.AllService.SettingService.GetInt(service.SettingKeyAuditRetentionDays)
	_, msgInDB := service.AllService.SettingService.GetInt(service.SettingKeyStationMessageRetentionDays)
	_, loginLogInDB := service.AllService.SettingService.GetInt(service.SettingKeyLoginLogRetentionDays)
	_, opLogInDB := service.AllService.SettingService.GetInt(service.SettingKeyOpLogRetentionDays)

	configLogSet := global.Config.Logger.MaxDays > 0
	configAuditSet := global.Config.Audit.RetentionDays > 0

	response.Success(c, gin.H{
		"log_max_days":         logDays,
		"audit_retention_days": auditDays,
		"station_message_days": service.EffectiveStationMessageRetentionDays(),
		"login_log_days":       service.EffectiveLoginLogRetentionDays(),
		"op_log_days":          service.EffectiveOpLogRetentionDays(),

		"log_source":             sourceOf(logInDB, configLogSet),
		"audit_source":           sourceOf(auditInDB, configAuditSet),
		"station_message_source": sourceOf(msgInDB, false),
		"login_log_source":       sourceOf(loginLogInDB, false),
		"op_log_source":          sourceOf(opLogInDB, false),

		// 当前正在写入的日志文件（按天轮转后的实际路径），便于运维确认
		"log_file":           logger.CurrentLogPath(),
		"log_days_in_effect": logger.CurrentMaxDays(),
	})
}

// Stats 存储占用统计（各表行数 + 数据库与日志目录磁盘占用）
// @Tags 存储管理
// @Summary 存储占用统计
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/retention/stats [get]
// @Security token
func (r *Retention) Stats(c *gin.Context) {
	response.Success(c, service.AllService.StorageRetentionService.CollectStorageStats())
}

// Update 保存保留策略
// @Tags 存储管理
// @Summary 保存保留策略
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/retention/update [post]
// @Security token
func (r *Retention) Update(c *gin.Context) {
	type Req struct {
		LogMaxDays        int  `json:"log_max_days"`
		AuditRetentionDay int  `json:"audit_retention_days"`
		StationMessageDay int  `json:"station_message_days"`
		LoginLogDay       int  `json:"login_log_days"`
		OpLogDay          int  `json:"op_log_days"`
		RunNow            bool `json:"run_now"`
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
	// 这三类允许 0（= 不自动清理）
	zeroAllowed := []struct {
		name string
		v    int
	}{
		{"站内消息保留天数", req.StationMessageDay},
		{"登录日志保留天数", req.LoginLogDay},
		{"操作日志保留天数", req.OpLogDay},
	}
	for _, it := range zeroAllowed {
		if it.v < 0 || it.v > retentionZeroMax {
			response.Fail(c, 101, it.name+"需在 0~3650 之间（0 表示不自动清理）")
			return
		}
	}

	svc := service.AllService.SettingService
	pairs := []struct {
		key string
		v   int
	}{
		{service.SettingKeyLogMaxDays, req.LogMaxDays},
		{service.SettingKeyAuditRetentionDays, req.AuditRetentionDay},
		{service.SettingKeyStationMessageRetentionDays, req.StationMessageDay},
		{service.SettingKeyLoginLogRetentionDays, req.LoginLogDay},
		{service.SettingKeyOpLogRetentionDays, req.OpLogDay},
	}
	for _, p := range pairs {
		if err := svc.Set(p.key, strconv.Itoa(p.v)); err != nil {
			response.Fail(c, 101, "保存设置失败("+p.key+")："+err.Error())
			return
		}
	}

	// 日志立刻生效：更新写入器的保留天数，并按新值清理一次
	logger.SetMaxDays(req.LogMaxDays)
	global.Logger.Infof("[Retention] 日志保留天数已更新为 %d 天（按天轮转，已按新值清理一次）", req.LogMaxDays)

	if req.RunNow {
		// 异步执行：表可能很大，避免请求超时
		go func() {
			if n := service.AllService.AuditRetentionService.PruneNow(); n > 0 {
				global.Logger.Infof("[Retention] 已按新策略（%d 天）立即清理审计记录 %d 条", req.AuditRetentionDay, n)
			}
			if n := service.AllService.StorageRetentionService.PruneNow(); n > 0 {
				global.Logger.Infof("[Retention] 已按新策略立即清理消息/日志类记录 %d 条", n)
			}
		}()
	}
	global.Logger.Infof(
		"[Retention] 保留策略已更新：审计 %d 天、站内消息 %d 天、登录日志 %d 天、操作日志 %d 天（0=不清理；%s）",
		req.AuditRetentionDay, req.StationMessageDay, req.LoginLogDay, req.OpLogDay,
		map[bool]string{true: "并已触发立即清理", false: "下一次巡检生效"}[req.RunNow],
	)

	response.Success(c, gin.H{
		"log_max_days":         req.LogMaxDays,
		"audit_retention_days": req.AuditRetentionDay,
		"station_message_days": req.StationMessageDay,
		"login_log_days":       req.LoginLogDay,
		"op_log_days":          req.OpLogDay,
		"run_now":              req.RunNow,
	})
}
