package logs

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"
)

//============================== WriteHTTP ==============================

// HTTPOption HTTP 客户端配置选项
type HTTPOption func(*httpClient)

// WithInsecureSkipVerify 跳过 TLS 证书验证（仅用于测试环境）
// 注意：若已通过 WithHTTPClient 设置非 *http.Transport 的 Transport，将 panic
func WithInsecureSkipVerify(b bool) HTTPOption {
	return func(h *httpClient) {
		transport, ok := h.Client.Transport.(*http.Transport)
		if !ok {
			panic("logs: WithInsecureSkipVerify requires *http.Transport, got " +
				fmt.Sprintf("%T", h.Client.Transport))
		}
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: b,
		}
	}
}

// WithTimeout 设置 HTTP 请求超时
func WithTimeout(d time.Duration) HTTPOption {
	return func(h *httpClient) {
		h.Client.Timeout = d
	}
}

// WithHTTPClient 设置自定义 http.Client
// 注意：若 Transport 不是 *http.Transport，后续 WithInsecureSkipVerify 会 panic
func WithHTTPClient(c *http.Client) HTTPOption {
	return func(h *httpClient) {
		h.Client = c
	}
}

// NewHTTPClient 创建 HTTP 日志输出，异步发送
// method HTTP 方法（POST/PUT 等）
// url 目标 URL
// opts 配置选项（默认启用 TLS 证书验证）
func NewHTTPClient(method, url string, opts ...HTTPOption) io.Writer {
	h := &httpClient{
		Client: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives: true,
				// 默认安全：不跳过 TLS 验证
			},
			Timeout: 10 * time.Second,
		},
		method: method,
		url:    url,
	}
	for _, opt := range opts {
		opt(h)
	}
	// handler 在 newChan 启动 goroutine 前传入，避免时序竞态
	h.Chan = newChan(100, func(ctx context.Context, bs []byte) {
		req, err := http.NewRequestWithContext(ctx, h.method, h.url, bytes.NewBuffer(bs))
		if err != nil {
			return
		}
		resp, err := h.Client.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
	})
	return h
}

// httpClient HTTP 日志客户端
type httpClient struct {
	*http.Client
	method string
	url    string
	*Chan
}

func (this *httpClient) Write(p []byte) (int, error) {
	return this.Chan.Write(p)
}

// Close 关闭 HTTP 客户端
func (this *httpClient) Close() error {
	this.Chan.Close()
	return nil
}
