package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
			if err := n.Send(ctx, title, body); err != nil {
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

// NewFromEnv 自动读取当前运行环境中的 Webhook 和 SMTP 配置构建 Notifier
func NewFromEnv() Notifier {
	var list []Notifier

	// 1. 飞书 Webhook
	if url := os.Getenv("OPS_FEISHU_WEBHOOK"); url != "" {
		list = append(list, NewWebhookNotifier(url, WebhookTypeFeishu))
	}

	// 2. 钉钉 Webhook
	if url := os.Getenv("OPS_DINGTALK_WEBHOOK"); url != "" {
		list = append(list, NewWebhookNotifier(url, WebhookTypeDingTalk))
	}

	// 3. 通用/Slack Webhook
	if url := os.Getenv("OPS_WEBHOOK_URL"); url != "" {
		typ := DetectWebhookType(url)
		list = append(list, NewWebhookNotifier(url, typ))
	}

	// 4. Telegram Bot
	botToken := os.Getenv("OPS_TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("OPS_TELEGRAM_CHAT_ID")
	if botToken != "" && chatID != "" {
		tgURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
		list = append(list, NewTelegramNotifier(tgURL, chatID))
	}

	// 5. SMTP 邮件 (兼容 /etc/restic/smtp.env)
	if smtpHost := os.Getenv("SMTP_HOST"); smtpHost != "" {
		smtpUser := os.Getenv("SMTP_USER")
		smtpPass := os.Getenv("SMTP_PASS")
		smtpFrom := os.Getenv("SMTP_FROM")
		smtpTo := os.Getenv("SMTP_TO")
		smtpPort := os.Getenv("SMTP_PORT")
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
