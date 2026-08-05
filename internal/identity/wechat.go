package identityauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const wechatCodeExchangeURL = "https://api.weixin.qq.com/sns/jscode2session"

var ErrWechatNotConfigured = errors.New("wechat mini program credentials are not configured")

type WechatSession struct {
	OpenID  string `json:"openid"`
	UnionID string `json:"unionid"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

type WechatExchanger struct {
	AppID    string
	Secret   string
	Endpoint string
	Client   *http.Client
}

func (e WechatExchanger) Exchange(ctx context.Context, code string) (WechatSession, error) {
	if strings.TrimSpace(e.AppID) == "" || strings.TrimSpace(e.Secret) == "" {
		return WechatSession{}, ErrWechatNotConfigured
	}
	endpoint := e.Endpoint
	if endpoint == "" {
		endpoint = wechatCodeExchangeURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return WechatSession{}, fmt.Errorf("parse wechat endpoint: %w", err)
	}
	query := parsed.Query()
	query.Set("appid", e.AppID)
	query.Set("secret", e.Secret)
	query.Set("js_code", strings.TrimSpace(code))
	query.Set("grant_type", "authorization_code")
	parsed.RawQuery = query.Encode()
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return WechatSession{}, err
	}
	response, err := client.Do(req)
	if err != nil {
		return WechatSession{}, fmt.Errorf("wechat code exchange request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return WechatSession{}, fmt.Errorf("wechat code exchange returned status %d", response.StatusCode)
	}
	var session WechatSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		return WechatSession{}, fmt.Errorf("decode wechat code exchange response: %w", err)
	}
	if session.ErrCode != 0 || session.OpenID == "" {
		return WechatSession{}, fmt.Errorf("wechat code exchange rejected: %s", session.ErrMsg)
	}
	return session, nil
}
