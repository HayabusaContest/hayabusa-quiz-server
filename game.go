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

const (
	ReqName           = "NAME"
	ReqQuestionStart  = "QUESTION_START"
	ReqQuestionUpdate = "QUESTION_UPDATE"
	ReqResult         = "RESULT"
	ReqFinish         = "FINISH"

	replyPass = "pass"

	modeFirstAnswer = "first_answer"
)

type Game struct {
	cfg    *Config
	agents []*Agent
	qs     []Question
	rt     time.Duration
	hub    *Hub // spectator feed (may be nil)
}

func NewGame(cfg *Config, agents []*Agent, qs []Question, hub *Hub) *Game {
	return &Game{
		cfg:    cfg,
		agents: agents,
		qs:     qs,
		rt:     time.Duration(cfg.ResponseTimeoutMs) * time.Millisecond,
		hub:    hub,
	}
}

func (g *Game) broadcast(p Packet) {
	for _, a := range g.agents {
		if err := a.SendJSON(p); err != nil {
			log.Printf("broadcast to %s failed: %v", a.Name, err)
		}
	}
}

func (g *Game) view(e ViewEvent) {
	if g.hub != nil {
		g.hub.Broadcast(e)
	}
}

// Run plays every question, then broadcasts the final scores.
func (g *Game) Run() {
	names := make([]string, len(g.agents))
	for i, a := range g.agents {
		names[i] = a.Name
	}
	g.view(ViewEvent{Type: "game_start", Agents: names})

	for i, q := range g.qs {
		g.playQuestion(i+1, q)
	}

	final := map[string]int{}
	for _, a := range g.agents {
		final[a.Name] = a.Score
	}
	g.broadcast(Packet{Request: ReqFinish, Scores: final})
	g.view(ViewEvent{Type: "finish", Scores: final})
	log.Printf("=== GAME FINISHED === %v", final)
}

// playQuestion reveals the question one rune at a time in lockstep.
// Each step all still-active agents must reply "pass" or an answer;
// the first non-pass answer commits an agent for the whole question.
//   - reveal_all:   reveal to the end; each agent scored by its earliest correct.
//   - first_answer: a correct answer ends the question (wrong answers lock out
//                   that agent and revealing continues for the rest).
func (g *Game) playQuestion(qid int, q Question) {
	for _, a := range g.agents {
		a.Committed = false
		a.LockedOut = false
		a.CorrectAt = 0
	}

	runes := []rune(q.Text)
	total := len(runes)

	g.broadcast(Packet{Request: ReqQuestionStart, QuestionID: qid})
	g.view(ViewEvent{Type: "question_start", QuestionID: qid, Total: total})

	firstAnswer := g.cfg.Mode == modeFirstAnswer

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
		g.view(ViewEvent{Type: "reveal", QuestionID: qid, Cursor: i, Text: prefix})

		upd := Packet{Request: ReqQuestionUpdate, QuestionID: qid, Cursor: i, Text: prefix}

		// lockstep barrier: fan out to all active agents, wait for all replies.
		replies := make([]string, len(active))
		var wg sync.WaitGroup
		for idx, a := range active {
			wg.Add(1)
			go func(idx int, a *Agent) {
				defer wg.Done()
				if err := a.SendJSON(upd); err != nil {
					log.Printf("send to %s failed: %v", a.Name, err)
					return
				}
				r, err := a.Recv(g.rt)
				if err != nil {
					log.Printf("recv from %s failed: %v (treated as pass)", a.Name, err)
					return
				}
				replies[idx] = r
			}(idx, a)
		}
		wg.Wait()

		correctNow := false
		for idx, a := range active {
			reply := strings.TrimSpace(replies[idx])
			if reply == "" || strings.EqualFold(reply, replyPass) {
				continue // still watching
			}
			a.Committed = true // one-shot: this is the agent's answer for the question
			ok := judge(reply, q.Answer)
			if ok {
				a.CorrectAt = i
				correctNow = true
				log.Printf("Q%d [%s] CORRECT at %d/%d: %q", qid, a.Name, i, total, reply)
			} else {
				a.LockedOut = true
				log.Printf("Q%d [%s] WRONG at %d/%d: %q (locked out)", qid, a.Name, i, total, reply)
			}
			g.view(ViewEvent{Type: "answer", QuestionID: qid, Cursor: i, Agent: a.Name, Reply: reply, Correct: ok})
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
	g.broadcast(Packet{Request: ReqResult, QuestionID: qid, Answer: q.Answer, Scores: scores})
	g.view(ViewEvent{Type: "result", QuestionID: qid, Answer: q.Answer, Scores: scores, Totals: totals})
	log.Printf("Q%d done answer=%q scores=%v", qid, q.Answer, scores)
}
