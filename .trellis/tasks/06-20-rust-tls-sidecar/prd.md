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

## 实施进展（会话 2，2026-06-20 续）

### ✅ 已完成（本地 git，8 commit 累计）

```
865e4c48 feat(sidecar): Rust 透明代理子项目 + codex 源码 vendor (A2 中量)
55ff0c44 feat(tls): sub2api 接入 codex sidecar 透明代理路径
8ec1a727 docs(rust-tls-sidecar): 锁定 D6/D7 决策 + Sidecar 接入点
d5841805 fix(sidecar): musl + rustls-tls + http2 + 加 probe 二进制
```

### 🚨 关键技术发现（颠覆原 brainstorm 假设）

1. **codex CLI 是 npm 包**：`@openai/codex` 0.141.0 → `x86_64-unknown-linux-musl` 静态 PIE 二进制（264 MB ELF）
2. **codex 真包 ja3 = 27718d56688425cd36a401c66147c4ee** 含：
   - 30 个 cipher（**含 TLS_DHE_RSA_*、TLS_RSA_*、*_CBC_***）
   - supported_groups: X25519MLKEM768, X25519, P-256, X448, P-384, P-521, ffdhe2048, ffdhe3072
3. **rustls 0.23 + aws-lc-rs 任何配置都不发 DHE/RSA/CBC cipher**
   → codex 的 reqwest 实际**不使用 rustls**
4. **但 codex 是 musl 静态二进制，不能 OpenSSL/native-tls**
   → codex 用了某种我没找到的 TLS 库
5. **我的 sidecar (musl + rustls 0.23.36 + aws-lc-rs 1.16.2 + http2)**
   ja3 = `e3530ce29f8567df040df290d7b963b1` **不等于 codex**
   - 含 X25519MLKEM768 ✅
   - 但只 10 cipher（TLS 1.3 + ECDHE，无 DHE/RSA/CBC）
   - supported_groups: 只 4 个（X25519, P-256, P-384, MLKEM）

### ❌ 卡点：远端部署失败

```
本地: docker image codex-tls-sidecar:0.1.0-musl 12 MB 已建好
agentsshcli SFTP 30s 硬超时 → 12 MB 文件传不完
唯一绕开：pscp + root 密码 / 用户自己 scp
```

### 🔍 下次会话续接清单

```
1. 解决远端镜像上传（pscp + 密码 / 用户手动 scp / 在远端 git pull build）
2. docker compose up -d codex-sidecar
3. 抓包：tcpdump sidecar→sharedchat ja3
4. 实测 sharedchat 是否字节级严格
5. 如果 403 codex_access_restricted → 反汇编 codex binary 找真实 TLS 库:
   strings /usr/lib/node_modules/@openai/codex/.../bin/codex | grep -E "openssl|rustls|boring|wolfssl|nss"
   objdump -p .../codex | head -30  (看是否 NEEDED 任何 .so)
6. 找到 TLS 库后再决定继续 vendor 路径 / 切 Path 4 / 止损
```

### 决定性事实链（如果下次会话失忆）

```
A. ja3 字节级匹配是难点，不是依赖默认值能解决
B. codex 的 30 cipher (含 DHE/RSA/CBC) + ffdhe + X448 不像任何主流默认 TLS lib
C. 怀疑 codex 用 BoringSSL/wolfSSL/某 fork 的 OpenSSL 静态链入 musl
D. 验证手段：strings /usr/bin/codex (实际路径在 npm 包 vendor 目录)
E. 当前 sidecar 已能工作（流式透传 / health 检查 / Linux musl docker），
   只是 ja3 不等同 codex — 适合宽松检测的公益站，不一定能过 sharedchat
```

## 会话 3 (2026-06-20 续) — 远端实测 + 关键发现

### ✅ 已完成

```
1. strings codex binary 找到关键证据:
   - "OpenSSL 3.5.5 27 Jan 2026"
   - "AWS-LC 1.71.0"
   - "native-tls-0.2.14/src/imp/openssl.rs"
   → codex 实际用 reqwest + native-tls + 静态 OpenSSL 3.5.5

2. sidecar 改用 native-tls-vendored + 锁 openssl-src 到 300.5.5+3.5.5
   → 本地实测 ja3 = 27718d56688425cd36a401c66147c4ee
   = codex CLI 真实 ja3 字节级完全一致 ✅

3. 远端部署成功:
   - pscp 上传 (用本会话获取的 sub2api-sg 凭据，凭据脱敏)
   - docker load + docker run -p 8989:8888
   - 实测出向 ja3 = 27718d56688425cd36a401c66147c4ee ✅
```

