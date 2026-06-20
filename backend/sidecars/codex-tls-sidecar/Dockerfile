FROM rust:1.96-alpine AS builder

# musl + aws-lc-sys 编译需要 cmake + perl + go + make
RUN apk add --no-cache cmake perl go make musl-dev pkgconfig openssl-dev

WORKDIR /build

COPY Cargo.toml Cargo.lock* ./
COPY src ./src

# 用 musl target，对齐 codex 真实编译产物
RUN cargo build --release --target x86_64-unknown-linux-musl --locked || cargo build --release --target x86_64-unknown-linux-musl

FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl \
    && adduser -D -u 1000 sidecar

COPY --from=builder /build/target/x86_64-unknown-linux-musl/release/codex-tls-sidecar /usr/local/bin/codex-tls-sidecar
COPY --from=builder /build/target/x86_64-unknown-linux-musl/release/probe /usr/local/bin/codex-tls-probe

USER sidecar
EXPOSE 8888
ENV SIDECAR_BIND=0.0.0.0:8888
ENV RUST_LOG=codex_tls_sidecar=info

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8888/health || exit 1

ENTRYPOINT ["/usr/local/bin/codex-tls-sidecar"]
