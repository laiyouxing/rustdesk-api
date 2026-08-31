package main

import (
	"fmt"
	"io"
	nethttp "net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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
	"github.com/sirupsen/logrus"
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
		// 后台定时清理孤儿连接审计记录
		go service.AllService.AuditService.StartStaleConnCloseSweep()
		// 基于心跳 conns 的快速连接心跳检测（60s 超时），检测异常断开
		go service.AllService.AuditService.StartConnHeartbeatSweep()
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

	// 启动早期（配置已加载、日志已就绪，HTTP 服务尚未启动）打印时钟快照，
	// 用于主动发现服务器时钟漂移——这正是此前 MFA 校验偶发失败的根因环境。
	logClockSnapshot()

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
	// SECURITY: JWT 签名密钥为空时，MFA 令牌生成会静默返回空串，
	// 导致前端 MFA 流程无法完成（mfa_token="" 触发 "MFA令牌为必填字段" 错误）。
	// 此处启动时即拦截，避免运行时才暴露问题。
	if len(global.Jwt.Key) == 0 {
		global.Logger.Fatalf("[SECURITY] jwt.key 为空！请在 conf/config.yaml 中配置 jwt.key（建议 openssl rand -hex 生成）。" +
			"jwt.key 为空会导致 MFA 多因素认证流程完全不可用。")
	}
	//locker
	global.Lock = lock.NewLocal()

	//service
	service.New(&global.Config, global.DB, global.Logger, global.Jwt, global.Lock)
	service.AllService.ProcessMonitorService = &service.ProcessMonitorService{}
	service.AllService.ServerStatusService = &service.ServerStatusService{}
	service.AllService.AlertService.StartChecker()

	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{
		CaptchaThreshold: global.Config.App.CaptchaThreshold,
		BanThreshold:     global.Config.App.BanThreshold,
		AttemptsWindow:   banWindowDuration(),
		BanDuration:      banDurationDuration(),
	})
	global.LoginLimiter.RegisterProvider(utils.B64StringCaptchaProvider{})
	DatabaseAutoUpdate()
	syncQRImages()
}

// syncQRImages 将配置的收款码图片从绝对路径复制到 resources/static/qr/ 目录下，
// 以便 getQRURL 构造的 /static/qr/{filename} URL 能正确访问到文件。
func syncQRImages() {
	if global.Config.Payment.Cashier.SiteName == "" {
		return
	}
	qrDir := global.Config.Gin.ResourcesPath + "/static/qr"
	if err := os.MkdirAll(qrDir, 0755); err != nil {
		global.Logger.Errorf("创建收款码目录失败: %v", err)
		return
	}

	cashier := global.Config.Payment.Cashier
	paths := map[string]string{
		"支付宝收款码": cashier.AlipayQR,
		"微信收款码":  cashier.WechatQR,
	}
	for name, src := range paths {
		if src == "" || strings.HasPrefix(src, "http") {
			continue
		}
		if !filepath.IsAbs(src) {
			continue
		}
		dst := qrDir + "/" + filepath.Base(src)
		if err := copyFile(src, dst); err != nil {
			global.Logger.Errorf("复制%s图片失败 %s -> %s: %v", name, src, dst, err)
		} else {
			global.Logger.Infof("收款码图片已复制: %s -> %s", name, dst)
		}
	}
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}
	return d.Sync()
}

// defaultClockCheckURL 启动时时钟快照使用的参考时间源。
// 该站点会在响应头返回标准 RFC1123 的 Date 字段，作为"权威参考时间"。
// 在无外网环境（本地/CI）取不到时间时，会优雅跳过，绝不影响启动。
const defaultClockCheckURL = "https://www.tencent.com"

