use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;

use axum::body::Body;
use axum::extract::State;
use axum::http::{HeaderMap, HeaderName, HeaderValue, Method, StatusCode};
use axum::response::Response;
use axum::routing::{get, post};
use axum::Json;
use base64::Engine;
use bytes::Bytes;
use futures_util::TryStreamExt;
use serde::{Deserialize, Serialize};
use tracing::{error, info, warn};

mod codex_vendored;

use codex_vendored::login_default_client::build_reqwest_client;
use codex_vendored::rustls_provider::ensure_rustls_crypto_provider;

#[derive(Clone)]
struct AppState {
    client: reqwest::Client,
}

#[derive(Debug, Deserialize)]
struct ProxyRequest {
    method: String,
    url: String,
    #[serde(default)]
    headers: HashMap<String, String>,
    #[serde(default)]
    body_b64: Option<String>,
    #[serde(default)]
    timeout_ms: Option<u64>,
}

#[derive(Debug, Serialize)]
struct ErrorBody {
    error: String,
    detail: String,
}

#[tokio::main]
async fn main() {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "codex_tls_sidecar=info,tower_http=info".into()),
        )
        .init();

    // 安装 codex 同款 rustls crypto provider（aws_lc_rs）—— ja3 灵魂
    ensure_rustls_crypto_provider();

    let client = build_reqwest_client();

    let state = Arc::new(AppState { client });

    let app = axum::Router::new()
        .route("/health", get(health))
        .route("/proxy", post(proxy))
        .with_state(state);

    let bind: SocketAddr = std::env::var("SIDECAR_BIND")
        .unwrap_or_else(|_| "0.0.0.0:8888".to_string())
        .parse()
        .expect("invalid SIDECAR_BIND");

    info!(%bind, "codex-tls-sidecar listening");

    let listener = tokio::net::TcpListener::bind(bind)
        .await
        .expect("bind listener");
    axum::serve(listener, app)
        .await
        .expect("axum serve");
}

async fn health() -> &'static str {
    "ok"
}

async fn proxy(
    State(state): State<Arc<AppState>>,
    Json(req): Json<ProxyRequest>,
) -> Response {
    let method = match Method::from_bytes(req.method.as_bytes()) {
        Ok(m) => m,
        Err(e) => return error_response(StatusCode::BAD_REQUEST, "bad_method", &e.to_string()),
    };

    let body_bytes = match req.body_b64.as_deref() {
        Some(b64) => match base64::engine::general_purpose::STANDARD.decode(b64) {
            Ok(v) => Bytes::from(v),
            Err(e) => {
                return error_response(StatusCode::BAD_REQUEST, "bad_body_b64", &e.to_string())
            }
        },
        None => Bytes::new(),
    };

    let mut builder = state.client.request(method, &req.url);
    for (k, v) in req.headers.iter() {
        let name = match HeaderName::from_bytes(k.as_bytes()) {
            Ok(n) => n,
            Err(_) => {
                warn!(header = %k, "skip invalid header name");
                continue;
            }
        };
        let value = match HeaderValue::from_str(v) {
            Ok(val) => val,
            Err(_) => {
                warn!(header = %k, "skip invalid header value");
                continue;
            }
        };
        builder = builder.header(name, value);
    }
    if !body_bytes.is_empty() {
        builder = builder.body(body_bytes);
    }
    if let Some(ms) = req.timeout_ms {
        builder = builder.timeout(std::time::Duration::from_millis(ms));
    }

    let upstream = match builder.send().await {
        Ok(r) => r,
        Err(e) => {
            error!(?e, url = %req.url, "upstream request failed");
            return error_response(
                StatusCode::BAD_GATEWAY,
                "upstream_request_failed",
                &e.to_string(),
            );
        }
    };

    let status = upstream.status();
    let mut resp_headers = HeaderMap::new();
    for (k, v) in upstream.headers().iter() {
        if matches!(
            k.as_str(),
            "content-length" | "transfer-encoding" | "connection"
        ) {
            continue;
        }
        resp_headers.insert(k.clone(), v.clone());
    }

    let stream = upstream
        .bytes_stream()
        .map_err(|e| std::io::Error::new(std::io::ErrorKind::Other, e));
    let body = Body::from_stream(stream);

    let mut response = Response::new(body);
    *response.status_mut() = StatusCode::from_u16(status.as_u16())
        .unwrap_or(StatusCode::INTERNAL_SERVER_ERROR);
    *response.headers_mut() = resp_headers;
    response
}

fn error_response(status: StatusCode, error: &str, detail: &str) -> Response {
    let body = ErrorBody {
        error: error.to_string(),
        detail: detail.to_string(),
    };
    let json = serde_json::to_vec(&body).unwrap_or_else(|_| b"{}".to_vec());
    let mut resp = Response::new(Body::from(json));
    *resp.status_mut() = status;
    resp.headers_mut().insert(
        axum::http::header::CONTENT_TYPE,
        HeaderValue::from_static("application/json"),
    );
    resp
}
