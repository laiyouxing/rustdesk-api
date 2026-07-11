package service

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

type NotifyService struct{}

// SendStationMessage 发送站内离线消息
// 参数:
//
//	receiverId: 接收者用户ID，0=不限制(系统消息)，>0=仅该用户可见
func (s *NotifyService) SendStationMessage(receiverId uint, title, content, peerId string) {
	if title == "" && content == "" {
		return
	}
	msg := &model.StationMessage{
		Type:       "offline",
		Title:      title,
		Content:    content,
		PeerId:     peerId,
		SenderId:   0,
		ReceiverId: receiverId,
	}
	if err := DB.Create(msg).Error; err != nil {
		Logger.Warn("SendStationMessage failed: ", err)
	}
}

func (s *NotifyService) SendByConfig(cfg *model.AlertConfig, title, content string) {
	// 从通道表获取具体配置
	ch := &model.AlertChannel{}
	DB.Where("row_id = ?", cfg.ChannelId).First(ch)
	if ch.RowId == 0 {
		return
	}
	switch ch.Channel {
	case "wecom":
		_ = s.sendWecom(ch.WebhookUrl, title, content)
	case "dingtalk":
		_ = s.sendDingTalk(ch.WebhookUrl, title, content)
	case "smtp":
		// 接收人来自告警规则（发送配置），而非通道
		_ = s.sendSmtpWithChannel(ch, cfg.Recipients, title, content)
	}
}

func (s *NotifyService) sendWecom(webhook, title, content string) error {
	body := fmt.Sprintf(`{"msgtype":"markdown","markdown":{"content":"## ⚠️ 设备离线告警\n**%s**\n%s"}}`, title, content)
	return s.postJson(webhook, body)
}

func (s *NotifyService) sendDingTalk(webhook, title, content string) error {
	body := fmt.Sprintf(`{"msgtype":"text","text":{"content":"⚠️ 设备离线告警\n%s\n%s"}}`, title, content)
	return s.postJson(webhook, body)
}

// sendSmtpWithChannel 支持 465（SMTPS/TLS）和 587（STARTTLS）两种端口
// recipients: 收件人邮箱列表（逗号分隔），来自告警规则的“接收人”配置
func (s *NotifyService) sendSmtpWithChannel(ch *model.AlertChannel, recipientsStr, title, content string) error {
	if ch.SmtpHost == "" || recipientsStr == "" {
		return fmt.Errorf("SMTP 主机或收件人为空")
	}
	addr := net.JoinHostPort(ch.SmtpHost, fmt.Sprintf("%d", ch.SmtpPort))
	auth := smtp.PlainAuth("", ch.SmtpUser, ch.SmtpPass, ch.SmtpHost)

	// Build HTML email body
	htmlContent := buildEmailHTML(title, content)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: =?UTF-8?B?%s?=\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		ch.SmtpUser, recipientsStr, base64Encode(title), htmlContent)
	recipients := strings.Split(recipientsStr, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	deliver := func(client *smtp.Client) error {
		defer client.Close()
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
		if err := client.Mail(ch.SmtpUser); err != nil {
			return fmt.Errorf("SMTP MAIL FROM 失败: %w", err)
		}
		for _, to := range recipients {
			if err := client.Rcpt(to); err != nil {
				return fmt.Errorf("SMTP RCPT 失败(%s): %w", to, err)
			}
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("SMTP DATA 失败: %w", err)
		}
		if _, err := io.Copy(w, strings.NewReader(msg)); err != nil {
			return fmt.Errorf("SMTP 写入正文失败: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("SMTP 关闭正文失败: %w", err)
		}
		return nil
	}

	if ch.SmtpPort == 587 {
		// STARTTLS: 先明文连接，再升级到 TLS
		conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			return fmt.Errorf("SMTP 连接失败: %w", err)
		}
		client, err := smtp.NewClient(conn, ch.SmtpHost)
		if err != nil {
			conn.Close()
			return fmt.Errorf("SMTP 客户端创建失败: %w", err)
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			client.Close()
			conn.Close()
			return fmt.Errorf("SMTP STARTTLS 失败: %w", err)
		}
		return deliver(client)
	}
	// 默认 465 SMTPS: 直接 TLS 连接
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("SMTP TLS 连接失败: %w", err)
	}
	client, err := smtp.NewClient(conn, ch.SmtpHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP 客户端创建失败: %w", err)
	}
	return deliver(client)
}

func (s *NotifyService) postJson(url, body string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		Logger.Warn("Notify post failed: ", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func buildEmailHTML(title, content string) string {
	// Parse content lines into a table
	lines := strings.Split(content, "\n")
	var rows strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "：", 2)
		if len(parts) == 2 {
			rows.WriteString(fmt.Sprintf(`
			<tr>
				<td style="padding: 10px 16px; border-bottom: 1px solid #eee; color: #666; width: 100px; white-space: nowrap; font-weight: bold;">%s</td>
				<td style="padding: 10px 16px; border-bottom: 1px solid #eee; color: #333;">%s</td>
			</tr>`, parts[0], parts[1]))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="margin:0; padding:0; background:#f5f6fa; font-family: -apple-system, 'Microsoft YaHei', sans-serif;">
	<div style="max-width:560px; margin:40px auto; background:#fff; border-radius:10px; box-shadow:0 2px 12px rgba(0,0,0,0.08); overflow:hidden;">
		<div style="background:#e74c3c; padding:24px 30px; text-align:center;">
			<div style="font-size:40px; margin-bottom:8px;">⚠️</div>
			<div style="color:#fff; font-size:20px; font-weight:bold;">%s</div>
		</div>
		<table style="width:100%%; border-collapse:collapse; margin:20px 0;">
			%s
		</table>
		<div style="text-align:center; padding:16px; color:#aaa; font-size:12px; border-top:1px solid #eee;">
			此邮件由 RustDesk 告警系统自动发送
		</div>
	</div>
</body>
</html>`, title, rows.String())
}

// TestChannel 向指定通道发送一条测试消息，返回发送结果错误（nil 表示成功）
func (s *NotifyService) TestChannel(ch *model.AlertChannel, recipients string) error {
	const title = "RustDesk 告警通道测试"
	const content = "这是一条来自 RustDesk 告警系统的测试消息，如果您收到说明配置正确。"
	switch ch.Channel {
	case "wecom":
		return s.sendWecom(ch.WebhookUrl, title, content)
	case "dingtalk":
		return s.sendDingTalk(ch.WebhookUrl, title, content)
	case "smtp":
		if recipients == "" {
			recipients = ch.SmtpUser // 默认发给自己
		}
		return s.sendSmtpWithChannel(ch, recipients, title, content)
	default:
		return fmt.Errorf("不支持的通道类型: %s", ch.Channel)
	}
}
