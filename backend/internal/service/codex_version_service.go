package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// CodexVersionService 负责维护当前要伪装成的 Codex CLI 版本号，并据此组装 User-Agent。
//
// 行为：
//   - 启动时从 DB(setting) 读取上次缓存的版本，立即可用；
//   - 异步拉一次 GitHub Release，成功则更新内存与 DB；
//   - 后台 goroutine 按 RefreshInterval 周期性刷新；
//   - Refresh() 提供管理后台手动刷新入口；
//   - 整个降级链：GitHub → DB 缓存 → 配置 fallback_version → 代码硬编码兜底。
type CodexVersionService struct {
	cfg        *config.Config
	settingRepo SettingRepository
	httpClient *http.Client

	// 当前版本与对应 UA（atomic 读，无锁热路径）
	current atomic.Pointer[codexVersionSnapshot]

	// 启停控制
	startOnce sync.Once
	stopCh    chan struct{}
	stopped   atomic.Bool

	// refresh 互斥（避免并发 GitHub 请求）
	refreshMu sync.Mutex
}

// codexVersionSnapshot 是当前生效的版本快照（不可变，原子替换）
type codexVersionSnapshot struct {
	Version   string    `json:"version"`
	UserAgent string    `json:"user_agent"`
	Source    string    `json:"source"`     // "github" | "cache" | "fallback" | "hardcoded"
	FetchedAt time.Time `json:"fetched_at"`
}

// CodexVersionStatus 是返回给管理后台的状态信息
type CodexVersionStatus struct {
	Version          string    `json:"version"`
	UserAgent        string    `json:"user_agent"`
	Source           string    `json:"source"`
	FetchedAt        time.Time `json:"fetched_at"`
	AutoUpdate       bool      `json:"auto_update"`
	RefreshInterval  string    `json:"refresh_interval"`
	FallbackVersion  string    `json:"fallback_version"`
	UserAgentTemplate string   `json:"user_agent_template"`
}

const (
	// codexVersionSettingKey DB setting 表里缓存的键名
	codexVersionSettingKey = "gateway.codex_cli.cached_version"
	// codexVersionLastKnown 代码兜底版本（GitHub 不可达且 DB 也空时使用）
	// 与 config.fallback_version 默认值保持一致
	codexVersionLastKnown = "0.141.0"
	// codexVersionUserAgentDefault 模板渲染失败时的最终兜底 UA
	codexVersionUserAgentDefault = "codex_cli_rs/0.141.0 (Ubuntu 24.04.0; x86_64) xterm-256color"
)

// NewCodexVersionService 构造 service，但**不**启动后台 goroutine。
// 调用方需在 wire 注入完成后显式调用 Start()。
func NewCodexVersionService(cfg *config.Config, settingRepo SettingRepository) *CodexVersionService {
	timeoutSec := 10
	if cfg != nil && cfg.Gateway.CodexCLI.HTTPTimeoutSeconds > 0 {
		timeoutSec = cfg.Gateway.CodexCLI.HTTPTimeoutSeconds
	}
	svc := &CodexVersionService{
		cfg:         cfg,
		settingRepo: settingRepo,
		httpClient: &http.Client{
			Timeout: time.Duration(timeoutSec) * time.Second,
		},
		stopCh: make(chan struct{}),
	}
	// 立即同步装载一份兜底版本，保证 NewXxx 后调用方立刻可用
	svc.current.Store(svc.buildFallbackSnapshot())
	return svc
}

// Start 启动 service：先从 DB 装载上次缓存（同步、快），再异步拉 GitHub + 启动周期任务。
//
// 即使 ctx 被取消或 GitHub 不可达，Start() 也不会返回错误 —— 调用方拿到的版本永远可用，
// 只是 source 字段会反映实际数据来源。
func (s *CodexVersionService) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		// 1) 同步从 DB 装载上次缓存（快，毫秒级）
		if v := s.loadFromDB(ctx); v != "" {
			s.applyVersion(v, "cache", time.Time{})
		}
		// 2) 异步首次拉取（不阻塞启动）
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.refreshFromGitHub(refreshCtx); err != nil {
				slog.Warn("codex_version: initial GitHub fetch failed, using cached/fallback version",
					"error", err,
					"current_version", s.CurrentVersion(),
					"current_source", s.CurrentSource(),
				)
			}
		}()
		// 3) 启动周期刷新（仅在 auto_update=true 时）
		if s.cfg != nil && s.cfg.Gateway.CodexCLI.AutoUpdate {
			interval := s.cfg.Gateway.CodexCLI.RefreshInterval
			if interval <= 0 {
				interval = 6 * time.Hour
			}
			go s.periodicRefresh(interval)
		}
	})
}

