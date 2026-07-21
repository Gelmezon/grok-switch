package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Result is the outcome of a provider/model connectivity test.
type Result struct {
	OK         bool
	Latency    time.Duration
	StatusCode int
	Message    string
	Endpoint   string
}

// TestModel sends a minimal chat completion request to verify the mid-station
// accepts the API key and model. Timeout defaults to 20s when zero.
func TestModel(baseURL, apiKey, model string, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if baseURL == "" || apiKey == "" || model == "" {
		return Result{OK: false, Message: "Base URL、API Key 与模型均不能为空"}
	}

	endpoint := chatCompletionsURL(baseURL)
	body := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	}
	raw, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return Result{OK: false, Message: "构造请求失败: " + err.Error(), Endpoint: endpoint}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	lat := time.Since(start)
	if err != nil {
		return Result{
			OK:       false,
			Latency:  lat,
			Message:  "网络错误: " + err.Error(),
			Endpoint: endpoint,
		}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	r := Result{
		Latency:    lat,
		StatusCode: resp.StatusCode,
		Endpoint:   endpoint,
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		r.OK = true
		r.Message = fmt.Sprintf("模型 %s 可用", model)
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		r.Message = "认证失败（API Key 无效或无权限）"
	case resp.StatusCode == 404:
		// Retry models list as secondary check
		if mr := testModelsList(ctx, baseURL, apiKey, model); mr.OK {
			return mr
		}
		r.Message = "接口不存在（404）。请确认 Base URL 是否包含 /v1"
	case resp.StatusCode == 400 || resp.StatusCode == 422:
		// Often means auth ok but model name wrong
		msg := shortBody(respBody)
		if strings.Contains(strings.ToLower(msg), "model") {
			r.Message = "请求被拒绝，可能模型名不可用: " + msg
		} else {
			r.Message = "请求被拒绝 (HTTP " + fmt.Sprint(resp.StatusCode) + "): " + msg
		}
	default:
		r.Message = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, shortBody(respBody))
	}
	return r
}

func testModelsList(ctx context.Context, baseURL, apiKey, model string) Result {
	endpoint := modelsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Result{OK: false, Endpoint: endpoint, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	lat := time.Since(start)
	if err != nil {
		return Result{OK: false, Latency: lat, Endpoint: endpoint, Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	r := Result{Latency: lat, StatusCode: resp.StatusCode, Endpoint: endpoint}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		r.Message = fmt.Sprintf("models 列表 HTTP %d", resp.StatusCode)
		return r
	}
	// Best-effort: look for model id in JSON text
	if model != "" && !bytes.Contains(raw, []byte(model)) {
		r.OK = true // auth works
		r.Message = fmt.Sprintf("API 可达，但 models 列表中未找到 %s（仍可能可用）", model)
		return r
	}
	r.OK = true
	r.Message = fmt.Sprintf("API 可达，模型 %s 出现在列表中", model)
	return r
}

func chatCompletionsURL(base string) string {
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	if strings.Contains(base, "/v1/") {
		return strings.TrimRight(base, "/") + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}

func modelsURL(base string) string {
	if strings.HasSuffix(base, "/v1") {
		return base + "/models"
	}
	return base + "/v1/models"
}

func shortBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	if s == "" {
		return "(空响应)"
	}
	return s
}
