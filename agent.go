package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Agent wraps one connected player's WebSocket connection plus per-game state.
type Agent struct {
	Name string
	conn *websocket.Conn
	wmu  sync.Mutex

	// per-question state (reset each question)
	Committed bool // already gave a non-pass answer this question (one-shot)
	LockedOut bool // wrong answer this question -> out
	CorrectAt int  // 1-indexed cursor of the first correct answer; 0 if none

	// cumulative
	Score int
}

func NewAgent(conn *websocket.Conn) *Agent {
	return &Agent{conn: conn}
}

// SendJSON marshals and sends a server->agent packet.
func (a *Agent) SendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.wmu.Lock()
	defer a.wmu.Unlock()
	return a.conn.WriteMessage(websocket.TextMessage, b)
}

// Recv reads one string reply from the agent with a timeout.
func (a *Agent) Recv(timeout time.Duration) (string, error) {
	_ = a.conn.SetReadDeadline(time.Now().Add(timeout))
	_, b, err := a.conn.ReadMessage()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (a *Agent) Close() {
	_ = a.conn.Close()
}
