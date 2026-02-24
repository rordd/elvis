package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

func startTestWSChannel(t *testing.T, port int) (*WebSocketChannel, *bus.MessageBus) {
	t.Helper()
	mb := bus.NewMessageBus()
	cfg := config.WebSocketChannelConfig{
		Enabled:      true,
		Host:         "127.0.0.1",
		Port:         port,
		AllowOrigins: []string{"*"},
	}
	ch, err := NewWebSocketChannel(cfg, mb)
	if err != nil {
		t.Fatalf("NewWebSocketChannel: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := ch.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait for server to be ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			conn.Body.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return ch, mb
}

func dialTestWS(t *testing.T, port int) *websocket.Conn {
	t.Helper()
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestWebSocketConnect(t *testing.T) {
	ch, _ := startTestWSChannel(t, 18900)
	defer ch.Stop(context.Background())

	conn := dialTestWS(t, 18900)

	// Should receive a "connected" message with session_id
	var msg wsOutboundMsg
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if msg.Type != "connected" {
		t.Fatalf("expected type 'connected', got %q", msg.Type)
	}
	if msg.SessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
}

func TestWebSocketSendReceive(t *testing.T) {
	ch, mb := startTestWSChannel(t, 18901)
	defer ch.Stop(context.Background())

	conn := dialTestWS(t, 18901)

	// Read the connected message to get session ID
	var connMsg wsOutboundMsg
	if err := conn.ReadJSON(&connMsg); err != nil {
		t.Fatalf("ReadJSON connected: %v", err)
	}
	sessionID := connMsg.SessionID

	// Send a message
	outMsg := wsInboundMsg{
		Type:      "message",
		Content:   "hello world",
		SessionID: sessionID,
	}
	if err := conn.WriteJSON(outMsg); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	// Consume from bus
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	inbound, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if inbound.Content != "hello world" {
		t.Fatalf("expected content 'hello world', got %q", inbound.Content)
	}
	if inbound.Channel != "websocket" {
		t.Fatalf("expected channel 'websocket', got %q", inbound.Channel)
	}
	if inbound.ChatID != sessionID {
		t.Fatalf("expected chat_id %q, got %q", sessionID, inbound.ChatID)
	}
}

func TestWebSocketSendResponse(t *testing.T) {
	ch, _ := startTestWSChannel(t, 18902)
	defer ch.Stop(context.Background())

	conn := dialTestWS(t, 18902)

	// Read connected message
	var connMsg wsOutboundMsg
	if err := conn.ReadJSON(&connMsg); err != nil {
		t.Fatalf("ReadJSON connected: %v", err)
	}
	sessionID := connMsg.SessionID

	// Send a response via the channel
	err := ch.Send(context.Background(), bus.OutboundMessage{
		Channel: "websocket",
		ChatID:  sessionID,
		Content: "hello back",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read the response
	var resp wsOutboundMsg
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("ReadJSON response: %v", err)
	}
	if resp.Type != "response" {
		t.Fatalf("expected type 'response', got %q", resp.Type)
	}
	if resp.Content != "hello back" {
		t.Fatalf("expected content 'hello back', got %q", resp.Content)
	}
}

func TestWebSocketChatUI(t *testing.T) {
	ch, _ := startTestWSChannel(t, 18903)
	defer ch.Stop(context.Background())

	resp, err := http.Get("http://127.0.0.1:18903/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Fatalf("expected text/html, got %q", ct)
	}
}

func TestWebSocketEmptyMessageIgnored(t *testing.T) {
	ch, mb := startTestWSChannel(t, 18904)
	defer ch.Stop(context.Background())

	conn := dialTestWS(t, 18904)

	// Read connected
	var connMsg wsOutboundMsg
	conn.ReadJSON(&connMsg)

	// Send empty message
	msg, _ := json.Marshal(wsInboundMsg{Type: "message", Content: ""})
	conn.WriteMessage(websocket.TextMessage, msg)

	// Bus should not receive anything
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, ok := mb.ConsumeInbound(ctx)
	if ok {
		t.Fatal("expected no inbound message for empty content")
	}
}
