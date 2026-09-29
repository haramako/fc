# else if と elsif

日付: 2026-09-30

## 背景

利用者向けドキュメントの例を `fcc fmt` にかけたら、`} else if (...) {` が `} else` と字下げした `if` に崩れた。
fc の else-if は `elsif` と書く言語で、`else if` は `else` の中の `if` 文として整形されていた。意味は同じ
（構文木の `IfStmt.IsElsif` を見るのは整形だけ）。

## 決定事項

- `elsif` は昔の原始的な整形の都合で入れたもの。**`else if` が正しく動くなら `elsif` はそのうち消してよい**（ユーザー）
- まず `fcc fmt` が `} else if (...) {` を `elsif` と同じく字下げせずに続けるようにした（`internal/syntax/printer.go`）
- 利用者向けドキュメントの例は `else if` で書く
- `elsif` をなくすのは fc 4 で（エラーにして、fc 3 → 4 の migrate が `else if` に書き換える）。`plans/roadmap.md` の v4 に候補として置いた
