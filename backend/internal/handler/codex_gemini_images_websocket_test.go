//go:build unit

package handler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesWebSocketImageBridgeEmitsNativeEvents(t *testing.T) {
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: &service.BillingCacheService{},
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{}), SSEPingFormatNone, time.Second),
		cfg:                 &config.Config{},
	}
	h.responsesWSImageBridge = func(_ *gin.Context, payload []byte) ([]byte, map[string]any, *imageBridgeCapture, bool, error) {
		return payload, map[string]any{
			"id":         "resp_bridge_ws",
			"object":     "response",
			"created_at": int64(1710000000),
			"status":     "completed",
			"model":      "gpt-5.4",
			"output": []any{map[string]any{
				"id":            "ig_bridge_ws",
				"type":          "image_generation_call",
				"status":        "completed",
				"result":        "aW1hZ2U=",
				"output_format": "png",
			}},
		}, nil, true, nil
	}

	server := newOpenAIWSHandlerTestServer(t, h, middleware.AuthSubject{UserID: 1, Concurrency: 1})
	defer server.Close()
	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	conn, _, err := coderws.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = conn.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","input":"帮我生成一张图片"}`))
	cancelWrite()
	require.NoError(t, err)

	var eventTypes []string
	sequence := int64(0)
	for {
		readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
		_, payload, readErr := conn.Read(readCtx)
		cancelRead()
		require.NoError(t, readErr)
		require.Equal(t, sequence, gjson.GetBytes(payload, "sequence_number").Int())
		sequence++
		eventType := gjson.GetBytes(payload, "type").String()
		eventTypes = append(eventTypes, eventType)
		if eventType == "response.output_item.done" {
			require.Equal(t, "aW1hZ2U=", gjson.GetBytes(payload, "item.result").String())
		}
		if eventType == "response.completed" {
			require.Equal(t, "resp_bridge_ws", gjson.GetBytes(payload, "response.id").String())
			break
		}
	}
	require.Contains(t, eventTypes, "response.image_generation_call.completed")
	require.Contains(t, eventTypes, "response.completed")

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, closeErr := conn.Read(readCtx)
	cancelRead()
	require.Error(t, closeErr)
	require.Equal(t, coderws.StatusNormalClosure, coderws.CloseStatus(closeErr))
}
