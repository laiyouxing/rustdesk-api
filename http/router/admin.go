package router

import (
	"github.com/gin-gonic/gin"
	_ "github.com/lejianwen/rustdesk-api/v2/docs/admin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/admin"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/admin/my"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func Init(g *gin.Engine) {

	//swagger
	//g.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	if global.Config.App.ShowSwagger == 1 {
		g.GET("/admin/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName("admin")))
	}

	adg := g.Group("/api/admin")
	LoginBind(adg)
	adg.POST("/user/register", (&admin.User{}).Register)

	ConfigBind(adg)

	adg.Use(middleware.BackendUserAuth())
	FileBind(adg)
	UserBind(adg)
	GroupBind(adg)
	TagBind(adg)
	AddressBookBind(adg)
	PeerBind(adg)
	OauthBind(adg)
	LoginLogBind(adg)
	AuditBind(adg)
	AddressBookCollectionBind(adg)
	AddressBookCollectionRuleBind(adg)
	UserTokenBind(adg)

	//deprecated by ConfigBind
	//rs := &admin.Rustdesk{}
	//adg.GET("/server-config", rs.ServerConfig)
	//adg.GET("/app-config", rs.AppConfig)
	//deprecated end

	ShareRecordBind(adg)
	StrategyBind(adg)
	VersionBind(adg)
	ClientDownloadBind(adg)
	ServerStatusBind(adg)
	MyBind(adg)

	DashboardBind(adg)
	AlertChannelBind(adg)
	AlertConfigBind(adg)
	StationMessageBind(adg)
	RustdeskCmdBind(adg)
	DeviceGroupBind(adg)
	BackupBind(adg)
	ProcessMonitorBind(adg)
	InvitationBind(adg)
	//访问静态文件
	//g.StaticFS("/upload", http.Dir(global.Config.Gin.ResourcesPath+"/upload"))
}

func DashboardBind(adg *gin.RouterGroup) {
	cont := &admin.Dashboard{}
	rg := adg.Group("/dashboard")
	rg.GET("/stats", cont.Stats)
}

func AlertChannelBind(adg *gin.RouterGroup) {
	cont := &admin.AlertChannel{}
	rg := adg.Group("/alert_channel").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.GET("/all", cont.AllList)
	rg.POST("/create", cont.Create)
	rg.POST("/update", cont.Update)
	rg.POST("/delete", cont.Delete)
	rg.POST("/test", cont.Test)
}

func AlertConfigBind(adg *gin.RouterGroup) {
	cont := &admin.AlertConfig{}
	rg := adg.Group("/alert_config").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/update", cont.Update)
	rg.POST("/delete", cont.Delete)
	// alert targets
	target := &admin.AlertTargetCtl{}
	rg.GET("/targets", target.List)
	rg.POST("/targets/create", target.Create)
	rg.POST("/targets/delete", target.Delete)
	// available collections/peers for selection
	rg.GET("/available_collections", target.AvailableCollections)
	rg.GET("/available_peers", target.AvailablePeers)
}

func StationMessageBind(adg *gin.RouterGroup) {
	cont := &admin.StationMessage{}
	rg := adg.Group("/station_message")
	rg.GET("/list", cont.List)
	rg.GET("/unread_count", cont.UnreadCount)
	rg.POST("/mark_read", cont.MarkRead)
	// 发送消息：所有登录用户均可发送
	rg.POST("/send", cont.Send)
	// 广播和清理需要管理员权限
	rg.POST("/broadcast", middleware.AdminPrivilege(), cont.Broadcast)
	rg.POST("/cleanup", middleware.AdminPrivilege(), cont.Cleanup)
}

func ServerStatusBind(adg *gin.RouterGroup) {
	cont := &admin.ServerStatus{}
	rg := adg.Group("/server_status").Use(middleware.AdminPrivilege())
	rg.GET("/", cont.Status)
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/update", cont.Update)
	rg.POST("/delete", cont.Delete)
}

