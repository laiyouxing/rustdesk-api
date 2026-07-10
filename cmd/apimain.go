package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http"
	"github.com/lejianwen/rustdesk-api/v2/lib/cache"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/lib/logger"
	"github.com/lejianwen/rustdesk-api/v2/lib/orm"
	"github.com/lejianwen/rustdesk-api/v2/lib/upload"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

const DatabaseVersion = 265

// @title 管理系统API
// @version 1.0
// @description 接口
// @basePath /api
// @securityDefinitions.apikey token
// @in header
// @name api-token
// @securitydefinitions.apikey BearerAuth
// @in header
// @name Authorization

var rootCmd = &cobra.Command{
	Use:   "apimain",
	Short: "RUSTDESK API SERVER",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		InitGlobal()
	},
	Run: func(cmd *cobra.Command, args []string) {
		global.Logger.Info("API SERVER START")
		http.ApiInit()
		// 后台定时清理“进行中”的孤儿连接审计记录（旧客户端退出未发 close 等场景）
		go service.AllService.AuditService.StartStaleConnCloseSweep()
	},
}

var resetPwdCmd = &cobra.Command{
	Use:     "reset-admin-pwd [pwd]",
	Example: "reset-admin-pwd 123456",
	Short:   "Reset Admin Password",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pwd := args[0]
		admin := service.AllService.UserService.InfoById(1)
		if admin.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err := service.AllService.UserService.UpdatePassword(admin, pwd)
		if err != nil {
			global.Logger.Error("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success! ")
	},
}
var resetUserPwdCmd = &cobra.Command{
	Use:     "reset-pwd [userId] [pwd]",
	Example: "reset-pwd 2 123456",
	Short:   "Reset User Password",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		userId := args[0]
		pwd := args[1]
		uid, err := strconv.Atoi(userId)
		if err != nil {
			global.Logger.Warn("userId must be int!")
			return
		}
		if uid <= 0 {
			global.Logger.Warn("userId must be greater than 0! ")
			return
		}
		u := service.AllService.UserService.InfoById(uint(uid))
		if u.Id == 0 {
			global.Logger.Warn("user not found! ")
			return
		}
		err = service.AllService.UserService.UpdatePassword(u, pwd)
		if err != nil {
			global.Logger.Warn("reset password fail! ", err)
			return
		}
		global.Logger.Info("reset password success!")
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&global.ConfigPath, "config", "c", "./conf/config.yaml", "choose config file")
	rootCmd.AddCommand(resetPwdCmd, resetUserPwdCmd)
}
func main() {
	if err := rootCmd.Execute(); err != nil {
		global.Logger.Error(err)
		os.Exit(1)
	}
}

