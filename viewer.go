package main

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// ViewEvent is a read-only spectator event pushed to viewers (JSON).
type ViewEvent struct {
	Type       string         `json:"type"`
	QuestionID int            `json:"question_id,omitempty"`
	Total      int            `json:"total,omitempty"`
	Cursor     int            `json:"cursor,omitempty"`
	Text       string         `json:"text,omitempty"`
	Agent      string         `json:"agent,omitempty"`
	Reply      string         `json:"reply,omitempty"`
	Correct    bool           `json:"correct,omitempty"`
	Answer     string         `json:"answer,omitempty"`
	Scores     map[string]int `json:"scores,omitempty"` // per-question points
	Totals     map[string]int `json:"totals,omitempty"` // cumulative totals
	Agents     []string       `json:"agents,omitempty"`
}

// Hub fans out spectator events to all connected viewers. Viewers are
// write-only (they never affect the game) and never block it.
type Hub struct {
	mu      sync.Mutex
	viewers map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{viewers: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) Add(conn *websocket.Conn) {
	h.mu.Lock()
	h.viewers[conn] = struct{}{}
	n := len(h.viewers)
	h.mu.Unlock()
	log.Printf("viewer connected (%d watching)", n)
}

func (h *Hub) Broadcast(e ViewEvent) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.viewers {
		if err := c.WriteMessage(websocket.TextMessage, b); err != nil {
			delete(h.viewers, c)
			c.Close()
		}
	}
}
