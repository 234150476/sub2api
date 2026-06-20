# Rust TLS Sidecar — 让 sub2api 出向 ja3 = 真实 codex CLI

**Date**: 2026-06-20
**Status**: planning (brainstorm 完成)
**Parent context**: `06-20-06-20-sub2api-sharedchat-ja3` (上一任务的 Path 3 推荐方案)
**Priority**: P2

---

## Goal

让 sub2api 转发请求到 sharedchat 等 ja3 严格的公益站时，**出向 TLS ClientHello 字节级匹配真实 codex CLI**，绕过 `codex_access_restricted` 校验。

**成功判据**：
- `agentsshcli exec sub2api-sg "tcpdump …"` 抓到的 sub2api → sharedchat ja3 = `27718d56688425cd36a401c66147c4ee`（真实 codex ja3）
- 同一 API key + 同一账号在 sub2api 调 sharedchat 不再 403
- 多人并发 ≥10 不 OOM、响应正常

---

## 关键发现（brainstorm 阶段已确认）

### 1. codex 的 ja3 来源不是定制配置，是依赖默认值

读了 `codex-rs/codex-client/src/default_client.rs` 和 `custom_ca.rs`：

```rust
// 整个 build 路径里没有：
//   ❌ cipher suite 选择
//   ❌ ALPN 配置
//   ❌ HTTP/2 toggle
//   ❌ TLS 版本 pin
//   ❌ KX groups 定制
//   ❌ rustls ClientConfig 定制
// 只有：
let client = reqwest::Client::builder()
    .default_headers(default_headers())  // UA + originator + residency
    .build()?;
```

### 2. 真正"决定 ja3"的就一行

`codex-rs/utils/rustls-provider/src/lib.rs` (1.5 KB)：

```rust
rustls::crypto::aws_lc_rs::default_provider().install_default()?;
```

ja3 来自这行设的 **rustls 0.23 + aws-lc-rs** 默认值，**不是任何代码"配置"出来的**。

### 3. 因此 Path 3 不需要 vendor codex 源码

只需要：
- 写一个新 Rust crate（独立 service）
- Cargo.toml 锁定 codex 同款依赖版本
- 用 `reqwest::Client::builder().build()` 默认值
- 启动 `rustls::crypto::aws_lc_rs::default_provider().install_default()`
- 用 axum 起一个透明 HTTP proxy

整个 sidecar **预计 80-150 行 Rust 代码**，无需 vendor 任何 codex 源码。

### 4. codex 同款依赖（已从 codex-rs/Cargo.toml 提取）

```toml
reqwest = { version = "0.12", features = ["cookies"] }
rustls = { version = "0.23", default-features = false, features = ["aws_lc_rs", "std"] }
axum = { version = "0.8", default-features = false }
tokio = "1"
```

---

## 架构决策（brainstorm 锁定）

| 决策点 | 选择 | 理由 |
|---|---|---|
| **D1 部署形态** | 独立 docker service | 内部网络 / 重启隔离 / 不污染 sub2api 镜像 |
| **D2 API 协议** | 透明 HTTP 代理 | sidecar 越薄越好，业务逻辑全留 sub2api |
| **D3 启用范围** | 按 account/group 配置开关 | 默认不走 sidecar，正常账号零影响 |
| **D4 codex 复用方式** | **不 vendor 源码，仅锁依赖版本** | 关键发现：codex 没定制 TLS 配置 |
| **D5 并发模型** | 单进程 + tokio + 共享 reqwest::Client | 50MB 常驻，1000+ 并发轻松 |

## 已决（下次会话开工时已锁定）

| 决策点 | 选择 | 理由 |
|---|---|---|
| **D6 接入策略** | a. 新增并存 (UseSidecar 开关) | utls 留备用，新代码风险隔离 |
| **D7 Sidecar API 形态** | a. JSON 包装 + chunked 响应 | 简单可调试，chunked 自然支持 SSE |

## Sidecar 接入点（已定位）

`backend/internal/repository/http_upstream.go:206 DoWithTLS()` = sub2api 所有出向调用汇聚点。
找到 8+ 处调用方（claude_usage_service / account_test_service / etc.）。
只需在 DoWithTLS 加分支：

```go
if profile != nil && profile.UseSidecar {
    return s.doViaSidecar(req, profile.SidecarURL)  // 新增
}
// 原 utls 路径
```

---

## 设计大纲（详细在 design.md）

