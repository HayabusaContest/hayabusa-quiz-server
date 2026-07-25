# hayabusa-quiz-server

早押しクイズ AI 対戦システムのゲームサーバ(Go)です。問題文を1文字ずつ全エージェントに配信し、全員の `pass`/回答がそろってから次の1文字へ進めます(lockstep)。判定・採点・観戦フィード配信・接続待ち受けを担います。

システム全体像・プロトコル・compose 一括起動は [hayabusa-quiz-common](https://github.com/HayabusaContest/hayabusa-quiz-common) を参照してください。

## 実行方法

Go 1.22 以上。

```bash
git clone git@github.com:HayabusaContest/hayabusa-quiz-server.git
cd hayabusa-quiz-server
go run . -c config.yml        # ws://localhost:8080/ws で待受
```

単体バイナリにする場合:

```bash
go build -o hayabusa-quiz-server .
./hayabusa-quiz-server -c config.yml
```

Docker:

```bash
docker build -t hayabusa-quiz-server .
docker run -p 8080:8080 hayabusa-quiz-server
```

## 設定(config.yml)

```yaml
agent_count: 3           # この人数が接続したらゲーム開始
mode: reveal_all         # reveal_all(最速正解でスコア) / first_answer(正解で問題終了)
judge: normalized_match  # 解答の正規化文字列一致
questions: data/questions.csv
response_timeout_ms: 30000
```

- **reveal_all** … 最後まで開示し、各エージェントの最速正解位置でスコア。
- **first_answer** … 誰かが正解したら終了(誤答はロックアウトして続行)。
- 1問1回(pass以外を返した時点で確定)。サーバは1ゲーム終了後も**常駐**し、次の接続を待ちます。

## エンドポイント

- `ws /ws` … エージェント(プレイヤー)
- `ws /viewer` … 観戦ビューア(read-only)

プロトコルの詳細は [common の PROTOCOL.md](https://github.com/HayabusaContest/hayabusa-quiz-common/blob/main/PROTOCOL.md)。
