package notify

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPConfig SMTP 邮件连接与认证配置
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	To       []string
}

// SMTPNotifier SMTP 邮件通知器
type SMTPNotifier struct {
	cfg SMTPConfig
}

// NewSMTPNotifier 创建 SMTP 通知器
func NewSMTPNotifier(cfg SMTPConfig) *SMTPNotifier {
	return &SMTPNotifier{cfg: cfg}
}

// Send 发送邮件通知
func (s *SMTPNotifier) Send(ctx context.Context, title, body string) error {
	addr := fmt.Sprintf("%s:%s", s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.Username != "" && s.cfg.Password != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	header := make(map[string]string)
	header["From"] = s.cfg.From
	header["To"] = strings.Join(s.cfg.To, ";")
	header["Subject"] = title
	header["Content-Type"] = "text/plain; charset=UTF-8"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	done := make(chan error, 1)
	go func() {
		err := smtp.SendMail(addr, auth, s.cfg.From, s.cfg.To, []byte(message))
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("SMTP 发送邮件失败: %w", err)
		}
		return nil
	}
}
