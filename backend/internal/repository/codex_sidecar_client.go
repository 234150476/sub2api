package repository

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// codexSidecarClient 通过 Rust sidecar 转发出向请求，用于让 ja3 字节级匹配真实 codex CLI。
// sidecar URL 由全局 ENV CODEX_SIDECAR_URL 提供，留空时禁用。
type codexSidecarClient struct {
	url    string
	client *http.Client
}

var (
	codexSidecarOnce     sync.Once
	codexSidecarSingleton *codexSidecarClient
)

func getCodexSidecarClient() *codexSidecarClient {
	codexSidecarOnce.Do(func() {
		url := strings.TrimRight(os.Getenv("CODEX_SIDECAR_URL"), "/")
		if url == "" {
			return
		}
		codexSidecarSingleton = &codexSidecarClient{
			url: url,
			client: &http.Client{
				Timeout: 0,
				Transport: &http.Transport{
					MaxIdleConns:        defaultMaxIdleConns,
					MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
					IdleConnTimeout:     defaultIdleConnTimeout,
				},
			},
		}
	})
	return codexSidecarSingleton
}

type sidecarProxyRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	BodyB64   string            `json:"body_b64,omitempty"`
	TimeoutMs uint64            `json:"timeout_ms,omitempty"`
}

// doViaSidecar 把 req 序列化后 POST 到 Rust sidecar，sidecar 用 codex 同款 reqwest 重发，
// 把上游 response 流式透传回来。
func (s *httpUpstreamService) doViaSidecar(req *http.Request) (*http.Response, error) {
	sc := getCodexSidecarClient()
	if sc == nil {
		return nil, fmt.Errorf("codex sidecar not configured (CODEX_SIDECAR_URL is empty)")
	}

	bodyBytes, err := readRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	headers := make(map[string]string, len(req.Header))
	for k, vs := range req.Header {
		if len(vs) == 0 {
			continue
		}
		headers[k] = vs[0]
	}

	payload := sidecarProxyRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headers,
	}
	if len(bodyBytes) > 0 {
		payload.BodyB64 = base64.StdEncoding.EncodeToString(bodyBytes)
	}
	if req.Context() != nil {
		if dl, ok := req.Context().Deadline(); ok {
			remaining := time.Until(dl)
			if remaining > 0 {
				payload.TimeoutMs = uint64(remaining.Milliseconds())
			}
		}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal sidecar payload: %w", err)
	}

	sidecarReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, sc.url+"/proxy", bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("build sidecar request: %w", err)
	}
	sidecarReq.Header.Set("Content-Type", "application/json")

	slog.Debug("codex_sidecar_request", "url", payload.URL, "method", payload.Method, "body_size", len(bodyBytes))

	resp, err := sc.client.Do(sidecarReq)
	if err != nil {
		return nil, fmt.Errorf("call sidecar: %w", err)
	}

	resp.Request = req
	return resp, nil
}

func readRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	defer req.Body.Close()
	return io.ReadAll(req.Body)
}
