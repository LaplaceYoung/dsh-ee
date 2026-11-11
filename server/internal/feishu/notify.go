package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// SendMessage 用 tenant token 给一名飞书员工发消息。content 按飞书要求作为 JSON 字符串传递。
func (c *Client) SendMessage(ctx context.Context, openID, messageType string, content any) (string, error) {
	if openID == "" {
		return "", errors.New("飞书通知缺少员工 open_id")
	}
	token, err := c.tenantToken(ctx)
	if err != nil {
		return "", err
	}
	var text string
	switch v := content.(type) {
	case string:
		text = v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		text = string(raw)
	}
	body := map[string]any{"receive_id": openID, "msg_type": messageType, "content": text}
	var out struct {
		MessageID string `json:"message_id"`
	}
	q := url.Values{"receive_id_type": {"open_id"}}
	if err := c.call(ctx, http.MethodPost, "/open-apis/im/v1/messages?"+q.Encode(), body, &out, token); err != nil {
		return "", err
	}
	if out.MessageID == "" {
		return "", errors.New("飞书没有返回通知消息 id")
	}
	return out.MessageID, nil
}

// SendText 发送普通文本结果通知。
func (c *Client) SendText(ctx context.Context, openID, markdown string) (string, error) {
	return c.SendMessage(ctx, openID, "text", map[string]string{"text": markdown})
}

// SendInteractive 发送带按钮的交互卡片，卡片 body 由调用方的审批状态机生成。
func (c *Client) SendInteractive(ctx context.Context, openID string, card any) (string, error) {
	return c.SendMessage(ctx, openID, "interactive", card)
}