// Stop 停止后台刷新 goroutine（用于优雅关闭）
func (s *CodexVersionService) Stop() {
	if s.stopped.CompareAndSwap(false, true) {
		close(s.stopCh)
	}
}

// CurrentVersion 同步返回当前版本号（如 "0.141.0"），永不为空
func (s *CodexVersionService) CurrentVersion() string {
	if snap := s.current.Load(); snap != nil {
		return snap.Version
	}
	return codexVersionLastKnown
}

// CurrentUserAgent 同步返回组装好的 User-Agent，永不为空
func (s *CodexVersionService) CurrentUserAgent() string {
	if snap := s.current.Load(); snap != nil && snap.UserAgent != "" {
		return snap.UserAgent
	}
	return codexVersionUserAgentDefault
}

// CurrentSource 同步返回当前数据来源（github/cache/fallback/hardcoded）
func (s *CodexVersionService) CurrentSource() string {
	if snap := s.current.Load(); snap != nil {
		return snap.Source
	}
	return "hardcoded"
}

// Status 返回完整状态（admin handler 使用）
func (s *CodexVersionService) Status() CodexVersionStatus {
	snap := s.current.Load()
	if snap == nil {
		snap = s.buildFallbackSnapshot()
	}
	autoUpdate := false
	interval := time.Duration(0)
	fallback := codexVersionLastKnown
	tmpl := ""
	if s.cfg != nil {
		autoUpdate = s.cfg.Gateway.CodexCLI.AutoUpdate
		interval = s.cfg.Gateway.CodexCLI.RefreshInterval
		fallback = s.cfg.Gateway.CodexCLI.FallbackVersion
		tmpl = s.cfg.Gateway.CodexCLI.UserAgentTemplate
	}
	return CodexVersionStatus{
		Version:           snap.Version,
		UserAgent:         snap.UserAgent,
		Source:            snap.Source,
		FetchedAt:         snap.FetchedAt,
		AutoUpdate:        autoUpdate,
		RefreshInterval:   interval.String(),
		FallbackVersion:   fallback,
		UserAgentTemplate: tmpl,
	}
}

// Refresh 手动触发一次 GitHub 拉取，返回拉到的版本号或错误
func (s *CodexVersionService) Refresh(ctx context.Context) (string, error) {
	return s.refreshFromGitHub(ctx)
}

// ---- 内部实现 ----

// periodicRefresh 后台周期任务
func (s *CodexVersionService) periodicRefresh(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if _, err := s.refreshFromGitHub(ctx); err != nil {
				slog.Warn("codex_version: periodic GitHub fetch failed, retaining current snapshot",
					"error", err,
					"current_version", s.CurrentVersion(),
				)
			}
			cancel()
		}
	}
}