func InitGlobal() {
	//配置解析
	global.Viper = config.Init(&global.Config, global.ConfigPath)

	//日志
	global.Logger = logger.New(&logger.Config{
		Path:         global.Config.Logger.Path,
		Level:        global.Config.Logger.Level,
		ReportCaller: global.Config.Logger.ReportCaller,
	})

	global.InitI18n()

	//cache（按需初始化 Redis，避免闲置占用资源）
	if global.Config.Cache.Type == cache.TypeFile {
		fc := cache.NewFileCache()
		fc.SetDir(global.Config.Cache.FileDir)
		global.Cache = fc
	} else if global.Config.Cache.Type == cache.TypeRedis {
		global.Redis = redis.NewClient(&redis.Options{
			Addr:     global.Config.Cache.RedisAddr,
			Password: global.Config.Cache.RedisPwd,
			DB:       global.Config.Cache.RedisDb,
		})
		global.Cache = cache.NewRedis(&redis.Options{
			Addr:     global.Config.Cache.RedisAddr,
			Password: global.Config.Cache.RedisPwd,
			DB:       global.Config.Cache.RedisDb,
		})
	}
	//gorm
	if global.Config.Gorm.Type == config.TypeMysql {

		dsn := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
			global.Config.Mysql.Username,
			global.Config.Mysql.Password,
			global.Config.Mysql.Addr,
			global.Config.Mysql.Dbname,
			global.Config.Mysql.Tls,
		)

		global.DB = orm.NewMysql(&orm.MysqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else if global.Config.Gorm.Type == config.TypePostgresql {
		dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			global.Config.Postgresql.Host,
			global.Config.Postgresql.Port,
			global.Config.Postgresql.User,
			global.Config.Postgresql.Password,
			global.Config.Postgresql.Dbname,
			global.Config.Postgresql.Sslmode,
			global.Config.Postgresql.TimeZone,
		)
		global.DB = orm.NewPostgresql(&orm.PostgresqlConfig{
			Dsn:          dsn,
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	} else {
		//sqlite
		global.DB = orm.NewSqlite(&orm.SqliteConfig{
			MaxIdleConns: global.Config.Gorm.MaxIdleConns,
			MaxOpenConns: global.Config.Gorm.MaxOpenConns,
		}, global.Logger)
	}

	//validator
	global.ApiInitValidator()

	//oss
	global.Oss = &upload.Oss{
		AccessKeyId:     global.Config.Oss.AccessKeyId,
		AccessKeySecret: global.Config.Oss.AccessKeySecret,
		Host:            global.Config.Oss.Host,
		CallbackUrl:     global.Config.Oss.CallbackUrl,
		ExpireTime:      global.Config.Oss.ExpireTime,
		MaxByte:         global.Config.Oss.MaxByte,
	}

	//jwt
	//fmt.Println(global.Config.Jwt.PrivateKey)
	global.Jwt = jwt.NewJwt(global.Config.Jwt.Key, global.Config.Jwt.ExpireDuration)
	//locker
	global.Lock = lock.NewLocal()

	//service
	service.New(&global.Config, global.DB, global.Logger, global.Jwt, global.Lock)
	service.AllService.ProcessMonitorService = &service.ProcessMonitorService{}
	service.AllService.AlertService.StartChecker()

	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{
		CaptchaThreshold: global.Config.App.CaptchaThreshold,
		BanThreshold:     global.Config.App.BanThreshold,
		AttemptsWindow:   10 * time.Minute,
		BanDuration:      30 * time.Minute,
	})
	global.LoginLimiter.RegisterProvider(utils.B64StringCaptchaProvider{})
	DatabaseAutoUpdate()
}

func DatabaseAutoUpdate() {
	version := DatabaseVersion

	db := global.DB

	if global.Config.Gorm.Type == config.TypeMysql {
		//检查存不存在数据库，不存在则创建
		dbName := db.Migrator().CurrentDatabase()
		if dbName == "" {
			dbName = global.Config.Mysql.Dbname
			// 移除 DSN 中的数据库名称，以便初始连接时不指定数据库
			dsnWithoutDB := fmt.Sprintf("%s:%s@(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				global.Config.Mysql.Username,
				global.Config.Mysql.Password,
				global.Config.Mysql.Addr,
				"",
			)

			//新链接
			dbWithoutDB := orm.NewMysql(&orm.MysqlConfig{
				Dsn: dsnWithoutDB,
			}, global.Logger)
			// 获取底层的 *sql.DB 对象，并确保在程序退出时关闭连接
			sqlDBWithoutDB, err := dbWithoutDB.DB()
			if err != nil {
				global.Logger.Errorf("获取底层 *sql.DB 对象失败: %v", err)
				return
			}
			defer func() {
				if err := sqlDBWithoutDB.Close(); err != nil {
					global.Logger.Errorf("关闭连接失败: %v", err)
				}
			}()

			err = dbWithoutDB.Exec("CREATE DATABASE IF NOT EXISTS " + dbName + " DEFAULT CHARSET utf8mb4").Error
			if err != nil {
				global.Logger.Error(err)
				return
			}
		}
	}

	if !db.Migrator().HasTable(&model.Version{}) {
		Migrate(uint(version))
	} else {
		//查找最后一个version
		var v model.Version
		db.Last(&v)
		if v.Version < uint(version) {
			Migrate(uint(version))
		}

		// 245迁移
		if v.Version < 245 {
			//oauths 表的 oauth_type 字段设置为 op同样的值
			db.Exec("update oauths set oauth_type = op")
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google'")
			db.Exec("update user_thirds set oauth_type = third_type, op = third_type")
			//通过email迁移旧的google授权
			uts := make([]model.UserThird, 0)
			db.Where("oauth_type = ?", "google").Find(&uts)
			for _, ut := range uts {
				if ut.UserId > 0 {
					db.Model(&model.User{}).Where("id = ?", ut.UserId).Update("email", ut.OpenId)
				}
			}
		}
		if v.Version < 246 {
			db.Exec("update oauths set issuer = 'https://accounts.google.com' where op = 'google' and issuer is null")
		}
	}

	// 旧数据迁移：将旧 AlertConfig 中的通道信息迁移到 AlertChannel 表
	if db.Migrator().HasColumn(&model.AlertConfig{}, "webhook_url") {
		var oldCfgs []struct {
			RowId      uint
			UserId     uint
			Channel    string
			Name       string
			WebhookUrl string `gorm:"column:webhook_url"`
			SmtpHost   string `gorm:"column:smtp_host"`
			SmtpPort   int    `gorm:"column:smtp_port"`
			SmtpUser   string `gorm:"column:smtp_user"`
			SmtpPass   string `gorm:"column:smtp_pass"`
			SmtpTo     string `gorm:"column:smtp_to"`
		}
		db.Table("alert_configs").Where("channel_id = 0").Find(&oldCfgs)
		for _, oc := range oldCfgs {
			ch := &model.AlertChannel{
				UserId:     oc.UserId,
				Channel:    oc.Channel,
				Name:       oc.Name,
				WebhookUrl: oc.WebhookUrl,
				SmtpHost:   oc.SmtpHost,
				SmtpPort:   oc.SmtpPort,
				SmtpUser:   oc.SmtpUser,
				SmtpPass:   oc.SmtpPass,
			}
			db.Create(ch)
			db.Model(&model.AlertConfig{}).Where("row_id = ?", oc.RowId).Update("channel_id", ch.RowId)
			// 旧 alert_configs.smtp_to（接收人）迁移到新的 recipients 字段
			if oc.SmtpTo != "" {
				db.Model(&model.AlertConfig{}).Where("row_id = ?", oc.RowId).Update("recipients", oc.SmtpTo)
			}
		}
	}

	// 兜底迁移：确保所有新表都存在。AutoMigrate 是幂等的，
	// 已存在的表/列不会被改动。修复旧版本升级时因 version 记录
	// 已是最新而跳过 Migrate，导致新增表（如 app_releases/station_messages）缺失的问题。
	// 对每个模型单独 AutoMigrate，避免一个失败影响其他。
	fallbackModels := []interface{}{
		&model.Version{},
		&model.AppRelease{},
		&model.User{},
		&model.UserToken{},
		&model.Tag{},
		&model.AddressBook{},
		&model.Peer{},
		&model.Group{},
		&model.UserThird{},
		&model.Oauth{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.AuditConn{},
		&model.AuditFile{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.ServerCmd{},
		&model.DeviceGroup{},
		&model.AlertChannel{},
		&model.AlertConfig{},
		&model.AlertTarget{},
		&model.StationMessage{},
		&model.ClientDownload{},
		&model.Strategy{},
		&model.ProcessMonitorRule{},
		&model.ProcessMonitorStatus{},
	}
	for _, m := range fallbackModels {
		if err := db.AutoMigrate(m); err != nil {
			global.Logger.Errorf("fallback migrate %T err: %v", m, err)
		}
	}

	// 终极兜底：用原生 SQL 确保关键表存在（应对 AutoMigrate 可能因 GORM 版本差异失败的场景）
	ensureTable(db, &model.AppRelease{}, "app_releases", `CREATE TABLE IF NOT EXISTS app_releases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version varchar(32) NOT NULL DEFAULT '',
		platform varchar(16) NOT NULL DEFAULT '',
		url varchar(512) NOT NULL DEFAULT '',
		note text,
		status tinyint DEFAULT 1,
		created_at datetime,
		updated_at datetime
	)`)
	ensureTable(db, &model.StationMessage{}, "station_messages", `CREATE TABLE IF NOT EXISTS station_messages (
		row_id INTEGER PRIMARY KEY AUTOINCREMENT,
		type varchar(32) NOT NULL DEFAULT '',
		title varchar(200) NOT NULL DEFAULT '',
		content text,
		peer_id varchar(128) NOT NULL DEFAULT '',
		sender_id integer NOT NULL DEFAULT 0,
		sender_name varchar(100) NOT NULL DEFAULT '',
		receiver_id integer NOT NULL DEFAULT 0,
		is_read integer NOT NULL DEFAULT 0,
		created_at integer
	)`)
	ensureTable(db, &model.ClientDownload{}, "client_downloads", `CREATE TABLE IF NOT EXISTS client_downloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version varchar(32) NOT NULL DEFAULT '',
		platform varchar(16) NOT NULL DEFAULT '',
		url varchar(512) NOT NULL DEFAULT '',
		note text,
		status tinyint DEFAULT 1,
		created_at datetime,
		updated_at datetime
	)`)
}

func ensureTable(db *gorm.DB, m interface{}, tableName, createSQL string) {
	if !db.Migrator().HasTable(m) {
		global.Logger.Warnf("表 %s 不存在，尝试用原生 SQL 创建...", tableName)
		if err := db.Exec(createSQL).Error; err != nil {
			global.Logger.Errorf("原生 SQL 创建表 %s 失败: %v", tableName, err)
		} else {
			global.Logger.Infof("原生 SQL 创建表 %s 成功", tableName)
		}
	}
}
func Migrate(version uint) {
	global.Logger.Info("Migrating....", version)
	err := global.DB.AutoMigrate(
		&model.Version{},
		&model.AppRelease{},
		&model.User{},
		&model.UserToken{},
		&model.Tag{},
		&model.AddressBook{},
		&model.Peer{},
		&model.Group{},
		&model.UserThird{},
		&model.Oauth{},
		&model.LoginLog{},
		&model.ShareRecord{},
		&model.AuditConn{},
		&model.AuditFile{},
		&model.AddressBookCollection{},
		&model.AddressBookCollectionRule{},
		&model.ServerCmd{},
		&model.DeviceGroup{},
		&model.AlertChannel{},
		&model.AlertConfig{},
		&model.AlertTarget{},
		&model.StationMessage{},
		&model.ClientDownload{},
		&model.Strategy{},
		&model.ProcessMonitorRule{},
		&model.ProcessMonitorStatus{},
	)
	if err != nil {
		global.Logger.Error("migrate err :=>", err)
	}
	global.DB.Create(&model.Version{Version: version})
	//如果是初次则创建一个默认用户
	var vc int64
	global.DB.Model(&model.Version{}).Count(&vc)
	if vc == 1 {
		localizer := global.Localizer("")
		defaultGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "DefaultGroup",
		})
		group := &model.Group{
			Name: defaultGroup,
			Type: model.GroupTypeDefault,
		}
		service.AllService.GroupService.Create(group)

		shareGroup, _ := localizer.LocalizeMessage(&i18n.Message{
			ID: "ShareGroup",
		})
		groupShare := &model.Group{
			Name: shareGroup,
			Type: model.GroupTypeShare,
		}
		service.AllService.GroupService.Create(groupShare)
		//是true
		is_admin := true
		admin := &model.User{
			Username: "admin",
			Nickname: "Admin",
			Status:   model.COMMON_STATUS_ENABLE,
			IsAdmin:  &is_admin,
			GroupId:  1,
		}

		// 生成随机密码
		pwd := utils.RandomString(8)
		global.Logger.Info("Admin Password Is: ", pwd)
		var err error
		admin.Password, err = utils.EncryptPassword(pwd)
		if err != nil {
			global.Logger.Fatalf("failed to generate admin password: %v", err)
		}
		global.DB.Create(admin)
	}

}
