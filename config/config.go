package config

import (
	"fmt"
	"github.com/spf13/viper"
	"strings"
	"time"
)

const (
	DebugMode     = "debug"
	ReleaseMode   = "release"
	DefaultConfig = "conf/config.yaml"
)

type App struct {
	WebClient          int           `mapstructure:"web-client"`
	Register           bool          `mapstructure:"register"`
	RegisterStatus     int           `mapstructure:"register-status"`
	ShowSwagger        int           `mapstructure:"show-swagger"`
	TokenExpire        time.Duration `mapstructure:"token-expire"`
	WebSso             bool          `mapstructure:"web-sso"`
	DisablePwdLogin    bool          `mapstructure:"disable-pwd-login"`
	CaptchaThreshold   int           `mapstructure:"captcha-threshold"`
	BanThreshold       int           `mapstructure:"ban-threshold"`
	BanWindowMinutes   int           `mapstructure:"ban-window-minutes"`
	BanDurationMinutes int           `mapstructure:"ban-duration-minutes"`
}
type Admin struct {
	Title           string `mapstructure:"title"`
	Hello           string `mapstructure:"hello"`
	HelloFile       string `mapstructure:"hello-file"`
	IdServerPort    int    `mapstructure:"id-server-port"`
	RelayServerPort int    `mapstructure:"relay-server-port"`
	// RelayStatsHost 用于查询 hbbr 负载/连接数的回环地址。
	// hbbr 仅接受来自本机(loopback)的命令连接，因此 api-server 必须与 hbbr 部署在同一主机，
	// 且此处使用 127.0.0.1（或该主机的回环地址）。
	RelayStatsHost string `mapstructure:"relay-stats-host"`
}
type Cors struct {
	// AllowOrigins 允许跨域的源白名单（YAML 列表），例如 ["https://admin.example.com"]。
	// 为空则关闭跨域（不返回 Access-Control-Allow-Origin）。详见 http/middleware/cors.go。
	AllowOrigins []string `mapstructure:"allow-origins"`
}

// Server 控制 HTTP 服务本身的超时与可选 TLS（安全加固项）。
type Server struct {
	// ReadTimeout 读超时（秒）：读取整个请求（含 body）的时限，用于缓解慢速攻击。默认 15。
	ReadTimeout int `mapstructure:"read-timeout"`
	// WriteTimeout 写超时（秒）：写入响应的时限。默认 30。
	WriteTimeout int `mapstructure:"write-timeout"`
	// IdleTimeout 空闲超时（秒）：keep-alive 空闲连接的最大存活时间。默认 120。
	IdleTimeout int `mapstructure:"idle-timeout"`
	// TLS 可选 TLS 配置；默认关闭（走明文 HTTP，由前置反向代理做 TLS 终止）。
	TLS ServerTLS `mapstructure:"tls"`
}

// ServerTLS 可选 TLS 配置。启用后 api-server 直接以 HTTPS 监听。
type ServerTLS struct {
	// Enabled 是否启用 TLS（true 时 api-server 直接监听 HTTPS）。
	Enabled bool `mapstructure:"enabled"`
	// CertFile 证书文件路径（PEM），如 /path/to/fullchain.pem。
	CertFile string `mapstructure:"cert-file"`
	// KeyFile 私钥文件路径（PEM），如 /path/to/privkey.pem。
	KeyFile string `mapstructure:"key-file"`
}
type Config struct {
	Lang       string `mapstructure:"lang"`
	App        App
	Admin      Admin
	Gorm       Gorm
	Mysql      Mysql
	Postgresql Postgresql
	Gin        Gin
	Logger     Logger
	Redis      Redis
	Cache      Cache
	Oss        Oss
	Jwt        Jwt
	Rustdesk   Rustdesk
	Proxy      Proxy
	Ldap       Ldap
	Cors       Cors
	Server     Server
	// MfaTotpSkew 允许 TOTP 校验容忍的时钟漂移周期数（每个周期 30s）。
	// 标准库 totp.Validate 默认 Skew=1（仅容忍 ±30s）；运维中服务器/客户端时钟偏差
	// 或用户输入耗时偶发会踩线失败，故默认放宽为 3（±90s）。运维可在 config.yaml 中
	// 调整（mfa_totp_skew），无需重新编译。取值 <1 时由 service 回退到默认 3。
	MfaTotpSkew int `mapstructure:"mfa_totp_skew"`
}

func (a *Admin) Init() {
	if a.IdServerPort == 0 {
		a.IdServerPort = DefaultIdServerPort
	}
	if a.RelayServerPort == 0 {
		a.RelayServerPort = DefaultRelayServerPort
	}
	if a.RelayStatsHost == "" {
		a.RelayStatsHost = "127.0.0.1"
	}
}

// Init 初始化配置
func Init(rowVal *Config, path string) *viper.Viper {
	if path == "" {
		path = DefaultConfig
	}
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.SetEnvPrefix("RUSTDESK_API")
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	err := v.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	/*
		v.WatchConfig()


			//监听配置修改没什么必要
			v.OnConfigChange(func(e fsnotify.Event) {
				//配置文件修改监听
				fmt.Println("config file changed:", e.Name)
				if err2 := v.Unmarshal(rowVal); err2 != nil {
					fmt.Println(err2)
				}
				rowVal.Rustdesk.LoadKeyFile()
				rowVal.Rustdesk.ParsePort()
			})
	*/
	if err := v.Unmarshal(rowVal); err != nil {
		panic(fmt.Errorf("Fatal error config: %s \n", err))
	}
	rowVal.Rustdesk.LoadKeyFile()
	rowVal.Admin.Init()
	return v
}

// ReadEnv 读取环境变量
func ReadEnv(rowVal interface{}) *viper.Viper {
	v := viper.New()
	v.AutomaticEnv()
	if err := v.Unmarshal(rowVal); err != nil {
		fmt.Println(err)
	}
	return v
}
