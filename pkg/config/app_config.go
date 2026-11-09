package config

import (
	"ops-go/pkg/domain"
)

// TelegramConfig Telegram 通知配置
type TelegramConfig struct {
	BotToken string `json:"bot_token" yaml:"bot_token" toml:"bot_token"`
	ChatID   string `json:"chat_id" yaml:"chat_id" toml:"chat_id"`
}

// SMTPConfig SMTP 邮件通知配置
type SMTPConfig struct {
	Host     string   `json:"host" yaml:"host" toml:"host"`
	Port     string   `json:"port" yaml:"port" toml:"port"`
	Username string   `json:"user" yaml:"user" toml:"user"`
	Password string   `json:"pass" yaml:"pass" toml:"pass"`
	From     string   `json:"from" yaml:"from" toml:"from"`
	To       []string `json:"to" yaml:"to" toml:"to"`
}

// NotifyConfig 多渠道通知综合配置
type NotifyConfig struct {
	FeishuWebhook   string         `json:"feishu_webhook" yaml:"feishu_webhook" toml:"feishu_webhook"`
	DingTalkWebhook string         `json:"dingtalk_webhook" yaml:"dingtalk_webhook" toml:"dingtalk_webhook"`
	WebhookURL      string         `json:"webhook_url" yaml:"webhook_url" toml:"webhook_url"`
	Telegram        TelegramConfig `json:"telegram" yaml:"telegram" toml:"telegram"`
	SMTP            SMTPConfig     `json:"smtp" yaml:"smtp" toml:"smtp"`
}

// AppConfig 全局应用程序配置结构体
type AppConfig struct {
	Socket     string                          `json:"socket" yaml:"socket" toml:"socket"`
	ConfigPath string                          `json:"config" yaml:"config" toml:"config"`
	State      string                          `json:"state" yaml:"state" toml:"state"`
	ReportDir  string                          `json:"report_dir" yaml:"report_dir" toml:"report_dir"`
	BackupDir  string                          `json:"backup_dir" yaml:"backup_dir" toml:"backup_dir"`
	Notify     NotifyConfig                    `json:"notify" yaml:"notify" toml:"notify"`
	Containers map[string]domain.ContainerSpec `json:"containers" yaml:"containers" toml:"containers"`
}

// NewAppConfig 初始化包含默认空容器映射的 AppConfig
func NewAppConfig() *AppConfig {
	return &AppConfig{
		Containers: make(map[string]domain.ContainerSpec),
	}
}
