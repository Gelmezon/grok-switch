package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Gelmezon/grok-switch/internal/profiles"
)

// EndpointResult is the outcome of one concrete API endpoint request.
type EndpointResult struct {
	OK         bool          `json:"ok"`
	Latency    time.Duration `json:"latency"`
	StatusCode int           `json:"status_code"`
	Message    string        `json:"message"`
	Endpoint   string        `json:"endpoint"`
}

// Result separates authentication/API reachability from actual chat support.
// OK intentionally mirrors ChatCompletionOK: a relay is usable only when the
// chat completion request required by Grok succeeds.
type Result struct {
	OK               bool           `json:"ok"`
	AuthenticationOK bool           `json:"authentication_ok"`
	ModelsEndpointOK bool           `json:"models_endpoint_ok"`
	ChatCompletionOK bool           `json:"chat_completion_ok"`
	Latency          time.Duration  `json:"latency"`
	Message          string         `json:"message"`
	Protocol         string         `json:"protocol"`
	Models           EndpointResult `json:"models"`
	Chat             EndpointResult `json:"chat"`
}

// TestProfile probes a profile using its declared upstream protocol. Only
// OpenAI Chat Completions is currently implemented; other formats fail
// explicitly instead of being tested with the wrong wire protocol.
func TestProfile(p profiles.Profile, timeout time.Duration) Result {
	protocol := strings.TrimSpace(p.UpstreamFormat)
	if protocol == "" {
		protocol = "openai_chat"
	}
	if protocol != "openai_chat" {
		return Result{
			Protocol: protocol,
			Message:  fmt.Sprintf("暂不支持 %s 格式的连通性测试；当前仅支持 OpenAI Chat Completions", protocol),
		}
	}
	return TestModel(p.BaseURL, p.APIKey, p.DefaultModel, timeout)
}

// DiscoverProfileModels fetches the relay's model inventory and applies it to
// a profile so config generation can emit one model section per available ID.
func DiscoverProfileModels(p profiles.Profile, timeout time.Duration) (profiles.Profile, error) {
	ids, err := FetchModels(p.BaseURL, p.APIKey, timeout)
	if err != nil {
		return p, fmt.Errorf("拉取模型列表失败: %w", err)
	}
	if err := profiles.ApplyDiscoveredModels(&p, ids); err != nil {
		return p, err
	}
	return p, nil
}

// FetchModels returns normalized model IDs from an OpenAI-compatible /models
// endpoint. It accepts the standard {"data":[{"id":"..."}]} response and
// common relay variants using "models" or a top-level array.
func FetchModels(baseURL, apiKey string, timeout time.Duration) ([]string, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	if baseURL == "" || apiKey == "" {
		return nil, fmt.Errorf("Base URL 与 API Key 均不能为空")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	endpoint := modelsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 models 响应失败: %w", err)
	}
	if !isHTTPSuccess(resp.StatusCode) {
		if isAuthenticationFailure(resp.StatusCode) {
			return nil, fmt.Errorf("认证失败（HTTP %d）", resp.StatusCode)
		}
		return nil, fmt.Errorf("Models 端点 HTTP %d: %s", resp.StatusCode, shortBody(body))
	}

	ids, err := parseModelIDs(body)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("Models 端点未返回可用模型")
	}
	return ids, nil
}

