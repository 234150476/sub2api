# sub2api 流量伪装与 sharedchat ja3 攻坚

**Date**: 2026-06-20
**Status**: 部分完成（流量伪装功能上线，sharedchat ja3 攻坚因 utls 库能力上限放弃）

---

## Goal

1. 给 sub2api 加 Codex CLI 自动跟版能力
2. 给 sub2api 加分组级流量伪装开关 + SSE 极速心跳（核心目标：让 axonhub 排名上升）
3. 让 sub2api 转发能通过 sharedchat 的 codex_access_restricted 校验

---

## Outcome

### ✅ 已完成

- **特性 1：Codex CLI 自动跟版**
  - 后端：`internal/service/codex_version_service.go`（4 层降级链：GitHub → DB → fallback → hardcoded）
  - 前端：系统设置 → 网关 tab → Codex CLI 指纹版本卡片
  - 当前版本：0.141.0（每 6h 自动从 GitHub 拉取）

- **特性 2：分组级流量伪装**
  - schema 加 4 字段（traffic_shaping_enabled / sse_early_keepalive_enabled / stream_smoothing_enabled / silent_retry_5xx_enabled）
  - 默认全 false，对现有分组零影响
  - SSE 极速心跳已接到 OpenAI passthrough 路径（`openai_gateway_service.go:3839`）

- **修复 4 个真实代码 bug**（v3 镜像）：
  1. `account.go:1583` 删除 `IsAnthropicOAuthOrSetupToken()` 硬编码限制 — 让所有账号类型都能用 TLS 指纹
  2. `accounts.extra` 字段名对齐 — 代码读 `enable_tls_fingerprint`，前端/SQL 之前误用 `tls_fingerprint_enabled`
  3. `dialer.go:279` 显式设 `utls.Config.CurvePreferences` — 修复 utls 1.8.2 默认值 `[X25519MLKEM768, ...]` 第 0 个是 PQ 但 `curveForCurveID` 不实现导致 TLS 1.3 路径必抛 `unsupported curve` 的 bug
  4. `GroupTrafficShapingCard.vue` v-model bug — 子组件 emit 整对象时父组件 reactive 不响应，改用 props 直接修改字段

### ❌ 未达成

- **sharedchat (`new.sharedchat.cc/codex`) ja3 攻坚失败** — 撞到 utls 1.8.2 库能力上限

---

## Key Findings

### 决定性事实链

- 同一台服务器 / 同一 IP / 同一 API key（已脱敏） / 同一 ja3
- ✅ 真实 codex CLI 调 sharedchat 成功（"Hi there friend"，pcap 139K）
- ❌ sub2api v3 转发同样请求 → 403 codex_access_restricted

**真实 codex CLI ja3**: `27718d56688425cd36a401c66147c4ee`
**sub2api v3 ja3**: `b45d82e7c5bc52a83f1ebf515db3445b`（不一致 → sharedchat 拒绝）

### utls 1.8.2 死循环

- 真实 codex CLI 的 SupportedGroups 含 `4588 (X25519MLKEM768)` / `30 (X448)` / `256/257 (FFDHE2048/3072)` / Brainpool curves / MLDSA 签名
- 这些 utls 1.8.2 **不实现密钥生成** — 留着会抛 `tls: CurvePreferences includes unsupported curve`
- 删掉它们 → ja3 hash 变了 → 跟真实 codex 不一致 → sharedchat 拒绝
- **配置层 + utls 层都已穷尽，无法在 sub2api 用 Go + utls 仿出真实 codex CLI 的字节级精确指纹**

---

## 部署状态

```
镜像: sub2api:traffic-shaping-2026-06-20-v3 (b75a5a272311, 145MB)
位置: /opt/sub2api/docker-compose.yml
回滚备份: docker-compose.yml.pre-v2 / pre-v3
保留旧镜像: v1 (e22e9d1098cd) + v2 (bef4dec78c63)

数据库:
- profile #1 在表 `tls_fingerprint_profiles`，name=`codex_cli_rs (real ja3 capture)`，含 cipher_suites/curves/extensions 等 jsonb 字段
- account 47 (sharedchat-公益)：TLS 指纹配置在 `accounts.extra` (jsonb)，**不是外键**：
  - `enable_tls_fingerprint: true`
  - `tls_fingerprint_profile_id: 1`（jsonb 内嵌字段，应用层解析）
  - `openai_responses_supported: true`
  - `openai_apikey_responses_websockets_v2_mode: passthrough`
  - `openai_apikey_responses_websockets_v2_enabled: true`
- account 47 已 `schedulable=false`（2026-06-20 收尾时暂停）

迁移: 154 (group_traffic_shaping，4 字段全 false 默认)
```

