package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Notifier 消息发送器接口
type Notifier interface {
	Send(ctx context.Context, title, body string) error
}

// MultiNotifier 组合多个通知器并发广播
type MultiNotifier struct {
	notifiers []Notifier
}

// NewMultiNotifier 创建聚合通知器
func NewMultiNotifier(notifiers ...Notifier) *MultiNotifier {
	var valid []Notifier
	for _, n := range notifiers {
		if n != nil {
			valid = append(valid, n)
		}
	}
	return &MultiNotifier{notifiers: valid}
}

// Send 向所有已注册的渠道并发发送通知，汇总错误
func (m *MultiNotifier) Send(ctx context.Context, title, body string) error {
	if len(m.notifiers) == 0 {
		return nil
	}

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for _, n := range m.notifiers {
		n := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendCtx, sendCancel := context.WithTimeout(ctx, 10*time.Second)
			defer sendCancel()
			if err := n.Send(sendCtx, title, body); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// NotifyOptions 允许外部传入已解析的配置文件中的通知选项
type NotifyOptions struct {
	FeishuWebhook   string
	DingTalkWebhook string
	WebhookURL      string
	TelegramToken   string
	TelegramChatID  string
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	SMTPFrom        string
	SMTPTo          []string
}

// NewFromEnvOrOptions 按照 环境变量 > 配置选项 的优先级装配通知器
func NewFromEnvOrOptions(opts *NotifyOptions) Notifier {
	var list []Notifier

	// 1. 飞书 Webhook
	feishuURL := os.Getenv("OPS_FEISHU_WEBHOOK")
	if feishuURL == "" && opts != nil {
		feishuURL = opts.FeishuWebhook
	}
	if feishuURL != "" {
		list = append(list, NewWebhookNotifier(feishuURL, WebhookTypeFeishu))
	}

	// 2. 钉钉 Webhook
	dingURL := os.Getenv("OPS_DINGTALK_WEBHOOK")
	if dingURL == "" && opts != nil {
		dingURL = opts.DingTalkWebhook
	}
	if dingURL != "" {
		list = append(list, NewWebhookNotifier(dingURL, WebhookTypeDingTalk))
	}

	// 3. 通用/Slack Webhook
	genericURL := os.Getenv("OPS_WEBHOOK_URL")
	if genericURL == "" && opts != nil {
		genericURL = opts.WebhookURL
	}
	if genericURL != "" {
		typ := DetectWebhookType(genericURL)
		list = append(list, NewWebhookNotifier(genericURL, typ))
	}

	// 4. Telegram Bot
	tgToken := os.Getenv("OPS_TELEGRAM_BOT_TOKEN")
	tgChatID := os.Getenv("OPS_TELEGRAM_CHAT_ID")
	if (tgToken == "" || tgChatID == "") && opts != nil {
		if tgToken == "" {
			tgToken = opts.TelegramToken
		}
		if tgChatID == "" {
			tgChatID = opts.TelegramChatID
		}
	}
	if tgToken != "" && tgChatID != "" {
		tgURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tgToken)
		list = append(list, NewTelegramNotifier(tgURL, tgChatID))
	}

	// 5. SMTP 邮件 (兼容 /etc/restic/smtp.env 与 opts)
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	smtpFrom := os.Getenv("SMTP_FROM")
	smtpTo := os.Getenv("SMTP_TO")

	if smtpHost == "" && opts != nil {
		smtpHost = opts.SMTPHost
		smtpPort = opts.SMTPPort
		smtpUser = opts.SMTPUser
		smtpPass = opts.SMTPPass
		smtpFrom = opts.SMTPFrom
		if len(opts.SMTPTo) > 0 {
			smtpTo = strings.Join(opts.SMTPTo, ",")
		}
	}

	if smtpHost != "" {
		if smtpPort == "" {
			smtpPort = "587"
		}
		if smtpFrom == "" {
			smtpFrom = smtpUser
		}
		if smtpTo != "" {
			toList := strings.Split(smtpTo, ",")
			list = append(list, NewSMTPNotifier(SMTPConfig{
				Host:     smtpHost,
				Port:     smtpPort,
				Username: smtpUser,
				Password: smtpPass,
				From:     smtpFrom,
				To:       toList,
			}))
		}
	}

	if len(list) == 0 {
		return nil
	}
	return NewMultiNotifier(list...)
}

// NewFromEnv 仅从环境变量与 .env 读取配置构建 Notifier
func NewFromEnv() Notifier {
	return NewFromEnvOrOptions(nil)
}