func RustdeskCmdBind(adg *gin.RouterGroup) {
	cont := &admin.Rustdesk{}
	rg := adg.Group("/rustdesk")
	rg.POST("/sendCmd", cont.SendCmd)
	rg.GET("/cmdList", cont.CmdList)
	rg.POST("/cmdDelete", cont.CmdDelete)
	rg.POST("/cmdCreate", cont.CmdCreate)
}
func LoginBind(rg *gin.RouterGroup) {
	cont := &admin.Login{}
	rg.POST("/login", middleware.Limiter(), cont.Login)
	rg.POST("/login/mfa", cont.MfaLogin)
	rg.GET("/captcha", cont.Captcha)
	rg.POST("/logout", cont.Logout)
	rg.GET("/login-options", cont.LoginOptions)
	rg.POST("/oidc/auth", cont.OidcAuth)
	rg.GET("/oidc/auth-query", cont.OidcAuthQuery)
}

func UserBind(rg *gin.RouterGroup) {
	aR := rg.Group("/user")
	{
		cont := &admin.User{}
		aR.GET("/current", cont.Current)
		aR.POST("/changeCurPwd", cont.ChangeCurPwd)
		aR.POST("/myOauth", cont.MyOauth)
		//aR.GET("/myPeer", cont.MyPeer)
		aR.POST("/groupUsers", cont.GroupUsers)
		// MFA(TOTP) 自服务：当前登录用户自身的多因素认证管理
		aR.POST("/mfa/setup", cont.MfaSetup)
		aR.POST("/mfa/enable", cont.MfaEnable)
		aR.POST("/mfa/disable", cont.MfaDisable)
		aR.GET("/mfa/status", cont.MfaStatus)
	}
	aRP := rg.Group("/user").Use(middleware.AdminPrivilege())
	{
		cont := &admin.User{}
		aRP.GET("/list", cont.List)
		aRP.GET("/detail/:id", cont.Detail)
		aRP.POST("/create", cont.Create)
		aRP.POST("/update", cont.Update)
		aRP.POST("/delete", cont.Delete)
		aRP.POST("/changePwd", cont.UpdatePassword)
		// 管理员强制重置用户 MFA（救援）
		aRP.POST("/mfa/reset", cont.MfaReset)
	}
}

