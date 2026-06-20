// Package payment abstracts payment provider interactions for balance recharges.
package payment

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Provider handles creation and verification of payment requests.
type Provider interface {
	// Name returns provider key, e.g. "mock" / "epay".
	Name() string
	// CreateRecharge builds a payment request for given outTradeNo and amountCents.
	// Returns redirect URL and/or QR code content.
	CreateRecharge(ctx context.Context, outTradeNo, userID string, amountCents int64, method, description string) (*CreateResult, error)
	// VerifyCallback validates a provider notify/callback request and extracts trade result.
	VerifyCallback(r *http.Request) (*CallbackResult, error)
	// SupportsMethod returns whether the provider supports a payment method (e.g. "alipay" / "wxpay").
	SupportsMethod(method string) bool
}

// CreateResult is the unified payment creation result.
type CreateResult struct {
	PayURL   string `json:"pay_url"`
	QRCode   string `json:"qr_code"`
	FormHTML string `json:"form_html"`
}

// CallbackResult is the unified payment callback result.
type CallbackResult struct {
	OutTradeNo  string
	TradeNo     string
	Status      string // paid | pending | closed
	AmountCents int64
	PaidAt      time.Time
	RawBody     string
}

// Config holds per-provider credentials and settings.
type Config struct {
	Enabled          bool   `json:"enabled"`
	Provider         string `json:"provider"`
	EPayURL          string `json:"epay_url"`
	EPayPID          string `json:"epay_pid"`
	EPayKey          string `json:"epay_key"`
	EPayNotifyURL    string `json:"epay_notify_url"`
	EPayReturnURL    string `json:"epay_return_url"`
	MinRechargeCents int64  `json:"min_recharge_cents"`
}

// NewProvider builds a Provider from Config.
func NewProvider(cfg Config) Provider {
	switch cfg.Provider {
	case "epay":
		return &epayProvider{cfg: cfg}
	default:
		return &mockProvider{cfg: cfg}
	}
}

// mockProvider is a no-op provider useful for development and testing.
type mockProvider struct {
	cfg Config
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) CreateRecharge(ctx context.Context, outTradeNo, userID string, amountCents int64, method, description string) (*CreateResult, error) {
	_ = ctx
	_ = userID
	_ = method
	_ = description
	return &CreateResult{
		PayURL: fmt.Sprintf("/api/payments/mock/%s?amount=%d", outTradeNo, amountCents),
		QRCode: "",
	}, nil
}

func (m *mockProvider) VerifyCallback(r *http.Request) (*CallbackResult, error) {
	_ = r.ParseForm()
	outTradeNo := r.FormValue("out_trade_no")
	if outTradeNo == "" {
		return nil, fmt.Errorf("missing out_trade_no")
	}
	amountStr := r.FormValue("amount_cents")
	var amountCents int64
	fmt.Sscanf(amountStr, "%d", &amountCents)
	if amountCents <= 0 {
		return nil, fmt.Errorf("missing or invalid amount_cents")
	}
	return &CallbackResult{
		OutTradeNo:  outTradeNo,
		TradeNo:     "mock-" + outTradeNo,
		Status:      "paid",
		AmountCents: amountCents,
		PaidAt:      time.Now(),
		RawBody:     "",
	}, nil
}

func (m *mockProvider) SupportsMethod(method string) bool {
	return method == "mock" || method == "alipay" || method == "wxpay" || method == "qqpay"
}

// epayProvider implements a common Chinese easy-payment (易支付) gateway.
type epayProvider struct {
	cfg Config
}

func (e *epayProvider) Name() string { return "epay" }

func (e *epayProvider) SupportsMethod(method string) bool {
	return method == "alipay" || method == "wxpay" || method == "qqpay"
}

func (e *epayProvider) CreateRecharge(ctx context.Context, outTradeNo, userID string, amountCents int64, method, description string) (*CreateResult, error) {
	_ = ctx
	method = strings.TrimSpace(method)
	if !e.SupportsMethod(method) {
		method = "alipay"
	}
	amountYuan := fmt.Sprintf("%.2f", float64(amountCents)/100.0)
	params := url.Values{}
	params.Set("pid", e.cfg.EPayPID)
	params.Set("type", method)
	params.Set("out_trade_no", outTradeNo)
	params.Set("notify_url", e.cfg.EPayNotifyURL)
	params.Set("return_url", e.cfg.EPayReturnURL)
	params.Set("name", description)
	params.Set("money", amountYuan)
	params.Set("param", userID)
	params.Set("sign", e.sign(params))
	params.Set("sign_type", "MD5")

	payURL := strings.TrimRight(e.cfg.EPayURL, "/") + "/submit.php?" + params.Encode()
	return &CreateResult{PayURL: payURL}, nil
}

func (e *epayProvider) VerifyCallback(r *http.Request) (*CallbackResult, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	params := make(map[string]string)
	for k, v := range r.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	remoteSign := params["sign"]
	delete(params, "sign")
	delete(params, "sign_type")

	if e.signMap(params) != remoteSign {
		body, _ := io.ReadAll(r.Body)
		log.Warn().Str("body", string(body)).Msg("epay callback sign mismatch")
		return nil, fmt.Errorf("sign mismatch")
	}

	status := "pending"
	if params["trade_status"] == "TRADE_SUCCESS" {
		status = "paid"
	}
	var amountCents int64
	if money, err := strconv.ParseFloat(params["money"], 64); err == nil {
		amountCents = int64(money*100 + 0.5)
	}

	paidAt := time.Now()
	if t, err := time.Parse("2006-01-02 15:04:05", params["endtime"]); err == nil {
		paidAt = t
	}

	return &CallbackResult{
		OutTradeNo:  params["out_trade_no"],
		TradeNo:     params["trade_no"],
		Status:      status,
		AmountCents: amountCents,
		PaidAt:      paidAt,
		RawBody:     r.Form.Encode(),
	}, nil
}

func (e *epayProvider) sign(values url.Values) string {
	m := make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	return e.signMap(m)
}

func (e *epayProvider) signMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		if k != "" && m[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(m[k])
	}
	sb.WriteString(e.cfg.EPayKey)
	sum := md5.Sum([]byte(sb.String()))
	return fmt.Sprintf("%x", sum)
}
