package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// version is the build version, injected via -ldflags "-X main.version=...".
var version = "dev"

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
	cfgPath := flag.String("c", "config/config.yml", "path to server config yml")
	showVer := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Printf("hayabusa-quiz-server %s (protocol %s)\n", version, ProtocolVersion)
		return
	}

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
	log.Printf("hayabusa-quiz-server %s | protocol %s", version, ProtocolVersion)
	log.Printf("loaded %d questions | mode=%s | waiting for %d agents", len(qs), cfg.Mode, cfg.AgentCount)

	s := &Server{cfg: cfg, qs: qs, hub: NewHub()}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)          // agents (players)
	mux.HandleFunc("/viewer", s.handleViewer)  // spectators (read-only WS)
	mux.HandleFunc("/healthz", s.handleHealth) // liveness
	mux.HandleFunc("/readyz", s.handleHealth)  // readiness
	mux.HandleFunc("/", s.handleIndex)         // short hint

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	// graceful shutdown: stop accepting new connections on SIGINT/SIGTERM,
	// let the in-flight game finish, then exit.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Printf("shutdown signal received; draining...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("listening on %s | agents: ws /ws | viewers: ws /viewer | health: /healthz", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Printf("server stopped")
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

// handleHealth is a liveness/readiness probe (always 200 while serving).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"status":"ok","version":%q,"protocol":%q}`+"\n", version, ProtocolVersion)
}

// handleIndex returns a short hint (the viewer is a separate static app).
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("hayabusa-quiz-server\n  agents : ws /ws\n  viewers: ws /viewer\n  health : /healthz\n観戦は viewer アプリを /viewer に接続してください。\n"))
}