func parseModelIDs(body []byte) ([]string, error) {
	var entries []json.RawMessage
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("Models 端点返回空响应")
	}
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, fmt.Errorf("Models 响应不是有效 JSON: %w", err)
		}
	} else {
		var envelope struct {
			Data   []json.RawMessage `json:"data"`
			Models []json.RawMessage `json:"models"`
		}
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, fmt.Errorf("Models 响应不是有效 JSON: %w", err)
		}
		entries = envelope.Data
		if len(entries) == 0 {
			entries = envelope.Models
		}
	}

	seen := make(map[string]bool, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		id := ""
		var direct string
		if err := json.Unmarshal(entry, &direct); err == nil {
			id = direct
		} else {
			var item map[string]interface{}
			if err := json.Unmarshal(entry, &item); err != nil {
				continue
			}
			for _, key := range []string{"id", "model", "name"} {
				if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
					id = value
					break
				}
			}
		}
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// TestModel tests both /models and /chat/completions within one timeout. A
// successful /models response may establish API reachability/authentication,
// but never turns the overall result green when chat completions fail.
func TestModel(baseURL, apiKey, model string, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if baseURL == "" || apiKey == "" || model == "" {
		return Result{
			Protocol: "openai_chat",
			Message:  "Base URL、API Key 与模型均不能为空",
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()

	chatResult := make(chan EndpointResult, 1)
	modelsResult := make(chan EndpointResult, 1)
	go func() { chatResult <- testChatCompletion(ctx, baseURL, apiKey, model) }()
	go func() { modelsResult <- testModelsList(ctx, baseURL, apiKey, model) }()
	chat := <-chatResult
	models := <-modelsResult
	authenticationRejected := isAuthenticationFailure(chat.StatusCode) || isAuthenticationFailure(models.StatusCode)
	authenticationOK := !authenticationRejected && (isHTTPSuccess(chat.StatusCode) || isHTTPSuccess(models.StatusCode))
	result := Result{
		OK:               chat.OK,
		AuthenticationOK: authenticationOK,
		ModelsEndpointOK: models.OK,
		ChatCompletionOK: chat.OK,
		Latency:          time.Since(start),
		Message:          chat.Message,
		Protocol:         "openai_chat",
		Models:           models,
		Chat:             chat,
	}

	if chat.OK {
		result.Message = fmt.Sprintf("模型 %s 的 Chat Completions 可用", model)
		return result
	}
	if models.OK {
		result.Message = fmt.Sprintf("API 与 models 端点可达，但 Chat Completions 不可用: %s", chat.Message)
	}
	return result
}

func testChatCompletion(ctx context.Context, baseURL, apiKey, model string) EndpointResult {
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return EndpointResult{Message: "构造请求失败: " + err.Error(), Endpoint: endpoint}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return EndpointResult{
			Latency:  latency,
			Message:  "网络错误: " + err.Error(),
			Endpoint: endpoint,
		}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	result := EndpointResult{
		Latency:    latency,
		StatusCode: resp.StatusCode,
		Endpoint:   endpoint,
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		var payload struct {
			Choices []json.RawMessage `json:"choices"`
		}
		if err := json.Unmarshal(respBody, &payload); err != nil || len(payload.Choices) == 0 {
			result.Message = "Chat Completions 返回 2xx，但响应不是有效的 OpenAI choices 结构"
		} else {
			result.OK = true
			result.Message = fmt.Sprintf("模型 %s 可用", model)
		}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		result.Message = "认证失败（API Key 无效或无权限）"
	case resp.StatusCode == http.StatusNotFound:
		result.Message = "接口不存在（404）。请确认 Base URL 是否包含 /v1"
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity:
		message := shortBody(respBody)
		if strings.Contains(strings.ToLower(message), "model") {
			result.Message = "请求被拒绝，可能模型名不可用: " + message
		} else {
			result.Message = fmt.Sprintf("请求被拒绝 (HTTP %d): %s", resp.StatusCode, message)
		}
	default:
		result.Message = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, shortBody(respBody))
	}
	return result
}

func testModelsList(ctx context.Context, baseURL, apiKey, model string) EndpointResult {
	endpoint := modelsURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return EndpointResult{Endpoint: endpoint, Message: "构造请求失败: " + err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return EndpointResult{Latency: latency, Endpoint: endpoint, Message: "网络错误: " + err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	result := EndpointResult{Latency: latency, StatusCode: resp.StatusCode, Endpoint: endpoint}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			result.Message = "认证失败（API Key 无效或无权限）"
		} else {
			result.Message = fmt.Sprintf("models 列表 HTTP %d", resp.StatusCode)
		}
		return result
	}
	result.OK = true
	if model != "" && !bytes.Contains(raw, []byte(model)) {
		result.Message = fmt.Sprintf("models 端点可达，但列表中未找到 %s", model)
		return result
	}
	result.Message = fmt.Sprintf("models 端点可达，列表中包含 %s", model)
	return result
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

func shortBody(body []byte) string {
	message := strings.TrimSpace(string(body))
	message = strings.ReplaceAll(message, "\n", " ")
	if len(message) > 160 {
		return message[:160] + "…"
	}
	if message == "" {
		return "(空响应)"
	}
	return message
}

func isHTTPSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func isAuthenticationFailure(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}
