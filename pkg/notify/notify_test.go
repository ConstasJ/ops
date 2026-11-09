package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookNotifier_Feishu(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code": 0, "msg": "ok"}`))
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL, WebhookTypeFeishu)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := n.Send(ctx, "测试标题", "测试内容")
	require.NoError(t, err)
	assert.Equal(t, "text", receivedBody["msg_type"])
	content := receivedBody["content"].(map[string]any)
	assert.Contains(t, content["text"], "测试标题")
	assert.Contains(t, content["text"], "测试内容")
}

func TestWebhookNotifier_DingTalk(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode": 0, "errmsg": "ok"}`))
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL, WebhookTypeDingTalk)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := n.Send(ctx, "钉钉告警", "集群升级完毕")
	require.NoError(t, err)
	assert.Equal(t, "text", receivedBody["msgtype"])
	textObj := receivedBody["text"].(map[string]any)
	assert.Contains(t, textObj["content"], "钉钉告警")
}

func TestWebhookNotifier_Generic(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewWebhookNotifier(server.URL, WebhookTypeGeneric)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := n.Send(ctx, "通用通知", "Hello World")
	require.NoError(t, err)
	assert.Contains(t, receivedBody["text"], "通用通知")
}

func TestMultiNotifier(t *testing.T) {
	called1 := false
	called2 := false

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called1 = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called2 = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	multi := NewMultiNotifier(
		NewWebhookNotifier(server1.URL, WebhookTypeGeneric),
		NewWebhookNotifier(server2.URL, WebhookTypeGeneric),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := multi.Send(ctx, "广播", "群发消息")
	require.NoError(t, err)
	assert.True(t, called1)
	assert.True(t, called2)
}
