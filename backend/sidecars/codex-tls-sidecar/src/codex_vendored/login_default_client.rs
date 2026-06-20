// Vendored from codex-rs/login/src/auth/default_client.rs
// Source: https://github.com/openai/codex (commit fetched 2026-06-20)
// License: see codex repo (Apache-2.0)
//
// 改动说明（A2 中量 vendor）：
//   - 移除 ChatGPT cookie jar (sub2api 不调 chat.openai.com)
//   - 移除 custom CA 加载 (CODEX_CA_CERTIFICATE / SSL_CERT_FILE，sub2api 用标准 CA)
//   - 移除 OpenTelemetry 注入
//   - 移除 sandboxed proxy 检测 (CODEX_SANDBOX=seatbelt 不适用)
//   - 移除 ResidencyRequirement / Originator / UA suffix 全局静态量
//   - 保留：build_reqwest_client + default_headers + originator/UA 字符串拼装结构
//
// ja3 决定逻辑保持等同：reqwest::Client::builder().default_headers(...).build()
// + ensure_rustls_crypto_provider() (在 main.rs 启动时调用)

use reqwest::header::HeaderMap;
use reqwest::header::HeaderValue;
use reqwest::header::USER_AGENT;

pub const DEFAULT_ORIGINATOR: &str = "codex_cli_rs";

/// Create the default Codex reqwest client used for outbound HTTP traffic.
///
/// 等同 codex 的 build_reqwest_client：使用 reqwest 0.12 + rustls 0.23 + aws-lc-rs
/// 默认值，由依赖锁版本决定 ClientHello 字节布局（ja3）。
pub fn build_reqwest_client() -> reqwest::Client {
    try_build_reqwest_client().unwrap_or_else(|error| {
        tracing::warn!(error = %error, "failed to build default reqwest client; falling back");
        reqwest::Client::new()
    })
}

pub fn try_build_reqwest_client() -> Result<reqwest::Client, reqwest::Error> {
    reqwest::Client::builder()
        .default_headers(default_headers())
        .build()
}

/// Build the default headers attached to every outbound request.
///
/// 注：当 sidecar 作为透明代理时，sub2api 自己会传递 originator / User-Agent，
/// 这里设的是 sidecar 主动调上游时（罕见场景）的 fallback。
pub fn default_headers() -> HeaderMap {
    let mut headers = HeaderMap::new();
    headers.insert(
        "originator",
        HeaderValue::from_static(DEFAULT_ORIGINATOR),
    );
    if let Ok(user_agent) = HeaderValue::from_str(&get_codex_user_agent()) {
        headers.insert(USER_AGENT, user_agent);
    }
    headers
}

/// Build the codex-style User-Agent string.
///
/// 简化版：codex 原版含 OS / arch / terminal 信息，sidecar 透明代理时这些由
/// sub2api 透传，此处仅做 fallback。
pub fn get_codex_user_agent() -> String {
    let build_version = env!("CARGO_PKG_VERSION");
    format!("{DEFAULT_ORIGINATOR}/{build_version}")
}
