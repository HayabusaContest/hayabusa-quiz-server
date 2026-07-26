package main

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Packet is a server->agent message (JSON). Agents reply with a bare string.
type Packet struct {
	Request    string         `json:"request"`
	QuestionID int            `json:"question_id,omitempty"`
	Cursor     int            `json:"cursor,omitempty"`
	Text       string         `json:"text,omitempty"`
	Answer     string         `json:"answer,omitempty"`
	Scores     map[string]int `json:"scores,omitempty"`
}

// GameSnapshot is a read-only summary of a game for the REST API.
type GameSnapshot struct {
	ID        string         `json:"id"`
	Agents    []string       `json:"agents"`
	StartedAt string         `json:"started_at"`
	Finished  bool           `json:"finished"`
	Scores    map[string]int `json:"scores"`
	Questions int            `json:"questions"`
}

type Game struct {
	ID        string
	cfg       *Config
	agents    []*Agent
	qs        []Question
	rt        time.Duration
	hub       *Hub
	sink      EventSink
	startedAt time.Time

	mu       sync.RWMutex // guards finished/scores (read by REST goroutine)
	finished bool
	scores   map[string]int
}

func NewGame(cfg *Config, agents []*Agent, qs []Question, id string) *Game {
	hub := NewHub()
	return &Game{
		ID:        id,
		cfg:       cfg,
		agents:    agents,
		qs:        qs,
		rt:        time.Duration(cfg.ResponseTimeoutMs) * time.Millisecond,
		hub:       hub,
		sink:      NewComposite(NewHubSink(hub), NewLogSink(cfg.LogDir, id)),
		startedAt: time.Now(),
		scores:    map[string]int{},
	}
}

// Hub returns this game's spectator hub (viewers attach here).
func (g *Game) Hub() *Hub { return g.hub }

// Snapshot returns a read-only summary (safe to call from other goroutines).
func (g *Game) Snapshot() GameSnapshot {
	g.mu.RLock()
	defer g.mu.RUnlock()
	names := make([]string, len(g.agents))
	for i, a := range g.agents {
		names[i] = a.Name
	}
	sc := make(map[string]int, len(g.scores))
	for k, v := range g.scores {
		sc[k] = v
	}
	return GameSnapshot{
		ID: g.ID, Agents: names, StartedAt: g.startedAt.Format(time.RFC3339),
		Finished: g.finished, Scores: sc, Questions: len(g.qs),
	}
}

func (g *Game) setScores(s map[string]int) {
	cp := make(map[string]int, len(s))
	for k, v := range s {
		cp[k] = v
	}
	g.mu.Lock()
	g.scores = cp
	g.mu.Unlock()
}

func (g *Game) broadcast(p Packet) {
	for _, a := range g.agents {
		if err := a.SendJSON(p); err != nil {
			log.Printf("[%s] broadcast to %s failed: %v", g.ID, a.Name, err)
		}
	}
}

// view emits a spectator event to every attached sink (viewer hub, JSONL log, ...).
func (g *Game) view(e ViewEvent) { g.sink.Emit(e) }

// Run plays every question, then broadcasts the final scores.
func (g *Game) Run() {
	defer g.sink.Close()

	names := make([]string, len(g.agents))
	for i, a := range g.agents {
		names[i] = a.Name
	}
	g.view(ViewEvent{Type: ViewGameStart, Agents: names})

	for i, q := range g.qs {
		g.playQuestion(i+1, q)
	}

	final := map[string]int{}
	for _, a := range g.agents {
		final[a.Name] = a.Score
	}
	g.setScores(final)
	g.mu.Lock()
	g.finished = true
	g.mu.Unlock()
	g.broadcast(Packet{Request: ReqFinish, Scores: final})
	g.view(ViewEvent{Type: ViewFinish, Scores: final})
	log.Printf("[%s] === GAME FINISHED === %v", g.ID, final)
}

