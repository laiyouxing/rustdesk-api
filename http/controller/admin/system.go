package admin

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
)

// ServiceRestart 重启后端服务进程，仅管理员可用
// 重启策略（按优先级自动探测）：
//  1. 环境变量 RUSTDESK_API_RESTART_CMD 指定了自定义命令（空格分隔），直接执行；
//  2. systemd 环境（/run/systemd/system 存在或 INVOCATION_ID 已设置）：执行 systemctl restart <服务名>，
//     服务名取自 RUSTDESK_API_SYSTEMD_SERVICE，默认 rustdesk-api.service；
//  3. s6 环境（/run/s6-rc/servicedirs/api 存在）：执行 s6-svc -r <服务目录>；
//  4. 兜底：无守护进程时，自我重新执行当前二进制文件。
//
// 由于重启会中断当前进程，接口先返回成功，再延迟 1 秒后执行重启动作。
//
// @Tags ADMIN
// @Summary 重启后端服务
// @Description 重启 api-server 进程，使修改后的配置生效。仅管理员可用
// @Produce json
// @Success 200 {object} response.Response
// @Router /admin/config/restart [post]
// @Security token
func (co *Config) ServiceRestart(c *gin.Context) {
	go func() {
		time.Sleep(1 * time.Second)
		doRestart()
	}()
	response.Success(c, nil)
}

// doRestart 执行实际的重启逻辑
func doRestart() {
	// 1. 自定义重启命令
	if cmdStr := os.Getenv("RUSTDESK_API_RESTART_CMD"); cmdStr != "" {
		fields := strings.Fields(cmdStr)
		if len(fields) > 0 {
			_ = exec.Command(fields[0], fields[1:]...).Run()
			return
		}
	}

	// 2. systemd
	if isSystemd() {
		svc := os.Getenv("RUSTDESK_API_SYSTEMD_SERVICE")
		if svc == "" {
			svc = "rustdesk-api.service"
		}
		// 优先 systemctl，失败回退 service 命令
		if err := exec.Command("systemctl", "restart", svc).Run(); err == nil {
			return
		}
		_ = exec.Command("service", strings.TrimSuffix(svc, ".service"), "restart").Run()
		return
	}

	// 3. s6
	if _, err := os.Stat("/run/s6-rc/servicedirs/api"); err == nil {
		_ = exec.Command("s6-svc", "-r", "/run/s6-rc/servicedirs/api").Run()
		return
	}

	// 4. 兜底：自我重新执行
	_ = selfExecRestart()
}

// isSystemd 判断当前进程是否运行在 systemd 管理下
func isSystemd() bool {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	return false
}

// selfExecRestart 无守护进程时，重新执行当前二进制文件
func selfExecRestart() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	argv := os.Args
	if len(argv) == 0 {
		argv = []string{exe}
	}
	cmd := exec.Command(exe, argv[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if wd, werr := os.Getwd(); werr == nil {
		cmd.Dir = wd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// 给子进程一点时间接管，再退出当前进程（无守护进程时不会有重复进程）
	time.Sleep(500 * time.Millisecond)
	global.Logger.Info("self restart, exiting old process")
	os.Exit(0)
	return nil
}
