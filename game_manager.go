package main

import (
	"fmt"
	"sync"
)

// GameManager tracks active games so multiple can run concurrently and be
// discovered via the REST API and the viewer.
type GameManager struct {
	mu    sync.RWMutex
	games map[string]*Game
	order []string // creation order (for "newest")
	seq   int
}

func NewGameManager() *GameManager {
	return &GameManager{games: make(map[string]*Game)}
}

// NewID allocates a fresh unique game id.
func (m *GameManager) NewID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return fmt.Sprintf("game-%d", m.seq)
}

func (m *GameManager) Register(g *Game) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.games[g.ID] = g
	m.order = append(m.order, g.ID)
}

func (m *GameManager) Unregister(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.games, id)
	for i, x := range m.order {
		if x == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

func (m *GameManager) Get(id string) *Game {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.games[id]
}

// Newest returns the most recently registered active game (or nil).
func (m *GameManager) Newest() *Game {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := len(m.order) - 1; i >= 0; i-- {
		if g := m.games[m.order[i]]; g != nil {
			return g
		}
	}
	return nil
}

// List returns snapshots of all active games in creation order.
func (m *GameManager) List() []GameSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]GameSnapshot, 0, len(m.order))
	for _, id := range m.order {
		if g := m.games[id]; g != nil {
			out = append(out, g.Snapshot())
		}
	}
	return out
}

func (m *GameManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.games)
}