// playQuestion reveals the question one rune at a time in lockstep, scoring by
// earliest correct. Modes: reveal_all / first_answer / benchmark (see config).
func (g *Game) playQuestion(qid int, q Question) {
	for _, a := range g.agents {
		a.Committed = false
		a.LockedOut = false
		a.CorrectAt = 0
	}

	runes := []rune(q.Text)
	total := len(runes)

	g.broadcast(Packet{Request: ReqQuestionStart, QuestionID: qid})
	g.view(ViewEvent{Type: ViewQuestionStart, QuestionID: qid, Total: total})

	firstAnswer := g.cfg.Mode == ModeFirstAnswer
	benchmark := g.cfg.Mode == ModeBenchmark

	for i := 1; i <= total; i++ {
		var active []*Agent
		for _, a := range g.agents {
			if !a.Committed && !a.LockedOut {
				active = append(active, a)
			}
		}
		if len(active) == 0 {
			break
		}

		prefix := string(runes[:i])
		g.view(ViewEvent{Type: ViewReveal, QuestionID: qid, Cursor: i, Text: prefix})

		upd := Packet{Request: ReqQuestionUpdate, QuestionID: qid, Cursor: i, Text: prefix}

		// lockstep barrier: fan out to all active agents, wait for all replies.
		replies := make([]string, len(active))
		var wg sync.WaitGroup
		for idx, a := range active {
			wg.Add(1)
			go func(idx int, a *Agent) {
				defer wg.Done()
				if err := a.SendJSON(upd); err != nil {
					log.Printf("[%s] send to %s failed: %v", g.ID, a.Name, err)
					return
				}
				r, err := a.Recv(g.rt)
				if err != nil {
					log.Printf("[%s] recv from %s failed: %v (treated as pass)", g.ID, a.Name, err)
					return
				}
				replies[idx] = r
			}(idx, a)
		}
		wg.Wait()

		correctNow := false
		for idx, a := range active {
			reply := strings.TrimSpace(replies[idx])
			if reply == "" || strings.EqualFold(reply, ReplyPass) {
				continue // still watching
			}
			ok := judge(reply, q.Answer)
			if ok {
				a.Committed = true // 正解で確定(以後この問題は抜ける)
				a.CorrectAt = i
				correctNow = true
				log.Printf("[%s] Q%d [%s] CORRECT at %d/%d: %q", g.ID, qid, a.Name, i, total, reply)
			} else if benchmark {
				// benchmark: 誤答はロックアウトも確定もせず、次の文字でまた回答できる
				log.Printf("[%s] Q%d [%s] wrong at %d/%d: %q (benchmark: 続行)", g.ID, qid, a.Name, i, total, reply)
			} else {
				a.Committed = true // 1問1回:誤答でロックアウト
				a.LockedOut = true
				log.Printf("[%s] Q%d [%s] WRONG at %d/%d: %q (locked out)", g.ID, qid, a.Name, i, total, reply)
			}
			g.view(ViewEvent{Type: ViewAnswer, QuestionID: qid, Cursor: i, Agent: a.Name, Reply: reply, Correct: ok})
		}

		if firstAnswer && correctNow {
			break // mode first_answer: a correct answer ends the question
		}
	}

	scores := map[string]int{}
	totals := map[string]int{}
	for _, a := range g.agents {
		pts := 0
		if a.CorrectAt > 0 {
			pts = total - a.CorrectAt + 1 // earlier correct = higher score
		}
		a.Score += pts
		scores[a.Name] = pts
		totals[a.Name] = a.Score
	}
	g.setScores(totals)
	g.broadcast(Packet{Request: ReqResult, QuestionID: qid, Answer: q.Answer, Scores: scores})
	g.view(ViewEvent{Type: ViewResult, QuestionID: qid, Text: q.Text, Answer: q.Answer, Scores: scores, Totals: totals})
	log.Printf("[%s] Q%d done answer=%q scores=%v", g.ID, qid, q.Answer, scores)
}
