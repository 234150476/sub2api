package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/codex_template"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// rewriteBodyForCodexClient 用真 codex CLI 的请求 body 模板重写客户端 body。
//
// 公益站如 sharedchat 检测的不只是 ja3 / headers，还包括 body schema：
// 真 codex CLI 调 /codex/responses 时 body ~33 KB，含 instructions / tools / reasoning /
// client_metadata / include 等十几个 codex 特有字段；只发简单 {"input":"hi"} 会被拒。
//
// 嫁接策略 (方案 C)：
//  1. 读取真 codex body 模板（来自 codex_template/body.json）
//  2. 把客户端 body 的 input / model 等关键字段 merge 进模板
//  3. 给所有 UUID/timestamp 字段生成新值（避免 sharedchat 检测重复 turn）
//  4. 输出合成后的 body
//
// 调用方负责保证 originalBody 是合法 JSON。
func rewriteBodyForCodexClient(originalBody []byte, sessionID, threadID, turnID, windowID string) ([]byte, error) {
	if !json.Valid(originalBody) {
		return originalBody, fmt.Errorf("rewriteBodyForCodexClient: original body is not valid JSON")
	}

	template, err := codex_template.LoadTemplate()
	if err != nil {
		return originalBody, fmt.Errorf("rewriteBodyForCodexClient: load template: %w", err)
	}

	// 1. 用客户端的 model 覆盖（保留模板默认 gpt-5.4 时使用模板值）
	if model := gjson.GetBytes(originalBody, "model"); model.Exists() && model.String() != "" {
		template["model"] = model.String()
	}

	// 2. 用客户端的 input 覆盖（响应 API 的 input 可能是字符串或数组）
	if input := gjson.GetBytes(originalBody, "input"); input.Exists() {
		var parsed any
		if err := json.Unmarshal([]byte(input.Raw), &parsed); err == nil {
			template["input"] = parsed
		}
	}

	// 3. 用客户端的 max_output_tokens（如果有）
	if maxTokens := gjson.GetBytes(originalBody, "max_output_tokens"); maxTokens.Exists() {
		template["max_output_tokens"] = maxTokens.Int()
	}

	// 4. 给 UUID 字段生成本次请求新值（每次都不一样，避免被检测重放）
	if windowID == "" {
		windowID = sessionID + ":0"
	}
	if turnID == "" {
		turnID = uuid.NewString()
	}
	installationID := uuid.NewString()

	template["prompt_cache_key"] = sessionID

	// client_metadata 也要更新
	if metadata, ok := template["client_metadata"].(map[string]any); ok {
		metadata["session_id"] = sessionID
		metadata["thread_id"] = threadID
		metadata["turn_id"] = turnID
		metadata["x-codex-window-id"] = windowID
		metadata["x-codex-installation-id"] = installationID

		// x-codex-turn-metadata 是 stringified JSON
		turnMeta := map[string]any{
			"installation_id":          installationID,
			"session_id":               sessionID,
			"thread_id":                threadID,
			"turn_id":                  turnID,
			"window_id":                windowID,
			"request_kind":             "turn",
			"sandbox":                  "seccomp",
			"turn_started_at_unix_ms":  time.Now().UnixMilli(),
		}
		if turnMetaBytes, err := json.Marshal(turnMeta); err == nil {
			metadata["x-codex-turn-metadata"] = string(turnMetaBytes)
		}
	}

	// 5. marshal 输出
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(template); err != nil {
		return originalBody, fmt.Errorf("rewriteBodyForCodexClient: marshal: %w", err)
	}
	// 去掉 Encode 自动加的尾部换行
	out := bytes.TrimRight(buf.Bytes(), "\n")
	return out, nil
}
