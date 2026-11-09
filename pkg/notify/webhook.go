package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WebhookType Webhook 平台类型
type WebhookType string

const (
	WebhookTypeGeneric  WebhookType = "generic"
	WebhookTypeFeishu   WebhookType = "feishu"
	WebhookTypeDingTalk WebhookType = "dingtalk"
	WebhookTypeTelegram WebhookType = "telegram"
)

// DetectWebhookType 根据 URL 格式智能识别平台类型
func DetectWebhookType(rawURL string) WebhookType {
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "feishu.cn") || strings.Contains(lower, "larksuite.com") {
		return WebhookTypeFeishu
	}
	if strings.Contains(lower, "dingtalk.com") {
		return WebhookTypeDingTalk
	}
	if strings.Contains(lower, "api.telegram.org") {
		return WebhookTypeTelegram
	}
	return WebhookTypeGeneric
}

// WebhookNotifier 基于 HTTP POST 的 Webhook 发送器
type WebhookNotifier struct {
	url        string
	hookType   WebhookType
	httpClient *http.Client
}

// NewWebhookNotifier 创建 Webhook 发送器
func NewWebhookNotifier(url string, hookType WebhookType) *WebhookNotifier {
	return &WebhookNotifier{
		url:      url,
		hookType: hookType,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Send 发送 Webhook 消息
func (w *WebhookNotifier) Send(ctx context.Context, title, body string) error {
	var payload any

	switch w.hookType {
	case WebhookTypeFeishu:
		payload = map[string]any{
			"msg_type": "text",
			"content": map[string]string{
				"text": fmt.Sprintf("【%s】\n\n%s", title, body),
			},
		}
	case WebhookTypeDingTalk:
		payload = map[string]any{
			"msgtype": "text",
			"text": map[string]string{
				"content": fmt.Sprintf("%s\n\n%s", title, body),
			},
		}
	default:
		// 通用或 Slack
		payload = map[string]string{
			"text": fmt.Sprintf("*%s*\n\n%s", title, body),
		}
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 webhook 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建 webhook 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送 webhook 请求失败 (%s): %w", w.hookType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook 服务器返回非成功状态码: %d", resp.StatusCode)
	}

	return nil
}

// TelegramNotifier 专有 Telegram 发送器
type TelegramNotifier struct {
	apiURL     string
	chatID     string
	httpClient *http.Client
}

// NewTelegramNotifier 创建 Telegram 发送器
func NewTelegramNotifier(apiURL, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		apiURL: apiURL,
		chatID: chatID,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Send 向 Telegram 频道或对话发送消息
func (t *TelegramNotifier) Send(ctx context.Context, title, body string) error {
	text := fmt.Sprintf("*%s*\n\n%s", title, body)
	payload := map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.apiURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("发送 telegram 消息失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram api 返回状态码: %d", resp.StatusCode)
	}
	return nil
}