---

## Future Path（不在本次会话范围）

如果未来真要救 sharedchat 类公益站：

### 路径 3（多人并发推荐 ⭐）：Rust sidecar
- 写 ~100-200 行 Rust + axum + reqwest，**复用 codex 自己的 `default_client.rs`**（`build_reqwest_client()`）
- TLS 指纹跟真 codex 100% 一致（同一个 reqwest + rustls + aws-lc-rs）
- 部署成 docker compose 旁边的 service，sub2api 转发到 `http://codex-sidecar:8888/codex`
- 工作量：1-2 天

### 路径 1（不推荐）：fork utls 加 PQ 实现
- 学 BoringSSL 的 X25519MLKEM768 算法（论文级密码学）
- 用 Go 写实现（不能直接抄 C 代码）
- 工作量：2-4 周

### 路径 2（侵入大）：cgo 调 BoringSSL
- 引入 BoringSSL 作为 C 依赖，sub2api 编译变复杂
- 工作量：1-2 周

### 路径 4（单人可用，并发死路）：CLI shim
- 每请求 fork codex 子进程
- 2GB 内存 10 并发就 OOM，**多人场景直接死**

---

## 环境信息（永久）

### 本地 Windows
- Go 缓存重定向到 E 盘永久：`GOCACHE=E:\Temp\go-build` / `GOMODCACHE=E:\Temp\go-mod` / `GOTMPDIR=E:\Temp\go-tmp` / `GOPATH=E:\Temp\go-mod`
- C 盘零增长策略
- pscp.exe 在 `C:\Program Files\PuTTY\pscp.exe`，hostkey 通过 ssh known_hosts 维护（不要写入仓库）

### 远端 sub2api-sg（IP 见 agent-ssh-cli config）
- OS: OpenCloudOS 9.4 (RHEL9 衍生，dnf 包管理)
- 已装：Node.js 20.20.0 / npm 10.8.2 / wireshark-cli (tshark 4.6.5) / `@openai/codex` 0.141.0
- `/root/.codex/config.toml` 配置指向 sharedchat
- PG 用户名 **`sub2api`** / 库名 **`sub2api`**（不是 postgres、也不是 claude_relay！），密码在容器 env `POSTGRES_PASSWORD`
- 探 PG 命令：`agentsshcli exec sub2api-sg "docker exec sub2api-postgres psql -U sub2api -d sub2api -c '<sql>'"`
- 数据备份：`~/sub2api-backup-20260620-0415/`（db.sql.gz 31MB + image-original.tar.gz 59MB + compose backup）
- 容器：sub2api / sub2api-redis / sub2api-postgres

### agent-ssh-cli
- 配置：`~/.agent-ssh-cli/config.json`，连接名 `sub2api-sg`
- 密码已加密到 secrets.json
- commandBlacklist 拦了 `rm -rf /` / shutdown / reboot / mkfs / DROP DATABASE 等

---

## 部署流程（已成功跑过 v1/v2/v3 三次）

```bash
# 本地
1. 改代码
2. docker build --platform linux/amd64 -t sub2api:traffic-shaping-2026-06-20-vN .
3. docker save → gzip → E:\Temp\sub2api-vN.tar.gz

# 上传
4. pscp -pw <pw> -batch -hostkey <hostkey> E:\Temp\sub2api-vN.tar.gz root@<server-ip>:/tmp/sub2api-vN.tar.gz

# 远端
5. docker load -i /tmp/sub2api-vN.tar.gz
6. cd /opt/sub2api && sed -i.pre-vN 's|image: sub2api:traffic-shaping-...-OLD|image: sub2api:traffic-shaping-...-vN|' docker-compose.yml
7. docker compose up -d sub2api  # 业务中断 ~10 秒
8. 验证 health=200 + 看启动日志
```

---

## Lessons

