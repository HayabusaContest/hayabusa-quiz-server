package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server accepts agent connections and starts one game once agent_count agents
// have connected.
type Server struct {
	cfg     *Config
	qs      []Question
	hub     *Hub
	mu      sync.Mutex
	waiting []*Agent
	started bool
}

func main() {
	cfgPath := flag.String("c", "config.yml", "path to server config yml")
	flag.Parse()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	qs, err := LoadQuestions(cfg.Questions)
	if err != nil {
		log.Fatalf("questions error: %v", err)
	}
	if len(qs) == 0 {
		log.Fatalf("no questions loaded from %s", cfg.Questions)
	}
	log.Printf("loaded %d questions | mode=%s | waiting for %d agents", len(qs), cfg.Mode, cfg.AgentCount)

	s := &Server{cfg: cfg, qs: qs, hub: NewHub()}
	http.HandleFunc("/ws", s.handleWS)         // agents (players)
	http.HandleFunc("/viewer", s.handleViewer) // spectators (read-only WS)
	http.HandleFunc("/", s.handleIndex)        // short hint

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("listening on %s | agents: ws /ws | viewers: ws /viewer", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade error: %v", err)
		return
	}
	agent := NewAgent(conn)

	// NAME handshake: server asks, agent replies with its name.
	if err := agent.SendJSON(Packet{Request: ReqName}); err != nil {
		conn.Close()
		return
	}
	name, err := agent.Recv(30 * time.Second)
	if err != nil || name == "" {
		log.Printf("NAME handshake failed: %v", err)
		conn.Close()
		return
	}
	agent.Name = name

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		log.Printf("game already started; rejecting %s", name)
		conn.Close()
		return
	}
	s.waiting = append(s.waiting, agent)
	n := len(s.waiting)
	log.Printf("agent connected: %s (%d/%d)", name, n, s.cfg.AgentCount)
	if n < s.cfg.AgentCount {
		s.mu.Unlock()
		return // keep waiting; the connection stays open and is used once the game starts
	}
	s.started = true
	agents := s.waiting
	s.mu.Unlock()

	log.Printf("all %d agents connected; starting game", len(agents))
	NewGame(s.cfg, agents, s.qs, s.hub).Run()
	for _, a := range agents {
		a.Close()
	}
	// 常駐:次の N エージェントを受け付ける(大会サーバ用)
	s.mu.Lock()
	s.started = false
	s.waiting = nil
	s.mu.Unlock()
	log.Printf("game over; waiting for the next %d agents", s.cfg.AgentCount)
}

// handleViewer registers a read-only spectator connection.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("viewer upgrade error: %v", err)
		return
	}
	s.hub.Add(conn)
	// Viewers are write-only; we don't read from them. The connection stays
	// open and receives spectator events via the hub.
}

// handleIndex returns a short hint (the viewer is a separate static app).
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("arena game server\n  agents : ws /ws\n  viewers: ws /viewer\n観戦は viewer アプリを /viewer に接続してください。\n"))
}