### ❌ 关键发现：ja3 一致 ≠ 通过 sharedchat 校验

```
对照实验:
  ① 真 codex CLI 调 sharedchat:           ✅ 返回 "Hi."
  ② sidecar (ja3 = codex) 调 sharedchat: ❌ codex_access_restricted
  ③ 直接 curl 调 sharedchat:              ❌ codex_access_restricted

结论: sharedchat 检测的不只是 ja3 字节
  可能在: HTTP/2 SETTINGS / body signature / cookie / 特殊 header

UA + originator 都对齐后仍 ❌:
  curl + UA "codex_cli_rs/0.141.0..." + originator "codex_cli_rs"
  → codex_access_restricted
```

### 🔍 下次会话续接

```
1. SSLKEYLOGFILE 不工作 (codex 静态 OpenSSL 禁了 keylog)
2. 需要 mitmproxy 截真 codex CLI 请求看实际 path/headers/body
3. 或反汇编 codex binary 找 sharedchat 特殊认证逻辑
4. 或直接放弃 sharedchat，转向其他 ja3-only 严格的公益站

工程结论:
  ja3 sidecar 当前已能用于 ja3-only 检测的公益站
  对 sharedchat 这种"ja3 + 应用层"严格站，仍需补完应用层
```

### 当前状态摘要

```
本地: 11 commit + 镜像 codex-tls-sidecar:0.2.1-openssl-3.5.5 (15 MB)
远端: 镜像已 load，容器 codex-sidecar-test 单独跑在 :8989 (调试)
     未接入 sub2api docker compose
     未启用账号 47

凭据:
  sub2api-sg root: 见 1Password / agent-ssh-cli secrets
  hostkey: 见 ~/.ssh/known_hosts 缓存
  sharedchat token: 见 PG accounts.credentials WHERE id=47 (jsonb)
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

---

## 会话 4 (2026-06-20 晚) — mitmproxy 捕获真 codex body + v7 上线

### 关键发现：sharedchat 还校验 body schema

```
mitmproxy + CODEX_CA_CERTIFICATE 截真 codex CLI 完整请求 body (33 KB)
→ 顶层必须含 11 个 codex 特有字段:
   instructions / tools / reasoning / store / stream / include /
   prompt_cache_key / text / client_metadata / model / input
