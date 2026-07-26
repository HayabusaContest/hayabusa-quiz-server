# hayabusa-quiz-server

早押しクイズ AI 対戦システムのゲームサーバ(Go)です。問題文を1文字ずつ全エージェントに配信し、全員の `pass`/回答がそろってから次の1文字へ進めます(lockstep)。判定・採点・観戦フィード配信・接続待ち受けを担います。

システム全体像・プロトコル・compose 一括起動は [hayabusa-quiz-common](https://github.com/HayabusaContest/hayabusa-quiz-common) を参照してください。

## 実行方法

Go 1.22 以上。

```bash
git clone git@github.com:HayabusaContest/hayabusa-quiz-server.git
cd hayabusa-quiz-server
go run . -c config/config.yml        # ws://localhost:8080/ws で待受
```

単体バイナリ(版数埋め込み)にする場合:

```bash
go build -ldflags "-X main.version=$(git describe --tags --always)" -o hayabusa-quiz-server .
./hayabusa-quiz-server -v             # 版とプロトコル版を表示
./hayabusa-quiz-server -c config/config.yml
```

Docker:

```bash
docker build -t hayabusa-quiz-server .
docker run -p 8080:8080 hayabusa-quiz-server
```

## 設定(config/config.yml)

```yaml
agent_count: 3           # この人数が接続したらゲーム開始
mode: reveal_all         # reveal_all / first_answer / benchmark
judge: normalized_match  # 解答の正規化文字列一致
questions: data/questions.csv
response_timeout_ms: 30000
log_dir: logs            # ゲームログ(観戦イベントの JSONL)の保存先。空なら保存しない
```

`host` / `port` は環境変数 `HOST` / `PORT` で上書きできます(コンテナ運用向け)。

各ゲームは `logs/game_<日時>.jsonl` に保存されます。**[viewer](https://github.com/HayabusaContest/hayabusa-quiz-viewer) のアーカイブ再生**で観られ、viewer の `logs/` に置いて `tools/build_index.py` を回せば一覧(カタログ)に追加できます。

- **reveal_all** … 最後まで開示し、各エージェントの最速正解位置でスコア(1問1回・誤答ロックアウト)。
- **first_answer** … 誰かが正解したら終了(誤答はロックアウトして続行)。競技寄り。
- **benchmark** … 誤答ロックアウト無し・毎トークン回答可。pass を使わない**常時回答型(hayabusa-chick そのまま)**が乗る。
- サーバは1ゲーム終了後も**常駐**し、次の接続を待ちます。

## エンドポイント

- `ws /ws` … エージェント(プレイヤー)
- `ws /viewer` … 観戦ビューア(read-only)
- `GET /healthz`, `/readyz` … 死活監視(JSON。version・protocol を返す)
- `GET /` … テキストのヒント

`SIGINT` / `SIGTERM` を受けると新規接続を止め、進行中のゲームを終えてから終了します(graceful shutdown)。

## プロトコルと設計

- サーバ→エージェントは JSON、エージェント→サーバは文字列1本。定数は `protocol.go`(正典は common の [`protocol/protocol.json`](https://github.com/HayabusaContest/hayabusa-quiz-common/blob/main/protocol/protocol.json)、[PROTOCOL.md](https://github.com/HayabusaContest/hayabusa-quiz-common/blob/main/PROTOCOL.md))。
- 観戦イベントの出力は **`EventSink` 抽象**(`observer.go`)で、観戦ハブ配信(`HubSink`)と JSONL ログ(`LogSink`)へ `CompositeSink` が fan-out します。出力先(REST・実況など)はここに sink を足すだけで増やせます。

## テスト

```bash
go test ./...        # judge 正規化・判定 など
go vet ./...
```
