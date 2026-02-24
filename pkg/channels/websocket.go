package channels

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

//go:embed websocket_ui.html
var wsUIFS embed.FS

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // overridden per-instance in Start
	},
}

// wsInboundMsg is the JSON schema for messages received from WebSocket clients.
type wsInboundMsg struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
}

// wsOutboundMsg is the JSON schema for messages sent to WebSocket clients.
type wsOutboundMsg struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	SessionID string `json:"session_id,omitempty"`
	Source    string `json:"source,omitempty"`
	Done      *bool  `json:"done,omitempty"`
}

type wsConn struct {
	conn      *websocket.Conn
	sessionID string
	mu        sync.Mutex
}

func (c *wsConn) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

type WebSocketChannel struct {
	*BaseChannel
	cfg    config.WebSocketChannelConfig
	server *http.Server
	conns  sync.Map // sessionID -> *wsConn
}

func NewWebSocketChannel(cfg config.WebSocketChannelConfig, messageBus *bus.MessageBus) (*WebSocketChannel, error) {
	base := NewBaseChannel("websocket", cfg, messageBus, cfg.AllowFrom)
	return &WebSocketChannel{
		BaseChannel: base,
		cfg:         cfg,
	}, nil
}

func (c *WebSocketChannel) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve embedded chat UI at /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := wsUIFS.ReadFile("websocket_ui.html")
		if err != nil {
			http.Error(w, "UI not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// WebSocket endpoint
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     c.checkOrigin,
	}

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.ErrorCF("websocket", "Upgrade failed", map[string]any{
				"error": err.Error(),
			})
			return
		}
		c.handleConnection(ctx, conn)
	})

	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	c.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	c.setRunning(true)
	logger.InfoCF("websocket", "WebSocket channel started", map[string]any{
		"addr": addr,
	})

	go func() {
		if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.ErrorCF("websocket", "HTTP server error", map[string]any{
				"error": err.Error(),
			})
		}
	}()

	go func() {
		<-ctx.Done()
		c.Stop(context.Background())
	}()

	return nil
}

func (c *WebSocketChannel) Stop(ctx context.Context) error {
	if !c.IsRunning() {
		return nil
	}
	logger.InfoC("websocket", "Stopping WebSocket channel...")
	c.setRunning(false)

	// Close all connections
	c.conns.Range(func(key, value any) bool {
		if wc, ok := value.(*wsConn); ok {
			wc.conn.Close()
		}
		c.conns.Delete(key)
		return true
	})

	if c.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return c.server.Shutdown(shutdownCtx)
	}
	return nil
}

func (c *WebSocketChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("websocket channel not running")
	}

	out := wsOutboundMsg{
		Type:      "response",
		Content:   msg.Content,
		SessionID: msg.ChatID,
	}

	// Try to send to the specific session
	if v, ok := c.conns.Load(msg.ChatID); ok {
		wc := v.(*wsConn)
		if err := wc.writeJSON(out); err != nil {
			logger.ErrorCF("websocket", "Failed to send to session", map[string]any{
				"session_id": msg.ChatID,
				"error":      err.Error(),
			})
			c.conns.Delete(msg.ChatID)
			wc.conn.Close()
			return err
		}
		return nil
	}

	return fmt.Errorf("session %s not found", msg.ChatID)
}

// SendStream sends a streaming chunk to a specific session.
func (c *WebSocketChannel) SendStream(sessionID, content string, done bool) error {
	out := wsOutboundMsg{
		Type:      "stream",
		Content:   content,
		SessionID: sessionID,
		Done:      &done,
	}

	if v, ok := c.conns.Load(sessionID); ok {
		wc := v.(*wsConn)
		return wc.writeJSON(out)
	}
	return fmt.Errorf("session %s not found", sessionID)
}

func (c *WebSocketChannel) handleConnection(ctx context.Context, conn *websocket.Conn) {
	sessionID := uuid.New().String()
	wc := &wsConn{
		conn:      conn,
		sessionID: sessionID,
	}
	c.conns.Store(sessionID, wc)

	logger.InfoCF("websocket", "New connection", map[string]any{
		"session_id":  sessionID,
		"remote_addr": conn.RemoteAddr().String(),
	})

	// Send session_id to client
	wc.writeJSON(wsOutboundMsg{
		Type:      "connected",
		SessionID: sessionID,
	})

	defer func() {
		c.conns.Delete(sessionID)
		conn.Close()
		logger.InfoCF("websocket", "Connection closed", map[string]any{
			"session_id": sessionID,
		})
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.ErrorCF("websocket", "Read error", map[string]any{
					"session_id": sessionID,
					"error":      err.Error(),
				})
			}
			return
		}

		var inMsg wsInboundMsg
		if err := json.Unmarshal(rawMsg, &inMsg); err != nil {
			logger.ErrorCF("websocket", "Invalid JSON message", map[string]any{
				"session_id": sessionID,
				"error":      err.Error(),
			})
			continue
		}

		if inMsg.Type != "message" {
			continue
		}

		if inMsg.Content == "" {
			continue
		}

		// Use client-provided session_id if given, otherwise use connection session
		chatID := sessionID
		if inMsg.SessionID != "" {
			chatID = inMsg.SessionID
			// Re-map connection under the client-requested session ID
			if chatID != sessionID {
				c.conns.Store(chatID, wc)
			}
		}

		senderID := conn.RemoteAddr().String()

		logger.DebugCF("websocket", "Received message", map[string]any{
			"session_id": chatID,
			"sender":     senderID,
		})

		c.HandleMessage(senderID, chatID, inMsg.Content, nil, map[string]string{
			"session_id":  chatID,
			"remote_addr": conn.RemoteAddr().String(),
		})
	}
}

func (c *WebSocketChannel) checkOrigin(r *http.Request) bool {
	if len(c.cfg.AllowOrigins) == 0 {
		return true
	}
	for _, origin := range c.cfg.AllowOrigins {
		if origin == "*" {
			return true
		}
		if origin == r.Header.Get("Origin") {
			return true
		}
	}
	return false
}
