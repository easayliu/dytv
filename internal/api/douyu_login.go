package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	douyuGenerateCodeURL = "https://passport.douyu.com/scan/generateCode"
	douyuQRCheckURL      = "https://passport.douyu.com/lapi/passport/qrcode/check"
	douyuLoginReferer    = "https://passport.douyu.com/index/login?passport_reg_callback=PASSPORT_REG_SUCCESS_CALLBACK&passport_login_callback=PASSPORT_LOGIN_SUCCESS_CALLBACK&passport_close_callback=PASSPORT_CLOSE_CALLBACK&passport_dp_callback=PASSPORT_DP_CALLBACK&type=login&client_id=1&state=https%3A%2F%2Fwww.douyu.com%2F"
)

// QRCodeResult holds the result of QR code generation.
type QRCodeResult struct {
	Code   string `json:"code"`
	URL    string `json:"url"`
	Expire int    `json:"expire"`
}

// QRStatus holds the QR code polling status.
type QRStatus struct {
	Status  string `json:"status"` // waiting, scanned, success, expired
	URL     string `json:"url"`    // callback URL on success
	Message string `json:"message"`
}

// GenerateQRCode requests a new login QR code from Douyu passport.
func GenerateQRCode() (*QRCodeResult, error) {
	form := url.Values{"client_id": {"1"}}
	req, err := http.NewRequest("POST", douyuGenerateCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", douyuLoginReferer)

	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Error int `json:"error"`
		Data  struct {
			Code   string `json:"code"`
			URL    string `json:"url"`
			Expire int    `json:"expire"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if result.Error != 0 {
		return nil, fmt.Errorf("generate qrcode error: %s", result.Msg)
	}

	return &QRCodeResult{
		Code:   result.Data.Code,
		URL:    result.Data.URL,
		Expire: result.Data.Expire,
	}, nil
}

// CheckQRStatus polls the QR code scan status.
func CheckQRStatus(code string) (*QRStatus, error) {
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	u := fmt.Sprintf("%s?time=%s&code=%s", douyuQRCheckURL, ts, url.QueryEscape(code))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", douyuLoginReferer)

	client := &http.Client{Timeout: DefaultTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	log.Printf("[login] qrcode check response: %s", body)

	// 顶层 error 字段决定状态：-2=等待扫码, -1=已过期, 0=成功
	// data 字段类型不固定：失败时为字符串，成功时为含 url 的对象
	var result struct {
		Error int             `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	switch result.Error {
	case 0:
		// 成功，data 是对象，提取回调 URL
		var data struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			return nil, fmt.Errorf("parse success data: %w", err)
		}
		log.Printf("[login] login success, callback url=%q", data.URL)
		return &QRStatus{Status: "success", URL: data.URL, Message: "登录成功"}, nil
	case -2:
		return &QRStatus{Status: "waiting", Message: "等待扫码"}, nil
	case -1:
		return &QRStatus{Status: "expired", Message: "二维码已过期"}, nil
	case -3:
		return &QRStatus{Status: "scanned", Message: "已扫码，请在手机上确认"}, nil
	default:
		// 未知状态，用 data 字符串作为消息
		var msg string
		_ = json.Unmarshal(result.Data, &msg)
		log.Printf("[login] unknown status error=%d data=%q", result.Error, msg)
		return &QRStatus{Status: "waiting", Message: msg}, nil
	}
}

// DoLoginCallback follows the login callback URL and extracts cookies.
func DoLoginCallback(callbackURL string) (string, error) {
	// Ensure https scheme
	if strings.HasPrefix(callbackURL, "//") {
		callbackURL = "https:" + callbackURL
	}

	client := &http.Client{
		Timeout: DefaultTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest("GET", callbackURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("callback request failed: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[login] callback status=%d, Set-Cookie count=%d", resp.StatusCode, len(resp.Cookies()))

	// Collect auth cookies from Set-Cookie headers
	var cookies []string
	for _, c := range resp.Cookies() {
		log.Printf("[login] cookie: %s=%s...", c.Name, truncate(c.Value, 8))
		if c.Value != "" && c.Value != "deleted" {
			cookies = append(cookies, c.Name+"="+c.Value)
		}
	}

	if len(cookies) == 0 {
		return "", fmt.Errorf("no cookies received from callback")
	}

	log.Printf("[login] saved %d cookies", len(cookies))
	return strings.Join(cookies, "; "), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
