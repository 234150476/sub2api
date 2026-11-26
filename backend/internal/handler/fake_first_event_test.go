//go:build unit

package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// 守护首字加速：官方升级后如果注入点被移走、被吞掉或顺序变了，这里会失败。

// wantFakeChunks 写死为 2：CodeGO 会压一个包再转发，少于 2 个就失效。不要改成引用生产常量。
const wantFakeChunks = 2

const fakeFirstUpstreamChatStream = `data: {"id":"chatcmpl-upstream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"},"finish_reason":null}]}` + "\n\n" +
	`data: {"id":"chatcmpl-upstream","object":"chat.completion.chunk","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}` + "\n\n" +
	"data: [DONE]\n\n"

// sseDataLines 返回响应里所有 data: 行的 payload，顺序与客户端看到的一致。
func sseDataLines(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data: ") {
			out = append(out, strings.TrimPrefix(line, "data: "))
		}
	}
	return out
}

func isFakeFirstChunk(t *testing.T, payload string) bool {
	t.Helper()
	var chunk struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Choices []struct {
			Delta struct {
				Role    string  `json:"role"`
				Content *string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(payload), &chunk) != nil || len(chunk.Choices) != 1 {
		return false
	}
	d := chunk.Choices[0].Delta
	return chunk.Object == "chat.completion.chunk" && chunk.ID != "chatcmpl-upstream" &&
		d.Role == "assistant" && d.Content != nil && *d.Content == ""
}

// requireFakeFirstPrefix 校验：开启时恰好 2 个假包排在最前，其后是上游真实内容并正常收尾。
func requireFakeFirstPrefix(t *testing.T, body, contentMarker string, enabled bool) {
	t.Helper()
	lines := sseDataLines(body)
	require.GreaterOrEqual(t, len(lines), 2, body)
	fakes := 0
	for _, l := range lines {
		if isFakeFirstChunk(t, l) {
			fakes++
		}
	}
	if !enabled {
		require.Zero(t, fakes, "开关关闭时不得注入: %s", body)
		return
	}
	require.Equal(t, wantFakeChunks, fakes, "必须恰好 %d 个假包: %s", wantFakeChunks, body)
	for i := 0; i < wantFakeChunks; i++ {
		require.True(t, isFakeFirstChunk(t, lines[i]), "假包必须排在最前: %s", body)
	}
	require.Contains(t, strings.Join(lines[wantFakeChunks:], "\n"), contentMarker, "假包之后必须是上游真实内容")
	require.Equal(t, "[DONE]", lines[len(lines)-1])
}

func TestWriteFakeFirstChatChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeFakeFirstChatChunks(c, "gpt-4o")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
	lines := sseDataLines(rec.Body.String())
	require.Len(t, lines, wantFakeChunks)
	for _, l := range lines {
		require.True(t, isFakeFirstChunk(t, l), l)
	}
}

func runOpenAIChatWithFakeFirstEvent(t *testing.T, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	upstream := newAstraProCapturedUpstream(&http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(fakeFirstUpstreamChatStream)),
	})
	account := service.Account{
		ID: 7701, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "test-key", "base_url": "https://upstream.example.com"},
		Extra:       map[string]any{"fake_first_event": enabled, "openai_responses_supported": false},
	}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 1)}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	gateway := service.NewOpenAIGatewayService(
		&openAIWSUsageHandlerAccountRepoStub{account: account}, usageRepo, nil, nil, nil, nil, nil, cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, billing, upstream, &service.DeferredService{},
		nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(gateway, service.NewConcurrencyService(nil), billing,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg)

	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	groupID := int64(7702)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 7703, GroupID: &groupID,
		Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI},
		User:  &service.User{ID: 7704},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7704})

	h.ChatCompletions(c)

	_, accountIDs, _ := upstream.snapshot()
	require.Equal(t, []int64{7701}, accountIDs, "请求必须真正转发到上游: %s", rec.Body.String())
	return rec
}

func TestOpenAIChatCompletionsFakeFirstEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{true, false} {
		name := map[bool]string{true: "enabled", false: "disabled"}[enabled]
		t.Run(name, func(t *testing.T) {
			rec := runOpenAIChatWithFakeFirstEvent(t, enabled)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			requireFakeFirstPrefix(t, rec.Body.String(), "hello", enabled)
		})
	}
}