func GroupBind(rg *gin.RouterGroup) {
	aR := rg.Group("/group").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Group{}
		aR.GET("/list", cont.List)
		aR.GET("/tree", cont.Tree)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func DeviceGroupBind(rg *gin.RouterGroup) {
	aR := rg.Group("/device_group").Use(middleware.AdminPrivilege())
	{
		cont := &admin.DeviceGroup{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func TagBind(rg *gin.RouterGroup) {
	aR := rg.Group("/tag").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Tag{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func AddressBookBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book")
	{
		cont := &admin.AddressBook{}
		aR.POST("/shareByWebClient", cont.ShareByWebClient)

		arp := aR.Use(middleware.AdminPrivilege())
		arp.GET("/list", cont.List)
		//arp.GET("/detail/:id", cont.Detail)
		arp.POST("/create", cont.Create)
		arp.POST("/update", cont.Update)
		arp.POST("/delete", cont.Delete)
		arp.POST("/batchCreate", cont.BatchCreate)
		arp.POST("/batchCreateFromPeers", cont.BatchCreateFromPeers)

	}
}
func PeerBind(rg *gin.RouterGroup) {
	aR := rg.Group("/peer")
	aR.POST("/simpleData", (&admin.Peer{}).SimpleData)
	aR.Use(middleware.AdminPrivilege())
	{
		cont := &admin.Peer{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
		aR.POST("/batchDelete", cont.BatchDelete)
	}
}

func OauthBind(rg *gin.RouterGroup) {
	aR := rg.Group("/oauth")
	{
		cont := &admin.Oauth{}
		aR.POST("/confirm", cont.Confirm)
		aR.POST("/bind", cont.ToBind)
		aR.POST("/bindConfirm", cont.BindConfirm)
		aR.POST("/unbind", cont.Unbind)
		aR.GET("/info", cont.Info)
	}
	arp := aR.Use(middleware.AdminPrivilege())
	{
		cont := &admin.Oauth{}
		arp.GET("/list", cont.List)
		arp.GET("/detail/:id", cont.Detail)
		arp.POST("/create", cont.Create)
		arp.POST("/update", cont.Update)
		arp.POST("/delete", cont.Delete)

	}

}
func LoginLogBind(rg *gin.RouterGroup) {
	cont := &admin.LoginLog{}
	aR := rg.Group("/login_log").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.List)
	aR.POST("/delete", cont.Delete)
	aR.POST("/batchDelete", cont.BatchDelete)
}
func AuditBind(rg *gin.RouterGroup) {
	cont := &admin.Audit{}
	aR := rg.Group("/audit_conn").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.ConnList)
	aR.POST("/delete", cont.ConnDelete)
	aR.POST("/batchDelete", cont.BatchConnDelete)
	afR := rg.Group("/audit_file").Use(middleware.AdminPrivilege())
	afR.GET("/list", cont.FileList)
	afR.POST("/delete", cont.FileDelete)
	afR.POST("/batchDelete", cont.BatchFileDelete)
}
func AddressBookCollectionBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book_collection").Use(middleware.AdminPrivilege())
	{
		cont := &admin.AddressBookCollection{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}

}
func AddressBookCollectionRuleBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book_collection_rule").Use(middleware.AdminPrivilege())
	{
		cont := &admin.AddressBookCollectionRule{}
		aR.GET("/list", cont.List)
		aR.GET("/detail/:id", cont.Detail)
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}
func UserTokenBind(rg *gin.RouterGroup) {
	aR := rg.Group("/user_token").Use(middleware.AdminPrivilege())
	cont := &admin.UserToken{}
	aR.GET("/list", cont.List)
	aR.POST("/delete", cont.Delete)
	aR.POST("/batchDelete", cont.BatchDelete)
}
func ConfigBind(rg *gin.RouterGroup) {
	aR := rg.Group("/config")
	rs := &admin.Config{}

	// 注意：/config/admin 必须在 BackendUserAuth() 之前注册，是有意为之——
	// 登录页需要读取后台标题(title)做品牌展示，此时用户尚未登录。
	// 该接口仅返回 title / hello（hello 来自服务端配置指定的文件，非用户输入），
	// 不暴露数据库密码、OSS key、JWT key 等敏感配置，故保持公开。
	// 若未来需要锁定，请同时确认前端登录页的标题获取方式，避免破坏登录流程。
	aR.GET("/admin", rs.AdminConfig)

	aR.Use(middleware.BackendUserAuth())
	aR.GET("/server", rs.ServerConfig)
	aR.GET("/app", rs.AppConfig)

	// 配置文件读写：仅管理员
	aRf := aR.Group("/file").Use(middleware.AdminPrivilege())
	aRf.GET("/get", rs.ConfigFileGet)
	aRf.POST("/update", rs.ConfigFileUpdate)

	// 重启服务：仅管理员
	aR.POST("/restart", middleware.AdminPrivilege(), rs.ServiceRestart)
}

func FileBind(rg *gin.RouterGroup) {
	aR := rg.Group("/file")
	{
		cont := &admin.File{}
		aR.POST("/notify", cont.Notify)
		aR.OPTIONS("/oss_token", nil)
		aR.OPTIONS("/upload", nil)
		aR.GET("/oss_token", cont.OssToken)
		aR.POST("/upload", cont.Upload)
	}
}

func MyBind(rg *gin.RouterGroup) {
	{
		cont := &my.ShareRecord{}
		rg.GET("/my/share_record/list", cont.List)
		rg.POST("/my/share_record/delete", cont.Delete)
		rg.POST("/my/share_record/batchDelete", cont.BatchDelete)
	}

	{
		cont := &my.AddressBook{}
		rg.GET("/my/address_book/list", cont.List)
		rg.POST("/my/address_book/create", cont.Create)
		rg.POST("/my/address_book/update", cont.Update)
		rg.POST("/my/address_book/delete", cont.Delete)
		rg.POST("/my/address_book/batchCreateFromPeers", cont.BatchCreateFromPeers)
		rg.POST("/my/address_book/batchUpdateTags", cont.BatchUpdateTags)
	}

	{
		cont := &my.Tag{}
		rg.GET("/my/tag/list", cont.List)
		rg.POST("/my/tag/create", cont.Create)
		rg.POST("/my/tag/update", cont.Update)
		rg.POST("/my/tag/delete", cont.Delete)
	}

	{
		cont := &my.AddressBookCollection{}
		rg.GET("/my/address_book_collection/list", cont.List)
		rg.POST("/my/address_book_collection/create", cont.Create)
		rg.POST("/my/address_book_collection/update", cont.Update)
		rg.POST("/my/address_book_collection/delete", cont.Delete)
	}

	{
		cont := &my.AddressBookCollectionRule{}
		rg.GET("/my/address_book_collection_rule/list", cont.List)
		rg.POST("/my/address_book_collection_rule/create", cont.Create)
		rg.POST("/my/address_book_collection_rule/update", cont.Update)
		rg.POST("/my/address_book_collection_rule/delete", cont.Delete)
	}
	{
		cont := &my.Peer{}
		rg.GET("/my/peer/list", cont.List)

	}

	{
		cont := &my.LoginLog{}
		rg.GET("/my/login_log/list", cont.List)
		rg.POST("/my/login_log/delete", cont.Delete)
		rg.POST("/my/login_log/batchDelete", cont.BatchDelete)
	}
	{
		cont := &my.Audit{}
		rg.GET("/my/audit_conn/list", cont.List)
	}
}

func ShareRecordBind(rg *gin.RouterGroup) {
	aR := rg.Group("/share_record").Use(middleware.AdminPrivilege())
	{
		cont := &admin.ShareRecord{}
		aR.GET("/list", cont.List)
		aR.POST("/delete", cont.Delete)
		aR.POST("/batchDelete", cont.BatchDelete)
	}

}

func StrategyBind(rg *gin.RouterGroup) {
	sR := rg.Group("/strategy").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Strategy{}
		sR.GET("/list", cont.List)
		sR.POST("/create", cont.Create)
		sR.POST("/update", cont.Update)
		sR.POST("/delete", cont.Delete)
	}
}

func VersionBind(rg *gin.RouterGroup) {
	vR := rg.Group("/version").Use(middleware.AdminPrivilege())
	{
		cont := &admin.Version{}
		vR.GET("/list", cont.List)
		vR.POST("/create", cont.Create)
		vR.POST("/update", cont.Update)
		vR.POST("/delete", cont.Delete)
		vR.POST("/setEnable", cont.SetEnable)
	}
}

func ClientDownloadBind(rg *gin.RouterGroup) {
	vR := rg.Group("/client_download").Use(middleware.AdminPrivilege())
	{
		cont := &admin.ClientDownload{}
		vR.GET("/list", cont.List)
		vR.POST("/create", cont.Create)
		vR.POST("/update", cont.Update)
		vR.POST("/delete", cont.Delete)
		vR.POST("/setEnable", cont.SetEnable)
	}
}

func BackupBind(adg *gin.RouterGroup) {
	rg := adg.Group("/backup").Use(middleware.AdminPrivilege())
	cont := &admin.BackupCtl{}
	rg.GET("/config", cont.Config)
	rg.GET("/database", cont.Database)
}

func ProcessMonitorBind(adg *gin.RouterGroup) {
	cont := &admin.ProcessMonitor{}
	rg := adg.Group("/process_monitor").Use(middleware.AdminPrivilege())
	rg.GET("/rules", cont.RuleList)
	rg.POST("/rule/create", cont.RuleCreate)
	rg.POST("/rule/batch_create", cont.RuleBatchCreate)
	rg.POST("/rule/update", cont.RuleUpdate)
	rg.POST("/rule/delete", cont.RuleDelete)
	rg.GET("/status", cont.StatusList)
	rg.GET("/peer_sources", cont.PeerSources)
}

func InvitationBind(adg *gin.RouterGroup) {
	cont := &admin.Invitation{}
	rg := adg.Group("/invitation").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/batchCreate", cont.BatchCreate)
	rg.POST("/delete", cont.Delete)
	rg.GET("/info", cont.Info)
}