// logClockSnapshot 启动时打印一条时钟快照 INFO 日志：本地 UTC 时间 vs 参考时间（HTTP Date 头）及偏移。
// 用于主动发现服务器时钟漂移——这正是此前 MFA 校验偶发失败的潜在根因环境。
//
// 健壮性约束（务必满足）：
//   - 单次网络请求超时硬上限 3s，避免无外网时拖慢启动；
//   - 任何网络错误/超时/无 Date 头/解析失败，均仅 Warn 跳过，绝不 panic、绝不阻塞启动；
//   - 若 global.Logger 尚未就绪，退化为独立的 logrus 实例，保证日志本身不崩溃。
func logClockSnapshot() {
	lg := global.Logger
	if lg == nil {
		// 兜底：理论上 InitGlobal 已初始化 Logger；此处仅防极端时序，确保时钟快照永不 panic。
		lg = logrus.New()
	}

	client := &nethttp.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(defaultClockCheckURL)
	if err != nil {
		lg.Warnf("[CLOCK] 跳过时钟快照：无法连接参考时间源 %s: %v", defaultClockCheckURL, err)
		return
	}
	defer resp.Body.Close()

	dateStr := resp.Header.Get("Date")
	if dateStr == "" {
		lg.Warnf("[CLOCK] 跳过时钟快照：参考时间源 %s 未返回 Date 响应头", defaultClockCheckURL)
		return
	}
	refTime, err := time.Parse(time.RFC1123, dateStr)
	if err != nil {
		lg.Warnf("[CLOCK] 跳过时钟快照：解析 Date 头失败 %q: %v", dateStr, err)
		return
	}

	// 统一换算到 UTC 比较，消除本地时区差异；offsetMs 带符号：
	// 正数表示本机时钟"快于"参考时间，负数表示"慢于"参考时间。
	local := time.Now().UTC()
	offsetMs := local.Sub(refTime).Milliseconds()
	lg.Infof("[CLOCK] local=%s ref=%s offsetMs=%d",
		local.Format(time.RFC3339), refTime.Format(time.RFC3339), offsetMs)
}

// isValidDatabaseName 校验数据库名格式（MySQL 标识符限制）：
// 仅允许字母/数字/下划线，且必须以字母或下划线开头，长度不超过 64。
// 用于 CREATE DATABASE 前的纵深防御，避免库名被污染时产生 SQL 注入。
func isValidDatabaseName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	matched, err := regexp.MatchString(`^[A-Za-z_][A-Za-z0-9_]*$`, name)
	if err != nil {
		return false
	}
	return matched
}

// banWindowDuration 返回登录失败计数的滑动窗口（分钟），缺失或非法(<=0)时回退默认 15 分钟。
func banWindowDuration() time.Duration {
	m := global.Config.App.BanWindowMinutes
	if m <= 0 {
		m = 15
	}
	return time.Duration(m) * time.Minute
}

// banDurationDuration 返回触发封禁后的封禁时长（分钟），缺失或非法(<=0)时回退默认 30 分钟。
func banDurationDuration() time.Duration {
	m := global.Config.App.BanDurationMinutes
	if m <= 0 {
		m = 30
	}
	return time.Duration(m) * time.Minute
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

			// 安全加固：执行 CREATE DATABASE 前校验库名格式，防止库名被污染时产生注入。
			// 即便通过校验，也用反引号包裹标识符作为纵深防御。
			if !isValidDatabaseName(dbName) {
				global.Logger.Errorf("数据库名格式非法，已拒绝执行 CREATE DATABASE: %q", dbName)
				return
			}
			err = dbWithoutDB.Exec("CREATE DATABASE IF NOT EXISTS `" + dbName + "` DEFAULT CHARSET utf8mb4").Error
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
		&model.ProcessMonitorRulePeer{},
		&model.ProcessMonitorStatus{},
		&model.ServerStatusMonitor{},
		&model.PayOrder{},
		&model.InviteCode{},
		&model.Announcement{},
		&model.AccountOpLog{},
		&model.AddressBookOpLog{},
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
		&model.ProcessMonitorRulePeer{},
		&model.ProcessMonitorStatus{},
		&model.ServerStatusMonitor{},
		&model.PayOrder{},
		&model.InviteCode{},
		&model.AccountOpLog{},
		&model.AddressBookOpLog{},
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
		// SECURITY: 初始 admin 密码属于高敏感凭据，绝不能写入文件日志（./runtime/log.txt），
		// 否则任何能读取日志文件的人都能拿到管理员密码。这里仅一次性打印到 stderr（控制台，
		// 不会持久化到日志文件），并提示操作员首次登录后立即修改。
		fmt.Fprintf(os.Stderr, "\n[INIT] Generated initial admin password: %s\n[INIT] Please change it after first login!\n\n", pwd)
		var err error
		admin.Password, err = utils.EncryptPassword(pwd)
		if err != nil {
			global.Logger.Fatalf("failed to generate admin password: %v", err)
		}
		global.DB.Create(admin)
	}

}
