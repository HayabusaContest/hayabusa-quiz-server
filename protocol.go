package main

// プロトコル定数。正典は ../hayabusa-quiz-common/protocol/protocol.json。
// common の tests/test_protocol_sync.py が、この定数群が正典と一致するか検査する。

// ProtocolVersion はプロトコルの版(semver)。
const ProtocolVersion = "1.0.0"

// Request types: サーバ → エージェント。
const (
	ReqName           = "NAME"
	ReqQuestionStart  = "QUESTION_START"
	ReqQuestionUpdate = "QUESTION_UPDATE"
	ReqResult         = "RESULT"
	ReqFinish         = "FINISH"
)

// View event types: サーバ → 観戦ビューア(read-only)。
const (
	ViewGameStart     = "game_start"
	ViewQuestionStart = "question_start"
	ViewReveal        = "reveal"
	ViewAnswer        = "answer"
	ViewResult        = "result"
	ViewFinish        = "finish"
)

// Modes: 対戦モード(config)。
const (
	ModeRevealAll   = "reveal_all"
	ModeFirstAnswer = "first_answer"
	ModeBenchmark   = "benchmark"
)

// ReplyPass はエージェントの「見過ごし」返信キーワード。
const ReplyPass = "pass"
