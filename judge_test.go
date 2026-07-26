package main

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"東京":    "東京",
		"東京都":   "東京都",
		"ＡＢＣ":   "abc",   // 全角英字 -> 半角小文字
		"トウキョウ": "とうきょう", // カタカナ -> ひらがな
		"H2O ":  "h2o",   // 空白除去・小文字化
		"夏目 漱石": "夏目漱石",  // 空白除去
		"ワシントン": "わしんとん",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestJudge(t *testing.T) {
	type tc struct {
		reply, correct string
		want           bool
	}
	cases := []tc{
		{"東京", "東京", true},
		{"東京都", "東京", true},           // 部分文字列一致
		{"答えは東京です", "東京", true},       // 前後に語句があってもOK
		{"トウキョウ", "とうきょう", true},      // カタカナ/ひらがな正規化
		{"ジョージ・ワシントン", "ワシントン", true}, // 記号・かな
		{"大阪", "東京", false},
		{"pass", "東京", false},
		{"", "東京", false},
		{"なんでも", "", false}, // 正解が空なら常に false
	}
	for _, c := range cases {
		if got := judge(c.reply, c.correct); got != c.want {
			t.Errorf("judge(%q, %q)=%v, want %v", c.reply, c.correct, got, c.want)
		}
	}
}
