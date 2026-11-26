package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// fakeFirstChatChunkCount 连发两个空开头包：下游网关（如 CodeGO）会把每个 chunk
// 压到下一个 chunk 到达时才转发给客户端，只发一个会被压到真实内容到达时才出去。
const fakeFirstChatChunkCount = 2

// writeFakeFirstChatChunks 在 Forward 前写入标准的 chat.completion.chunk 开头包
// （role=assistant、content 为空），不改变最终回答内容。
func writeFakeFirstChatChunks(c *gin.Context, model string) {
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	payload, _ := json.Marshal(map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{"role": "assistant", "content": ""},
			"finish_reason": nil,
		}},
	})
	line := "data: " + string(payload) + "\n\n"
	for i := 0; i < fakeFirstChatChunkCount; i++ {
		_, _ = c.Writer.WriteString(line)
	}
	c.Writer.Flush()
}
