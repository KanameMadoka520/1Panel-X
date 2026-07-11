package webhook_alert

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Platform string

const (
	PlatformWeCom    Platform = "weCom"
	PlatformDingTalk Platform = "dingTalk"
	PlatformFeiShu   Platform = "feiShu"

	requestTimeout  = 10 * time.Second
	maxResponseBody = 64 * 1024
)

var allowedHosts = map[Platform]map[string]struct{}{
	PlatformWeCom: {
		"qyapi.weixin.qq.com": {},
	},
	PlatformDingTalk: {
		"oapi.dingtalk.com": {},
	},
	PlatformFeiShu: {
		"open.feishu.cn":     {},
		"open.larksuite.com": {},
	},
}

func Send(ctx context.Context, platform Platform, webhookURL, content string, transport *http.Transport) error {
	validatedURL, err := validateURL(platform, webhookURL)
	if err != nil {
		return err
	}

	payload, err := buildPayload(platform, content)
	if err != nil {
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, validatedURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "1Panel-Webhook-Alert")

	secureTransport := cloneSecureTransport(transport)
	defer secureTransport.CloseIdleConnections()
	client := &http.Client{
		Transport: secureTransport,
		Timeout:   requestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Err != nil {
			return fmt.Errorf("send %s webhook: %w", platform, urlErr.Err)
		}
		return fmt.Errorf("send %s webhook: %w", platform, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if err != nil {
		return fmt.Errorf("read %s webhook response: %w", platform, err)
	}
	if len(body) > maxResponseBody {
		return fmt.Errorf("%s webhook response exceeds %d bytes", platform, maxResponseBody)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s webhook returned HTTP status %d", platform, resp.StatusCode)
	}

	if err := validatePlatformResponse(platform, body); err != nil {
		return err
	}
	return nil
}

func ValidateURL(platform Platform, webhookURL string) error {
	_, err := validateURL(platform, webhookURL)
	return err
}

func validateURL(platform Platform, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("webhook URL is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse webhook URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("webhook URL scheme must be https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("webhook URL host is required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("webhook URL must not contain user information")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return "", fmt.Errorf("webhook URL port must be 443")
	}

	host := strings.ToLower(parsed.Hostname())
	if net.ParseIP(host) != nil {
		return "", fmt.Errorf("webhook URL IP addresses are not allowed")
	}
	platformHosts, ok := allowedHosts[platform]
	if !ok {
		return "", fmt.Errorf("unsupported webhook platform %q", platform)
	}
	if _, ok := platformHosts[host]; !ok {
		return "", fmt.Errorf("webhook URL host %q is not allowed for %s", host, platform)
	}

	return parsed.String(), nil
}

func cloneSecureTransport(transport *http.Transport) *http.Transport {
	base := transport
	if base == nil {
		base = http.DefaultTransport.(*http.Transport)
	}

	cloned := base.Clone()
	tlsConfig := &tls.Config{}
	if cloned.TLSClientConfig != nil {
		tlsConfig = cloned.TLSClientConfig.Clone()
	}
	tlsConfig.InsecureSkipVerify = false
	if tlsConfig.MinVersion < tls.VersionTLS12 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}
	// The request host must determine SNI and certificate hostname validation.
	tlsConfig.ServerName = ""
	cloned.TLSClientConfig = tlsConfig
	cloned.DialTLS = nil
	cloned.DialTLSContext = nil
	return cloned
}

func buildPayload(platform Platform, content string) ([]byte, error) {
	var payload any
	switch platform {
	case PlatformWeCom, PlatformDingTalk:
		payload = struct {
			MsgType string `json:"msgtype"`
			Text    struct {
				Content string `json:"content"`
			} `json:"text"`
		}{
			MsgType: "text",
			Text: struct {
				Content string `json:"content"`
			}{Content: content},
		}
	case PlatformFeiShu:
		payload = struct {
			MsgType string `json:"msg_type"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		}{
			MsgType: "text",
			Content: struct {
				Text string `json:"text"`
			}{Text: content},
		}
	default:
		return nil, fmt.Errorf("unsupported webhook platform %q", platform)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s webhook payload: %w", platform, err)
	}
	return data, nil
}

func validatePlatformResponse(platform Platform, body []byte) error {
	switch platform {
	case PlatformWeCom, PlatformDingTalk:
		var result struct {
			ErrCode *int `json:"errcode"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("decode %s webhook response: %w", platform, err)
		}
		if result.ErrCode == nil {
			return fmt.Errorf("%s webhook response is missing errcode", platform)
		}
		if *result.ErrCode != 0 {
			return fmt.Errorf("%s webhook returned errcode %d", platform, *result.ErrCode)
		}
		return nil
	case PlatformFeiShu:
		var result struct {
			Code       *int `json:"code"`
			StatusCode *int `json:"StatusCode"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return fmt.Errorf("decode %s webhook response: %w", platform, err)
		}
		if result.Code != nil {
			if *result.Code != 0 {
				return fmt.Errorf("%s webhook returned code %d", platform, *result.Code)
			}
			return nil
		}
		if result.StatusCode != nil {
			if *result.StatusCode != 0 {
				return fmt.Errorf("%s webhook returned StatusCode %d", platform, *result.StatusCode)
			}
			return nil
		}
		return fmt.Errorf("%s webhook response is missing code", platform)
	default:
		return fmt.Errorf("unsupported webhook platform %q", platform)
	}
}
