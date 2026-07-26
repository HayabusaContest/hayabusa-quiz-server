package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config is the server configuration (config.yml).
type Config struct {
	Host              string `yaml:"host"`
	Port              int    `yaml:"port"`
	AgentCount        int    `yaml:"agent_count"`
	Mode              string `yaml:"mode"`  // reveal_all | first_answer | benchmark
	Judge             string `yaml:"judge"` // normalized_match
	Questions         string `yaml:"questions"`
	ResponseTimeoutMs int    `yaml:"response_timeout_ms"`
	LogDir            string `yaml:"log_dir"` // ゲームログ(JSONL)の保存先。空なら保存しない
}

// Question is one quiz row (No, 問題文, 解答).
type Question struct {
	No     string
	Text   string
	Answer string
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{
		Host:              "0.0.0.0",
		Port:              8080,
		AgentCount:        2,
		Mode:              "reveal_all",
		Judge:             "normalized_match",
		ResponseTimeoutMs: 30000,
		LogDir:            "logs",
	}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}
	applyEnvOverrides(c)
	if c.Questions == "" {
		return nil, fmt.Errorf("config: 'questions' path is required")
	}
	if c.AgentCount < 1 {
		return nil, fmt.Errorf("config: agent_count must be >= 1")
	}
	if c.Mode != ModeRevealAll && c.Mode != ModeFirstAnswer && c.Mode != ModeBenchmark {
		return nil, fmt.Errorf("config: mode must be %q, %q, or %q", ModeRevealAll, ModeFirstAnswer, ModeBenchmark)
	}
	return c, nil
}

// applyEnvOverrides lets container/orchestration override host/port without
// editing config.yml (HOST / PORT env vars).
func applyEnvOverrides(c *Config) {
	if v := os.Getenv("HOST"); v != "" {
		c.Host = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
}

func LoadQuestions(path string) ([]Question, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")) // strip UTF-8 BOM
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var qs []Question
	for i, row := range rows {
		if len(row) < 3 {
			continue
		}
		if i == 0 && (row[0] == "No" || row[1] == "問題文") {
			continue // header
		}
		qs = append(qs, Question{No: row[0], Text: row[1], Answer: row[2]})
	}
	return qs, nil
}
