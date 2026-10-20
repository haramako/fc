# fuzz-loop

origin/main の最新で、夜間の CI (`.github/workflows/fuzz.yml` の random) と同じ差分 fuzz (`internal/driver` の `TestRandom*`) を回し続ける。
1 回ごとに `git fetch` して origin/main に合わせ、分まで入れた種 (`YYYYMMDDHHMM0000`) で回す。失敗したらそこで止まる。

## 実行

```bash
caffeinate -i Agent/scripts/fuzz-loop/fuzz-loop.sh   # Mac。caffeinate はアイドルでスリープさせない
```

- 前提: Go と cc65 (ca65 / ld65) が PATH にある。origin から clone できる
- 環境変数: `FC_FUZZ_DIR`（作業場所。既定 `~/.cache/fc-fuzz`）、`FC_FUZZ_REPO`（clone 元）、`FC_FUZZ_RANDN`（1 回の `-randn`。既定 4000。
  `-foldn` / `-metan` / `-mutn` はその 1/4・1/10・3/20）
- 作業場所: `src/`（専用の clone。手元の作業コピーは触らない）、`summary.log`（回ごとの開始・終了・終了コード）、`logs/`（失敗した回の `go test` の出力と
  `FUZZ_STATS` の jsonl。通った回のものは消す）
- 終了コード: 1 = fuzz の失敗（ログのパスを出す）、2 = 最初の clone に失敗
- 目安: Apple M1 で 1 回（既定の本数）約 1 時間 40 分

## 制約

- 失敗は、再現・最小化して仕様 (`docs/reference/`) と比べるまで、生成器の穴かコンパイラのバグかは分からない（調べ方は `Agent/wiki/testing-and-fuzzing.md`）
- 同じ rev でも種を変えて回し続ける。go test のビルドキャッシュは共有なので、rev が変わらなければ 2 回目からビルドは速い
