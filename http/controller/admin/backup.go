package admin

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/config"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
)

type BackupCtl struct{}

// Config 导出配置文件
func (b *BackupCtl) Config(ctx *gin.Context) {
	cfgPath := global.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfig
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		response.Fail(ctx, 500, "读取配置文件失败: "+err.Error())
		return
	}

	filename := fmt.Sprintf("rustdesk-api-config-%s.yaml", time.Now().Format("20060102_150405"))
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Data(200, "application/octet-stream", data)
}

// Database 导出数据库备份
func (b *BackupCtl) Database(ctx *gin.Context) {
	dbType := global.Config.Gorm.Type

	var filename string
	var data []byte
	var err error

	switch dbType {
	case config.TypeSqlite:
		filename = fmt.Sprintf("rustdesk-api-db-%s.db", time.Now().Format("20060102_150405"))
		data, err = b.exportSqlite()
	case config.TypeMysql:
		filename = fmt.Sprintf("rustdesk-api-db-%s.sql", time.Now().Format("20060102_150405"))
		data, err = b.exportMysql()
	case config.TypePostgresql:
		filename = fmt.Sprintf("rustdesk-api-db-%s.sql", time.Now().Format("20060102_150405"))
		data, err = b.exportPostgresql()
	default:
		response.Fail(ctx, 400, "不支持的数据库类型: "+dbType)
		return
	}

	if err != nil {
		response.Fail(ctx, 500, "导出数据库失败: "+err.Error())
		return
	}

	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Data(200, "application/octet-stream", data)
}

func (b *BackupCtl) exportSqlite() ([]byte, error) {
	dbPath := "./data/rustdeskapi.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// 尝试从工作目录或其他常见路径查找
		absPath, _ := filepath.Abs(dbPath)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("数据库文件不存在: %s", dbPath)
		}
		dbPath = absPath
	}
	return os.ReadFile(dbPath)
}

func (b *BackupCtl) exportMysql() ([]byte, error) {
	mysql := global.Config.Mysql
	addr := mysql.Addr
	if addr == "" {
		addr = "127.0.0.1:3306"
	}
	host := addr
	port := "3306"
	if parts := strings.SplitN(addr, ":", 2); len(parts) == 2 {
		host = parts[0]
		port = parts[1]
	}

	args := []string{
		"-h" + host,
		"-P" + port,
		"-u" + mysql.Username,
		"-p" + mysql.Password,
		mysql.Dbname,
	}

	cmd := exec.Command("mysqldump", args...)
	return cmd.Output()
}

func (b *BackupCtl) exportPostgresql() ([]byte, error) {
	pg := global.Config.Postgresql
	port := pg.Port
	if port == "" {
		port = "5432"
	}

	// 使用 PGPASSWORD 环境变量传递密码
	cmd := exec.Command("pg_dump",
		"-h"+pg.Host,
		"-p"+port,
		"-U"+pg.User,
		"-d"+pg.Dbname,
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+pg.Password)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 stderr pipe 失败: %w", err)
	}

	out, err := cmd.Output()
	if err != nil {
		errMsg, _ := io.ReadAll(stderr)
		if len(errMsg) > 0 {
			return nil, fmt.Errorf("pg_dump 失败: %s", string(errMsg))
		}
		return nil, fmt.Errorf("pg_dump 执行失败: %w", err)
	}
	return out, nil
}