// refreshFromGitHub 调 GitHub API 拉最新 release，成功则更新 atomic + DB
func (s *CodexVersionService) refreshFromGitHub(ctx context.Context) (string, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	if s.cfg == nil {
		return "", errors.New("codex_version: nil config")
	}
	apiURL := strings.TrimSpace(s.cfg.Gateway.CodexCLI.GitHubAPIURL)
	if apiURL == "" {
		apiURL = "https://api.github.com/repos/openai/codex/releases/latest"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("codex_version: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "sub2api-codex-version-fetcher")
	if token := strings.TrimSpace(s.cfg.Gateway.CodexCLI.GitHubToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("codex_version: github request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("codex_version: github status %d: %s", resp.StatusCode, string(body))
	}

	var release struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("codex_version: decode release: %w", err)
	}
	if release.Draft || release.Prerelease {
		return "", fmt.Errorf("codex_version: latest release is draft/prerelease (tag=%q)", release.TagName)
	}
	version := extractCodexVersion(release.TagName)
	if version == "" {
		return "", fmt.Errorf("codex_version: cannot parse version from tag %q", release.TagName)
	}

	s.applyVersion(version, "github", time.Now())
	// 异步落库（DB 失败不影响内存生效）
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.saveToDB(ctx, version); err != nil {
			slog.Warn("codex_version: persist to DB failed",
				"error", err,
				"version", version,
			)
		}
	}()
	slog.Info("codex_version: refreshed from GitHub",
		"version", version,
		"tag", release.TagName,
	)
	return version, nil
}

// applyVersion 渲染 UA 并原子替换当前快照
func (s *CodexVersionService) applyVersion(version, source string, fetchedAt time.Time) {
	if version == "" {
		return
	}
	ua := s.renderUserAgent(version)
	if fetchedAt.IsZero() {
		fetchedAt = time.Now()
	}
	s.current.Store(&codexVersionSnapshot{
		Version:   version,
		UserAgent: ua,
		Source:    source,
		FetchedAt: fetchedAt,
	})
}

// buildFallbackSnapshot 仅用于 NewXxx 兜底（没有任何外部信号时使用）
func (s *CodexVersionService) buildFallbackSnapshot() *codexVersionSnapshot {
	version := codexVersionLastKnown
	source := "hardcoded"
	if s.cfg != nil && strings.TrimSpace(s.cfg.Gateway.CodexCLI.FallbackVersion) != "" {
		version = strings.TrimSpace(s.cfg.Gateway.CodexCLI.FallbackVersion)
		source = "fallback"
	}
	return &codexVersionSnapshot{
		Version:   version,
		UserAgent: s.renderUserAgent(version),
		Source:    source,
		FetchedAt: time.Time{},
	}
}

// renderUserAgent 用配置模板渲染 UA。模板缺失或渲染失败时回退到硬编码 UA。
func (s *CodexVersionService) renderUserAgent(version string) string {
	tmplStr := ""
	if s.cfg != nil {
		tmplStr = strings.TrimSpace(s.cfg.Gateway.CodexCLI.UserAgentTemplate)
	}
	if tmplStr == "" {
		return fmt.Sprintf("codex_cli_rs/%s (Ubuntu 24.04.0; x86_64) xterm-256color", version)
	}
	tmpl, err := template.New("codex_ua").Parse(tmplStr)
	if err != nil {
		slog.Warn("codex_version: parse UA template failed, using hardcoded UA",
			"error", err,
			"template", tmplStr,
		)
		return fmt.Sprintf("codex_cli_rs/%s (Ubuntu 24.04.0; x86_64) xterm-256color", version)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, map[string]string{"Version": version}); err != nil {
		slog.Warn("codex_version: execute UA template failed, using hardcoded UA",
			"error", err,
		)
		return fmt.Sprintf("codex_cli_rs/%s (Ubuntu 24.04.0; x86_64) xterm-256color", version)
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return fmt.Sprintf("codex_cli_rs/%s (Ubuntu 24.04.0; x86_64) xterm-256color", version)
	}
	return out
}

// loadFromDB 同步读取 DB 缓存的版本号，失败或不存在返回 ""
func (s *CodexVersionService) loadFromDB(ctx context.Context) string {
	if s.settingRepo == nil {
		return ""
	}
	v, err := s.settingRepo.GetValue(ctx, codexVersionSettingKey)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// saveToDB 异步落库当前版本号
func (s *CodexVersionService) saveToDB(ctx context.Context, version string) error {
	if s.settingRepo == nil {
		return nil
	}
	return s.settingRepo.Set(ctx, codexVersionSettingKey, version)
}

// extractCodexVersion 从 GitHub tag_name 抽取版本号。
//
// Codex 仓库的 tag 格式是 "rust-v0.141.0"，但也兼容 "v0.141.0" / "0.141.0" 三种写法，
// 防止上游改命名规则后服务直接挂掉。
func extractCodexVersion(tag string) string {
	t := strings.TrimSpace(tag)
	if t == "" {
		return ""
	}
	// 依次剥前缀
	for _, prefix := range []string{"rust-v", "rust-", "v"} {
		if strings.HasPrefix(t, prefix) {
			t = strings.TrimPrefix(t, prefix)
			break
		}
	}
	t = strings.TrimSpace(t)
	// 必须形如 X.Y.Z
	if !looksLikeSemver(t) {
		return ""
	}
	return t
}

// looksLikeSemver 极简 semver 校验（X.Y.Z，每段都是数字）
func looksLikeSemver(s string) bool {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				// 允许 "0.141.0-beta" 这种后缀，仅检查首段非数字才拒绝
				if p == parts[0] || p == parts[1] {
					return false
				}
				break
			}
		}
	}
	return true
}
