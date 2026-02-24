# Task: Implement WebSocket Channel for PicoClaw

## Context
PicoClaw channels are in pkg/channels/. Each channel implements the Channel interface from base.go.
The channel manager in manager.go registers and starts channels.
Look at telegram.go as a reference for how channels work (receive messages via channel-specific protocol, publish to bus, send responses).

## What to build

### 1. pkg/channels/websocket.go
Implement a WebSocket channel that:
- Implements Channel interface (Name, Start, Stop, Send, IsRunning, IsAllowed)
- Starts an HTTP server with WebSocket upgrade on configured port
- Accepts WebSocket connections from web UI clients
- Receives JSON messages: {"type": "message", "content": "text", "session_id": "optional"}
- Publishes received messages to bus.MessageBus as inbound messages
- Sends responses back via WebSocket: {"type": "response", "content": "text", "session_id": "id", "source": "provider_name"}
- Supports streaming: {"type": "stream", "content": "chunk", "done": false/true}
- Handles multiple concurrent connections
- Uses gorilla/websocket (already a dependency in go.mod)

### 2. pkg/config/config.go changes
Add WebSocket channel config to ChannelsConfig:
```go
WebSocket WebSocketChannelConfig `json:"websocket"`
```
WebSocketChannelConfig struct:
- Enabled bool
- Host string (default "0.0.0.0")  
- Port int (default 8081)
- AllowOrigins []string
- AllowFrom []string (sender whitelist, same as other channels)

### 3. pkg/channels/manager.go changes
Register the websocket channel in the manager initialization (where telegram, discord etc are registered).

### 4. Static file serving (optional but nice)
Serve a simple chat UI at GET / on the same port. Create a minimal embedded HTML file:
- pkg/channels/websocket_ui.html (embed via go:embed)
- Simple chat interface: message bubbles, input box, send button
- Auto-connects to ws://localhost:PORT/ws
- Clean dark theme, mobile-friendly
- Shows "source" label on responses (ruleengine vs cloud)

### 5. Tests
- pkg/channels/websocket_test.go - basic connection/message tests

### 6. Build & verify
- go generate ./cmd/picoclaw/ && go build -o picoclaw-elvis ./cmd/picoclaw

### 7. Git commit
- git add -A && git commit -m "feat: add websocket channel with embedded chat UI"

## Important notes
- Use gorilla/websocket (already in go.mod)
- Follow the same patterns as telegram.go for bus integration
- The bus.InboundMessage and bus.OutboundMessage types are defined in pkg/bus/
- Read pkg/bus/bus.go to understand message types
- CORS: allow all origins by default for dev