1. **配置层不能解决协议外差异** — TLS 字段可调但 HTTP/2 帧序、密码学库实现是协议外的
2. **开源 ≠ 能仿** — 仿制需要 4 个条件：源码可见 ✓ + 字段已知 ✓ + 库实现这些字段 ✗ + 协议外细节相同 ✗
3. **utls 是 fork 自 Go crypto/tls** — 继承了 Go 标准库的 cipher/curve 实现局限
4. **抓真实包对照** 是诊断 ja3 类问题的唯一可靠方法（猜参数浪费时间）
5. **sub2api 的 v3 改进对宽松检测的公益站仍有用** — 不要因为 sharedchat 失败就觉得改动白做

---

## B 阶段：收尾清理（2026-06-20 完成）

| 操作 | 状态 |
|---|---|
| 账号 47 schedulable=false | ✅ 已执行 |
| 删测试 pcap (6 个) | ✅ 已删 sub2api-call/sub2api-v2-test/v2-test{2,3,4}/v3-final-test |
| 保留原始证据 pcap | ✅ /tmp/codex-real.pcap (316K) + /tmp/codex-real-call.pcap (139K) — Path 3 字节级对照原料 |
| profile #1 | ✅ 保留 — 下次 Path 3 时用 |
| PRD 修正 | ✅ jsonb 机制 + PG 凭证 |

---

## 会话 6 (2026-06-21) — 多账号配置 + 三大功能完整闭环

### v8 - v15 完整版本里程碑

| 版本 | 核心改动 | commit |
|---|---|---|
| v8 | sharedchat 测试连接补 codex body+headers 注入 | 59269904 |
| v9 | strict_anthropic_passthrough 子开关救 PackyCode "请求体被篡改"403 | 9da455d0 |
| v10 | 账号编辑弹窗加 inject_codex_headers + strict_anthropic_passthrough 开关 UI | c8d2f610 |
| v11 | codex_strict_passthrough 子开关（body 路径不重写） | 08464a88 |
| v12 | codex strict 真二选一（body+headers+白名单都跳过） | 76b5616f |
| v13 | 修流量伪装 4 开关 update 路径漏链 | 9b36bba7 |
| v14 | 修流量伪装 4 字段 GET 路径 dto 漏链 | ea91fc7c |
| v15 | stream_smoothing 真实现（OpenAI passthrough SSE 主循环 15ms 节流） | 065755c2 |

### 三大功能最终生效点

| 功能 | 字段 | 真生效？ | 落点 |
|---|---|---|---|
| 流量伪装 SSE 极速心跳 | `groups.sse_early_keepalive_enabled` | ✅ | `openai_gateway_service.go:3942` 写 `: ping\n\n` |
| 流量伪装 流式响应平滑 | `groups.stream_smoothing_enabled` | ✅ v15 实现 | OpenAI passthrough SSE 主循环每行后 15ms sleep |
| 流量伪装 5xx 静默重试 | `groups.silent_retry_5xx_enabled` | ⚠️ 占位字段 | 字段定义齐但 0 个消费者，下次实现 |
| Codex 注入 codex CLI 特征 | `accounts.extra.inject_codex_headers` | ✅ | account 47 sharedchat-公益 |
| Codex 严格透传 | `accounts.extra.codex_strict_passthrough` | ✅ v12 真二选一 | 仅 inject_codex_headers=true 时生效 |
| Anthropic 自动透传 | `accounts.extra.anthropic_passthrough` | ✅ 已有 | account 11/35 |
| Anthropic 严格透传 | `accounts.extra.strict_anthropic_passthrough` | ✅ v9 实现 | 跳过 StripEmptyTextBlocks + sanitizeAnthropicBodyForBetaTokens |

### 决定性发现

#### 1. axonhub TTFT 真实计算逻辑（直接读源码确认）

来源：`looplj/axonhub` `internal/server/orchestrator/performance.go:149-158`

```go
func (s *recordPerformanceStream) Current() *llm.Response {
    event := s.stream.Current()
    if event == nil { return event }
    if !s.firstTokenSet && s.state.Perf != nil {
        s.state.Perf.MarkFirstToken()  // ← TTFT 计时停止
        s.firstTokenSet = true
    }
}
```

