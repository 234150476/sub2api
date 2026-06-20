// Codex 源码 vendor 子模块
//
// 包含从 https://github.com/openai/codex 抄过来的关键 HTTP/TLS 构造代码。
// 用 A2 中量 vendor 策略：
//   - 保留与 ja3 字节布局相关的所有逻辑（reqwest::Client::builder + rustls aws_lc_rs）
//   - 保留 codex 的 CodexHttpClient / CodexRequestBuilder 包装结构（便于后续 diff upstream）
//   - 移除与 ja3 无关的层（OTel / ChatGPT cookie jar / custom CA / OS info）
//
// 决定 ja3 的真正代码（依赖默认值）：
//   1. ensure_rustls_crypto_provider()                   ← rustls_provider.rs
//   2. reqwest::Client::builder().build()                ← login_default_client.rs
//
// vendor 时间：2026-06-20
// 上游 commit：(查 E:\Temp\codex-source 的 git log)
// 跟版策略：codex 升级 reqwest / rustls / aws-lc-rs 后需手动同步本目录

#![allow(dead_code)]

pub mod default_client;
pub mod login_default_client;
pub mod rustls_provider;
