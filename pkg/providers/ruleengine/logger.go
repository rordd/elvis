package ruleengine

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// InteractionLog represents a single logged interaction.
type InteractionLog struct {
	Timestamp string     `json:"timestamp"`
	UserInput string     `json:"user_input"`
	Response  string     `json:"response"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Intent    string     `json:"intent,omitempty"`
}

// InteractionLogger appends interaction logs to a JSONL file.
type InteractionLogger struct {
	mu      sync.Mutex
	logFile string
}

// NewInteractionLogger creates a new logger that writes to the given file path.
func NewInteractionLogger(logFile string) *InteractionLogger {
	return &InteractionLogger{logFile: logFile}
}

// Log appends an interaction entry to the log file.
func (l *InteractionLogger) Log(userInput, response string, toolCalls []ToolCall, intent string) {
	entry := InteractionLog{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		UserInput: userInput,
		Response:  response,
		ToolCalls: toolCalls,
		Intent:    intent,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		log.Printf("ruleengine logger: failed to marshal log entry: %v", err)
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("ruleengine logger: failed to open log file: %v", err)
		return
	}
	defer f.Close()

	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		log.Printf("ruleengine logger: failed to write log entry: %v", err)
	}
}
