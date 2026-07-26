package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// version is the build version, injected via -ldflags "-X main.version=...".
var version = "dev"

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server accepts agent connections, forms games in a waiting room, and runs
// them concurrently (each batch of agent_count agents becomes its own game).
type Server struct {
	cfg  *Config
	qs   []Question
	mgr  *GameManager
	room *WaitingRoom
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
	log.Printf("loaded %d questions | mode=%s | agent_count=%d (games run concurrently)", len(qs), cfg.Mode, cfg.AgentCount)

	s := &Server{cfg: cfg, qs: qs, mgr: NewGameManager(), room: NewWaitingRoom(cfg.AgentCount)}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)           // agents (players)
	mux.HandleFunc("/viewer", s.handleViewer)   // spectators (read-only WS); ?game=<id>
	mux.HandleFunc("GET /games", s.handleGames) // active games list
	mux.HandleFunc("GET /games/{id}", s.handleGameByID)
	mux.HandleFunc("/healthz", s.handleHealth) // liveness
	mux.HandleFunc("/readyz", s.handleHealth)  // readiness
	mux.HandleFunc("/", s.handleIndex)         // short hint

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	// graceful shutdown: stop accepting new connections on SIGINT/SIGTERM.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Printf("shutdown signal received; draining...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Printf("listening on %s | agents: ws /ws | viewers: ws /viewer | games: GET /games | health: /healthz", addr)
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
	if !tokenOK(r, s.cfg.Auth.PlayerToken) {
		log.Printf("agent rejected: invalid player token")
		conn.Close()
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

	batch, ready := s.room.Add(agent)
	log.Printf("agent connected: %s (waiting %d/%d)", name, s.room.Waiting(), s.cfg.AgentCount)
	if !ready {
		return // connection stays open (hijacked); used once its batch fills
	}

	id := s.mgr.NewID()
	game := NewGame(s.cfg, batch, s.qs, id)
	s.mgr.Register(game)
	log.Printf("[%s] starting game with %d agents", id, len(batch))
	go func() {
		game.Run()
		for _, a := range batch {
			a.Close()
		}
		s.mgr.Unregister(id)
		log.Printf("[%s] game removed (%d active)", id, s.mgr.ActiveCount())
	}()
}

// handleViewer registers a read-only spectator connection to a game's hub.
// ?game=<id> selects a game; without it the newest active game is used.
func (s *Server) handleViewer(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("viewer upgrade error: %v", err)
		return
	}
	if !tokenOK(r, s.cfg.Auth.ReceiverToken) {
		conn.Close()
		return
	}
	var g *Game
	if id := r.URL.Query().Get("game"); id != "" {
		g = s.mgr.Get(id)
	} else {
		g = s.mgr.Newest()
	}
	if g == nil {
		// no active game yet; let the viewer reconnect.
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"waiting"}`))
		conn.Close()
		return
	}
	g.Hub().Add(conn)
	// Viewers are write-only; the connection receives events via the hub.
}

// handleHealth is a liveness/readiness probe (always 200 while serving).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"status":"ok","version":%q,"protocol":%q,"active_games":%d}`+"\n",
		version, ProtocolVersion, s.mgr.ActiveCount())
}

// handleIndex returns a short hint (the viewer is a separate static app).
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("hayabusa-quiz-server\n  agents : ws /ws\n  viewers: ws /viewer?game=<id>\n  games  : GET /games\n  health : /healthz\n観戦は viewer アプリを /viewer に接続してください。\n"))
}