```
┌─ sub2api (Go) ──────────┐
│  account.extra:         │
│    use_tls_sidecar:true │
│    sidecar_url: ...     │  ──── HTTP ────►  ┌─ codex-sidecar (Rust) ──┐
│  ↓                      │                   │  axum :8888             │
│  outboundProxy 改成     │                   │  ↓                      │
│  http://codex-sidecar   │                   │  reqwest::Client (默认) │
│    :8888                │                   │  ↓ rustls + aws-lc-rs   │
└─────────────────────────┘                   └─ TLS → 上游 ────────────┘
                                                       │
                                                       ▼
                                              new.sharedchat.cc/codex
                                              (ja3 = 27718d56...)
```

---

## 实施清单（详细在 implement.md）

### Phase 1: Rust sidecar 实现（0.5 天）
- [ ] 新建 `backend/sidecars/codex-tls-sidecar/` 子项目（Cargo workspace 或独立 repo）
- [ ] Cargo.toml 锁 codex 同款依赖
- [ ] `main.rs`: 安装 aws_lc_rs provider + axum server
- [ ] handler: 透明转发（method/headers/body 全保留）
- [ ] SSE 流式转发（reqwest stream → axum response）
- [ ] 健康检查端点 `/health`

### Phase 2: 容器化（0.5 天）
- [ ] Dockerfile: 多阶段构建（rust:1.83-slim → debian:bookworm-slim）
- [ ] docker-compose.yml 加 `codex-sidecar` service（内网 :8888）
- [ ] 资源限制：512MB / 0.5 CPU
- [ ] healthcheck

### Phase 3: sub2api 接入（0.5-1 天）
- [ ] `accounts.extra` 加字段：`use_tls_sidecar: bool` + `sidecar_url: string`
- [ ] `tlsfingerprint/dialer.go` 或新文件：检测到 `use_tls_sidecar` 时改走 sidecar
- [ ] 前端：账号配置 UI 加开关（在 TLS 指纹卡片下方）
- [ ] e2e 测试：抓包验证 ja3 正确

### Phase 4: 验证 + 部署（0.5 天）
- [ ] sub2api-sg 上抓包对照 ja3 字节级匹配
- [ ] 账号 47 schedulable=true 重启
- [ ] 调 sharedchat 验证不再 403
- [ ] 监控日志看 sidecar 健康度

---

## 风险与未知

### 风险 1: HTTP/2 帧序差异
- ja3 是 ClientHello 字节级匹配，但 sharedchat 可能也校验 HTTP/2 SETTINGS 帧
- **缓解**：sidecar 的 HTTP/2 也走 reqwest 默认 = 走 hyper 默认，跟 codex 一致
- **未知**：codex 的 hyper 版本是否和 reqwest 0.12 transitive 出来的版本一致 → 需要 `cargo tree` 确认

### 风险 2: aws-lc-rs 编译复杂度
- aws-lc-rs 是 Rust binding 到 BoringSSL 的 C 代码，编译需要 cmake + perl + go
- **缓解**：用 `rust:1.83-slim` 作为 builder，`apt install cmake perl golang` 即可
- **测试**：在本地 Windows 也要能 cross-compile linux/amd64（用 rust target add）

### 风险 3: sub2api 多账号路由
- 不同账号可能配置不同 sidecar URL（多 region）
- **缓解**：account.extra.sidecar_url 默认指向 `codex-sidecar:8888`，可覆盖

### 风险 4: codex 依赖跟版
- codex 升级 reqwest/rustls/aws-lc-rs 后 ja3 可能变
- **缓解**：codex_version_service 已有版本监控；扩展为对比 sidecar 编译时锁的版本
- **告警**：sidecar 启动时打印实际 reqwest/rustls 版本，方便对比

---

## 不在范围

- ❌ ChatGPT cookie store（codex-client 里的 cloudflare cookie 处理）— sub2api 不用
- ❌ 自定义 CA 加载（CODEX_CA_CERTIFICATE / SSL_CERT_FILE）— 标准 CA 即可
- ❌ codex 业务逻辑（认证 / token 刷新 / SSE 重试）— 还在 sub2api 那边
- ❌ Windows 部署 sidecar — 只支持 Linux 容器
- ❌ 独立伸缩（多 sidecar 实例）— 单实例满足 1000+ 并发即可

---

## 参考

- 上一任务 PRD: `.trellis/tasks/06-20-06-20-sub2api-sharedchat-ja3/prd.md`
- 真实 codex 抓包：`/tmp/codex-real.pcap` 在 sub2api-sg
- profile #1（旧 utls 失败配置，留作字节对照）：PG `tls_fingerprint_profiles.id=1`
- codex 源码探索：
  - `codex-rs/codex-client/src/default_client.rs` (6 KB)
  - `codex-rs/codex-client/src/custom_ca.rs` (34 KB)
  - `codex-rs/utils/rustls-provider/src/lib.rs` (1.5 KB) ← 关键
  - `codex-rs/Cargo.toml` workspace dependencies
