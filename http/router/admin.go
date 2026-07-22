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
	target := &admin.AlertTargetCtl{}
	rg.GET("/targets", target.List)
	rg.POST("/targets/create", target.Create)
	rg.POST("/targets/delete", target.Delete)
	rg.GET("/available_collections", target.AvailableCollections)
	rg.GET("/available_peers", target.AvailablePeers)
}

func StationMessageBind(adg *gin.RouterGroup) {
	cont := &admin.StationMessage{}
	rg := adg.Group("/station_message")
	rg.GET("/list", cont.List)
	rg.GET("/unread_count", cont.UnreadCount)
	rg.POST("/mark_read", cont.MarkRead)
	rg.POST("/send", cont.Send)
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
		aR.POST("/groupUsers", cont.GroupUsers)
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
	cont := &admin.AuditLog{}
	aR := rg.Group("/audit").Use(middleware.AdminPrivilege())
	aR.GET("/list", cont.List)
}

func AddressBookCollectionBind(rg *gin.RouterGroup) {
	aR := rg.Group("/address_book_collection").Use(middleware.AdminPrivilege())
	{
		cont := &admin.AddressBookCollection{}
		aR.GET("/list", cont.List)
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
		aR.POST("/create", cont.Create)
		aR.POST("/update", cont.Update)
		aR.POST("/delete", cont.Delete)
	}
}

func UserTokenBind(rg *gin.RouterGroup) {
	cont := &admin.UserToken{}
	rg2 := rg.Group("/user_tokens").Use(middleware.AdminPrivilege())
	rg2.GET("/list", cont.List)
	rg2.POST("/batchDelete", cont.BatchDelete)
}

func ShareRecordBind(adg *gin.RouterGroup) {
	cont := &admin.ShareRecord{}
	rg := adg.Group("/share_record").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/delete", cont.Delete)
}

func StrategyBind(adg *gin.RouterGroup) {
	cont := &admin.Strategy{}
	rg := adg.Group("/strategy").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/update", cont.Update)
	rg.POST("/delete", cont.Delete)
}

func VersionBind(adg *gin.RouterGroup) {
	cont := &admin.VersionRelease{}
	rg := adg.Group("/version_release").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/update", cont.Update)
	rg.POST("/delete", cont.Delete)
	rg.GET("/detail", cont.Detail)
}

func ClientDownloadBind(adg *gin.RouterGroup) {
	cont := &admin.ClientDownload{}
	rg := adg.Group("/client_download")
	rg.GET("/config", cont.Config)
	rg.GET("/list", cont.List)
	rg.GET("/versions", cont.Versions)
}

func FileBind(adg *gin.RouterGroup) {
	cont := &admin.File{}
	rg := adg.Group("/file")
	rg.GET("/download", cont.Download)
	rg.POST("/upload", cont.Upload)
}

func ConfigBind(adg *gin.RouterGroup) {
	cont := &admin.Config{}
	rg := adg.Group("/config")
	rg.GET("/get", cont.Get)
	rg.POST("/set", cont.Set)
}

func MyBind(rg *gin.RouterGroup) {
	aR := rg.Group("/my")
	{
		cont := &my.My{}
		aR.GET("/info", cont.Info)
		aR.POST("/update", cont.Update)
		aR.POST("/changePwd", cont.ChangePwd)
		aR.POST("/bindMfa", cont.BindMfa)
		aR.POST("/unbindMfa", cont.UnbindMfa)
		aR.POST("/changeAvatar", cont.ChangeAvatar)
		aR.POST("/changeLang", cont.ChangeLang)
		aR.GET("/menuItems", cont.MenuItems)
		aR.POST("/bindOidc", cont.BindOidc)
		// 分享记录
		aR.GET("/shareRecords", (&my.MyShare{}).List)
		aR.POST("/createShareRecord", (&my.MyShare{}).Create)
		aR.POST("/deleteShareRecord", (&my.MyShare{}).Delete)
	}
	aR2 := rg.Group("/my")
	{
		cont := &my.MyPeer{}
		aR2.GET("/peers", cont.List)
	}
	aR3 := rg.Group("/my")
	{
		cont := &my.MyLoginLog{}
		aR3.GET("/loginLogs", cont.List)
	}
}

func ProcessMonitorBind(adg *gin.RouterGroup) {
	cont := &admin.ProcessMonitor{}
	rg := adg.Group("/process_monitor").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/update", cont.Update)
}

func BackupBind(adg *gin.RouterGroup) {
	cont := &admin.Backup{}
	rg := adg.Group("/backup").Use(middleware.AdminPrivilege())
	rg.GET("/list", cont.List)
	rg.POST("/create", cont.Create)
	rg.POST("/do", cont.DoBackup)
	rg.POST("/restore", cont.Restore)
	rg.POST("/delete", cont.Delete)
}