`s.stream.Current()` 返回的是**已解析的 `*llm.Response` 结构体**，SSE 注释帧
（`: ping`）在更上游的 SSE parser 阶段就被丢掉。所以我们的 `: ping` 帧
**不会被 axonhub 计为首字节**，没有"作弊"嫌疑。axonhub 测的是真实首个有效
event 的到达时间。

#### 2. PackyCode "请求体被篡改"根因

sub2api 在 anthropic_passthrough 路径上有两处 body 改动：
- `gateway_service.go:5553` `StripEmptyTextBlocks` — 剥空 text block
- `gateway_service.go:5786` `sanitizeAnthropicBodyForBetaTokens` — 按 beta header 删 context_management 字段

PackyCode/rightcode 等公益站对 body 做 hash 校验，sub2api 任何修改都会
报"请求体被篡改"。`strict_anthropic_passthrough=true` 跳过这两步，让 body 字节级原样上送。

#### 3. inject_codex_headers 当前实现是"模拟模式"，不是"透传模式"

`codex_body_rewriter.go` 把客户端 body 整个丢弃，载入 sub2api embed 的 33KB
codex 模板，重新生成所有 UUID。这对 curl/普通 SDK 是救命方案；对真 codex
CLI 客户端反而破坏 prompt cache_key/UUID 连续性。

`codex_strict_passthrough=true` 跳过 body rewrite + headers inject + 放宽
header 白名单（`codexStrictPassthroughExtraHeaders`），让真 codex CLI 字节
级透传。

### 远端最终状态（2026-06-21）

| 项 | 值 |
|---|---|
| 镜像 | sub2api:traffic-shaping-2026-06-21-v15 |
| 容器 | sub2api / sub2api-postgres / sub2api-redis 全 healthy |
| 镜像更新风险 | 零（tag 在 registry 不存在 + pull_policy=missing + 无 watchtower） |

### 账号开关分布

```sql
-- account 47 sharedchat-公益 (openai apikey)
extra: {
  "enable_tls_fingerprint": true,
  "tls_fingerprint_profile_id": 1,
  "inject_codex_headers": true,
  "openai_responses_supported": true
}

-- account 11 rightcode (anthropic apikey)
-- account 35 PackyCode (anthropic apikey)
extra: {
  "anthropic_passthrough": true,
  "strict_anthropic_passthrough": true
}

-- group 36 linuxhub (openai)
{
  traffic_shaping_enabled: true,
  sse_early_keepalive_enabled: true,
  stream_smoothing_enabled: true,
  silent_retry_5xx_enabled: true
}
```

### 已知未实现/未验证

- `silent_retry_5xx_enabled` 字段已加但 0 消费者
- group 36 当前唯一账号 52 linuxdohub 401 (refresh token 失效)，schedulable=false，无法验证流量伪装实际效果
- axonhub 之前测的 3.51s 不是 group 36 数据（它账号挂了），来源待你下次确认

### Lessons (会话 6 新增)

6. **read/write 字段链路必须对称** — 加了字段不仅要写到 DB，还要让 GET 路径回传。
   v13 修了 update 但 dto 漏，造成"保存后重新打开还是 OFF"假象，看似 bug 实际是
   GET 路径漏链。
7. **占位字段必须在 UI 标 coming-soon** — silent_retry_5xx UI 标了 `auto-on` 而不是
   `coming-soon`，让用户误以为能用。
8. **二选一就要真二选一** — codex_strict_passthrough 第一版只跳了 body 没跳 headers，
   逻辑不对称。修真二选一 = body + headers + 白名单都按开关切换。
9. **DB 不是唯一真相，缓存才是** — sub2api 用 scheduler_snapshot 把 group 数据
   缓存到 redis，DB 改了但缓存可能滞后。debug 时先 invalidate 再观察。
10. **直接读源码 > 猜实现** — axonhub TTFT 怎么算的，10 分钟读源码 > 1 小时猜。

---

## 会话 7（2026-06-21 收尾）— 持久化记忆

> 用户决定删除本地仓库，本会话所有沉淀已写入：
> - 本 PRD 文件（追加会话 6 完整记录）
> - 远端 GitHub: 234150476/sub2api 包含 v8-v15 全部 commit (24 个本地 commit 全部 push)
> - memory/ 下用户偏好 + 部署纪律 + 系统知识
> - journal-1.md 追加会话索引
