package main

import "sync"

// WaitingRoom collects connected agents until agent_count are present, then
// hands back a batch to start a game. Multiple batches can form over time,
// enabling concurrent games (each batch becomes its own game).
type WaitingRoom struct {
	mu      sync.Mutex
	waiting []*Agent
	need    int
}

func NewWaitingRoom(need int) *WaitingRoom {
	return &WaitingRoom{need: need}
}

// Add appends an agent. When enough are waiting, it returns a ready batch of
// exactly `need` agents (removed from the room) and ok=true.
func (w *WaitingRoom) Add(a *Agent) ([]*Agent, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.waiting = append(w.waiting, a)
	if len(w.waiting) < w.need {
		return nil, false
	}
	batch := make([]*Agent, w.need)
	copy(batch, w.waiting[:w.need])
	w.waiting = w.waiting[w.need:]
	return batch, true
}

// Waiting returns how many agents are currently waiting for a game to fill.
func (w *WaitingRoom) Waiting() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.waiting)
}
