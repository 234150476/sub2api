# Journal - cfy (Part 1)

> AI development session journal
> Started: 2026-06-20

---

## 2026-06-20 会话 1-3 — sharedchat ja3 攻坚

详见 `.trellis/tasks/06-20-06-20-sub2api-sharedchat-ja3/prd.md`

- 流量伪装基础设施 + Codex CLI 自动跟版（v1）
- 4 个真实代码 bug 修复（v3 镜像）
- sharedchat ja3 攻坚因 utls 1.8.2 库能力上限放弃

## 2026-06-20 会话 4 — Rust sidecar 路线 + body 嫁接突破

详见 `.trellis/tasks/06-20-rust-tls-sidecar/prd.md`

- Rust sidecar 工程完成且 ja3 字节级 100% = 真 codex CLI
- 实测发现 sharedchat 检测优先级是 body schema ≫ headers ≫ ja3 字节
- 应用层方案 (codex_body_rewriter + headers inject) 救活 sharedchat
- v7 镜像 sub2api 实测通过完整 codex 客户端检测（命中公益站每日额度）

## 2026-06-21 会话 5 — 端到端验证 sharedchat 通过

- v7 实测 HTTP 200 返回 "Hi there today" 3 词
- 归档 rust-tls-sidecar 任务

## 2026-06-21 会话 6 — 多账号配置 + 三大功能完整闭环

- v8: sharedchat 测试连接补 codex body+headers
- v9: strict_anthropic_passthrough（PackyCode 篡改 403 修复）
- v10: 账号编辑弹窗加 inject_codex_headers + strict_anthropic_passthrough UI
- v11/v12: codex_strict_passthrough 真二选一（body+headers+白名单）
- v13/v14: 修流量伪装 update + GET 路径漏链
- v15: stream_smoothing 真实现

## 2026-06-21 会话 7 — 持久化收尾

用户决定删除本地仓库。所有沉淀已保存：
- 远端 GitHub: 234150476/sub2api（24 个本地 commit 全部 push）
- 本 PRD 完整记录会话 1-7
- ~/.claude/projects/.../memory/ 用户偏好 + 部署纪律
- 远端 sub2api-sg 镜像 v15 healthy
