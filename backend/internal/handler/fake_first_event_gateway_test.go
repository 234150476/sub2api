//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeFirstAnthropicUpstream 返回 Anthropic 流；写法与 gateway_compat_stream_usage_test.go 一致。
type fakeFirstAnthropicUpstream struct {
	service.HTTPUpstream
	calls int
}

func (u *fakeFirstAnthropicUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	u.calls++
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(compatStreamUsagePartialStream + compatStreamUsageCompletion)),
	}, nil
}

func runGatewayChatWithFakeFirstEvent(t *testing.T, enabled bool) (*httptest.ResponseRecorder, int) {
	t.Helper()
	cfg := &config.Config{Gateway: config.GatewayConfig{StreamDataIntervalTimeout: 1}}
	groupID := int64(7801)
	group := &service.Group{ID: groupID, Hydrated: true, Platform: service.PlatformAnthropic, Status: service.StatusActive, RateMultiplier: 1}
	account := &service.Account{
		ID: 7802, Platform: service.PlatformAnthropic, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials:   map[string]any{"api_key": "test-key"},
		Extra:         map[string]any{"fake_first_event": enabled},
		AccountGroups: []service.AccountGroup{{AccountID: 7802, GroupID: groupID}},
	}
	snapshot := service.NewSchedulerSnapshotService(&fakeSchedulerCache{accounts: []*service.Account{account}}, nil, nil, nil, nil)
	billingCache := service.NewBillingCacheService(newHandlerInflightCache(100), nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	upstream := &fakeFirstAnthropicUpstream{}
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 2)}
	gw := service.NewGatewayService(
		nil, &fakeGroupRepo{group: group}, usageRepo, &compatStreamUsageBillingRepo{}, nil, nil, nil, nil, cfg,
		snapshot, nil, service.NewBillingService(cfg, nil), nil, nil, nil, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	h := &GatewayHandler{
		gatewayService: gw, billingCacheService: billingCache, cfg: cfg, maxAccountSwitches: 1,
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := `{"model":"claude-sonnet-4.5","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID: 7803, UserID: 7804, GroupID: &groupID, Group: group, User: &service.User{ID: 7804, Balance: 100},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 7804})

	h.ChatCompletions(c)
	return rec, upstream.calls
}

func TestGatewayChatCompletionsFakeFirstEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{true, false} {
		name := map[bool]string{true: "enabled", false: "disabled"}[enabled]
		t.Run(name, func(t *testing.T) {
			rec, calls := runGatewayChatWithFakeFirstEvent(t, enabled)
			require.Equal(t, 1, calls, rec.Body.String())
			requireFakeFirstPrefix(t, rec.Body.String(), "partial", enabled)
		})
	}
}
