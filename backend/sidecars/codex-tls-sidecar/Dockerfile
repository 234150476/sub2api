FROM rust:1.83-slim AS builder

# aws-lc-sys 编译需要 cmake + perl + golang
RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake \
    perl \
    golang \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

COPY Cargo.toml Cargo.lock* ./
COPY src ./src

RUN cargo build --release --locked || cargo build --release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 1000 --no-create-home sidecar

COPY --from=builder /build/target/release/codex-tls-sidecar /usr/local/bin/codex-tls-sidecar

USER sidecar
EXPOSE 8888
ENV SIDECAR_BIND=0.0.0.0:8888
ENV RUST_LOG=codex_tls_sidecar=info

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8888/health || exit 1

ENTRYPOINT ["/usr/local/bin/codex-tls-sidecar"]
