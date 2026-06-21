// Package codex_template 嵌入真实 codex CLI 调 sharedchat 的 request body 模板。
//
// 来源: 2026-06-21 通过 mitmproxy 截取 codex 0.141.0 调 https://new.sharedchat.cc/codex/responses
// 的实际请求 body。包含 codex 完整 instructions / tools / reasoning / 工具列表等字段。
//
// 用途: sub2api 转发到 sharedchat 时，把客户端简单 body 嫁接到此模板，
// 让 sharedchat 校验通过 codex_access_restricted 检测。
package codex_template

import (
	_ "embed"
	"encoding/json"
)

//go:embed body.json
var rawBody []byte

// LoadTemplate 返回真 codex body 的 map 副本（每次调用返回独立副本，可安全修改）。
func LoadTemplate() (map[string]any, error) {
	var template map[string]any
	if err := json.Unmarshal(rawBody, &template); err != nil {
		return nil, err
	}
	return template, nil
}

// MustLoadTemplate 同 LoadTemplate，但 panic 而非返回 error（用于初始化时验证嵌入文件可解析）。
func MustLoadTemplate() map[string]any {
	template, err := LoadTemplate()
	if err != nil {
		panic("codex_template: invalid embedded body.json: " + err.Error())
	}
	return template
}
