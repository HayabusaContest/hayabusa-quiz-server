package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// EventSink は観戦イベントの出力先。sink を足せば出力先(配信/ログ/…)を増やせる。
type EventSink interface {
	Emit(e ViewEvent)
	Close()
}

// CompositeSink は1つのイベントを複数の sink へ登録順に配る。
type CompositeSink struct{ sinks []EventSink }

// NewComposite は nil を除いた sink 群をまとめる。
func NewComposite(sinks ...EventSink) *CompositeSink {
	out := make([]EventSink, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			out = append(out, s)
		}
	}
	return &CompositeSink{sinks: out}
}

func (c *CompositeSink) Emit(e ViewEvent) {
	for _, s := range c.sinks {
		s.Emit(e)
	}
}

func (c *CompositeSink) Close() {
	for _, s := range c.sinks {
		s.Close()
	}
}

// HubSink は観戦ハブ(接続中の全ビューア)へブロードキャストする sink。
type HubSink struct{ hub *Hub }

// NewHubSink は hub が nil なら interface-nil を返す(配信不要。typed-nil 回避)。
func NewHubSink(hub *Hub) EventSink {
	if hub == nil {
		return nil
	}
	return &HubSink{hub: hub}
}

func (h *HubSink) Emit(e ViewEvent) { h.hub.Broadcast(e) }
func (h *HubSink) Close()           {}

// LogSink はゲームログ(観戦イベントの JSONL)をファイルへ追記する sink。
type LogSink struct{ f *os.File }

// NewLogSink は dir が空、または作成失敗時に interface-nil を返す(保存しない)。
// id をファイル名に含めるので、同一秒に始まる並行ゲームでも衝突しない。
func NewLogSink(dir, id string) EventSink {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("log dir error: %v", err)
		return nil
	}
	path := filepath.Join(dir, fmt.Sprintf("game_%s_%s.jsonl", time.Now().Format("20060102-150405"), id))
	f, err := os.Create(path)
	if err != nil {
		log.Printf("log file error: %v", err)
		return nil
	}
	log.Printf("logging game to %s", path)
	return &LogSink{f: f}
}

func (l *LogSink) Emit(e ViewEvent) {
	if l.f == nil {
		return
	}
	if b, err := json.Marshal(e); err == nil {
		_, _ = l.f.Write(append(b, '\n'))
	}
}

func (l *LogSink) Close() {
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
}