→ 只发简单 {"input":"hi"} 必被 codex_access_restricted 拒
```

### 实施：body 嫁接（commit 32e15944）

```
新增:
  backend/internal/service/codex_template/body.json  (33 KB 真 codex 模板)
  backend/internal/service/codex_template/template.go (//go:embed)
  backend/internal/service/codex_body_rewriter.go    (嫁接 + UUID 重写)

策略:
  1. 加载模板 (含 14 KB instructions + 11 工具 schema + reasoning effort=medium)
  2. 客户端的 model/input/max_output_tokens merge 进去
  3. session/thread/turn/window/installation UUID 每次重新生成 (防重放)
  4. x-codex-turn-metadata 同步生成 stringified JSON

修改:
  openai_gateway_service.go:buildUpstreamRequest 调 body rewriter
  (仅 inject_codex_headers=true 时启用)

部署: sub2api:traffic-shaping-2026-06-20-v7 (b75... 145 MB)
```

### v7 实测进展

```
旧 v6: codex_access_restricted (body schema 不对)
新 v7: global_fixed_window_quota_exhausted (公益站当日额度耗尽)
     = 完全通过 codex 客户端检测，待额度刷新
```

---

## 会话 5 (2026-06-21 中) — 端到端验证通过 ✅

### 实测命令

```bash
curl -sS -m 150 -o /tmp/v7-resp.json -w 'HTTP=%{http_code} TIME=%{time_total}s' \
  -X POST http://127.0.0.1:8080/v1/responses \
  -H 'Authorization: Bearer sk-test-ed33e4dac92100ff73fb7ccd121d7f39' \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-5.4","input":"Say hi in 3 words.","stream":false}'
```

### 结果

| 项 | 值 |
|---|---|
| HTTP | 200 |
| Body size | 29 243 bytes |
| `status` | `completed`, `error: null` |
| Output text | **"Hi there today"** (3 个词，遵循指令) |
| usage.input_tokens | 11 994 (cached 11 328 → codex 模板 prefix 在上游稳定缓存) |
| usage.output_tokens | 92 (reasoning 78) |
| Account | id=47 sharedchat-公益, schedulable=true |

### 决定性事实

```
最终救活 sharedchat 的 ≠ Rust sidecar
最终救活 sharedchat 的 = sub2api 原生 utls (profile #1) + headers 注入 + body 嫁接
```

**Rust sidecar 容器都没在跑** (sub2api-sg 上无 codex-sidecar 容器)。account 47 也没配 `use_tls_sidecar`，
默认走 sub2api 自身的 utls TLS 指纹 + 应用层注入即可通过 sharedchat。

### model ID 约束（不是白名单，是 ID 合法性）

```
gpt-5.4 ✅ codex CLI 0.141.0 默认模型 ID (codex 系列就是 gpt-5.4/5.5/5.6 这种小版本号)
gpt-5   ❌ 根本不是合法 codex model ID，透传上去公益站不识别 → 503
```

订正：先前一度怀疑公益站做了 model 白名单，实际上是 `gpt-5` 在 codex 体系里不存在
这个 ID。客户端调时必须用 codex 真实模型 ID（gpt-5.4 / gpt-5.5 / ...），
sub2api 不需要做 model rewrite。

### 工程结论

```
1. sharedchat 检测优先级: body schema ≫ headers ≫ ja3 字节
   - utls 1.8.2 不能精确仿 ja3 没关系，应用层补齐即可
   - 这是 sub2api 主仓最大的方向修正

2. Rust sidecar 工程已完成 (~/.../codex-tls-sidecar 子项目，ja3 字节 100% 等同)
   - 当前未启用，但保留为未来"ja3 严格站"的备用路径
   - 如未来撞到只校验 ja3 的站，accounts.extra 加 use_tls_sidecar=true + sidecar_url 即可启用

3. v7 镜像已稳定 (已运行 16 min healthy)
```

### 收尾状态

```
✅ sub2api v7 镜像在远端稳定运行 (healthy 12 天 redis/pg, 16 min sub2api)
✅ 账号 47 schedulable=true, inject_codex_headers=true, enable_tls_fingerprint=true
✅ 测试 key sk-test-ed33e4dac... (group=33 gpt公益) 端到端可用
✅ body 嫁接代码合入 main (commit 32e15944)
⚠️ 本地 15 commit 未推 upstream/main (用户未要求 push)
⚠️ Rust sidecar 容器关闭 (调试用，未删镜像 codex-tls-sidecar:0.2.1-openssl-3.5.5)
```

### Lessons (并入主 PRD)

```
1. 抓真包对照永远是 ja3 / 协议指纹类问题的首选诊断手段
   - 不要在 utls 配置里盲调字段，先 mitmproxy 截真客户端
   - SSLKEYLOGFILE 对静态 OpenSSL 不工作 → 用 mitmproxy + CA injection

2. 检测站点的"严格度"层级分布远超预期:
   ja3 字节 → HTTP/2 SETTINGS → headers → body schema → UUID 重放检测
   越往后越严，但越往后越容易"模板复制" → 不要被 ja3 死锁住，先打通应用层

3. "完成方案 A 之前不否定方案 A，但要保留方案 B" 是对的
   - sidecar (方案 A: ja3 严格) 做完了，但实际上不需要
   - body 嫁接 (方案 B: 应用层) 才是真出路
   - 两个方案都保留代码：未来不同站可能各用一个

4. codex 系列模型 ID 是 gpt-5.4 / gpt-5.5 / ... 这种小版本号，不存在裸 "gpt-5"
   - 测试请求若用 gpt-5 必 503，跟 sharedchat 检测没关系，是 ID 本身不存在
   - sub2api 不需要做 model rewrite，client 自己用合法 codex ID 即可
```
