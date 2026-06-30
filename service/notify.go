package service

import (
	"bytes"
	"crypto/tls"
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
	switch cfg.Channel {
	case "wecom":
		s.sendWecom(cfg.WebhookUrl, title, content)
	case "dingtalk":
		s.sendDingTalk(cfg.WebhookUrl, title, content)
	case "smtp":
		s.sendSmtp(cfg, title, content)
	}
}

func (s *NotifyService) sendWecom(webhook, title, content string) {
	body := fmt.Sprintf(`{"msgtype":"markdown","markdown":{"content":"## ⚠️ 设备离线告警\n**%s**\n%s"}}`, title, content)
	s.postJson(webhook, body)
}

func (s *NotifyService) sendDingTalk(webhook, title, content string) {
	body := fmt.Sprintf(`{"msgtype":"text","text":{"content":"⚠️ 设备离线告警\n%s\n%s"}}`, title, content)
	s.postJson(webhook, body)
}

// sendSmtp 支持 465（SMTPS/TLS）和 587（STARTTLS）两种端口
func (s *NotifyService) sendSmtp(cfg *model.AlertConfig, title, content string) {
	if cfg.SmtpHost == "" || cfg.SmtpTo == "" {
		return
	}
	addr := fmt.Sprintf("%s:%d", cfg.SmtpHost, cfg.SmtpPort)
	auth := smtp.PlainAuth("", cfg.SmtpUser, cfg.SmtpPass, cfg.SmtpHost)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		cfg.SmtpUser, cfg.SmtpTo, title, content)
	recipients := strings.Split(cfg.SmtpTo, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: true}

	var conn net.Conn
	var err error

	if cfg.SmtpPort == 587 {
		// STARTTLS: 先明文连接，再升级到 TLS
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
		if err != nil {
			Logger.Warn("SMTP dial failed: ", err)
			return
		}
		client, err := smtp.NewClient(conn, cfg.SmtpHost)
		if err != nil {
			conn.Close()
			Logger.Warn("SMTP new client failed: ", err)
			return
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			client.Close()
			Logger.Warn("SMTP STARTTLS failed: ", err)
			return
		}
		defer client.Close()
		if err = client.Auth(auth); err != nil {
			Logger.Warn("SMTP auth failed: ", err)
			return
		}
		if err = client.Mail(cfg.SmtpUser); err != nil {
			Logger.Warn("SMTP mail from failed: ", err)
			return
		}
		for _, to := range recipients {
			if err = client.Rcpt(to); err != nil {
				Logger.Warn("SMTP Rcpt failed for ", to, ": ", err)
			}
		}
		w, err := client.Data()
		if err != nil {
			Logger.Warn("SMTP data failed: ", err)
			return
		}
		io.Copy(w, strings.NewReader(msg))
		w.Close()
	} else {
		// 默认 465 SMTPS: 直接 TLS 连接
		conn, err = tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			Logger.Warn("SMTP TLS dial failed: ", err)
			return
		}
		client, err := smtp.NewClient(conn, cfg.SmtpHost)
		if err != nil {
			Logger.Warn("SMTP new client failed: ", err)
			return
		}
		defer client.Close()
		if err = client.Auth(auth); err != nil {
			Logger.Warn("SMTP auth failed: ", err)
			return
		}
		if err = client.Mail(cfg.SmtpUser); err != nil {
			Logger.Warn("SMTP mail from failed: ", err)
			return
		}
		for _, to := range recipients {
			if err = client.Rcpt(to); err != nil {
				Logger.Warn("SMTP Rcpt failed for ", to, ": ", err)
			}
		}
		w, err := client.Data()
		if err != nil {
			Logger.Warn("SMTP data failed: ", err)
			return
		}
		io.Copy(w, strings.NewReader(msg))
		w.Close()
	}
}

func (s *NotifyService) postJson(url, body string) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		Logger.Warn("Notify post failed: ", err)
		return
	}
	defer resp.Body.Close()
}
