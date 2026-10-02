# GitHub Actions の失敗調査

2026-10-03、ユーザーの依頼で最新の実行ログとローカルの Windows で調査。今回は原因確認のみで、実装・テスト・workflow は変更しない。

- [CI 37021499327](https://github.com/haramako/fc/actions/runs/37021499327)（`5e783ca`）は Linux 成功、Windows 失敗。失敗は3件。
  - `TestMacroServers`: `filepath.Dir` の結果 `\p` を `/p` と直接比較。
  - `TestStarlarkMacros`: 診断の `tools\tables.star` に対し `tools/tables.star` を期待。
  - `TestSizeHTML`: HTML 内の JSON 抽出の正規表現が `;\n` 固定。Windows checkout のテンプレートは CRLF なので `;\r\n` となり一致しない。
- `go test ./internal/project ./internal/driver -run '^(TestMacroServers|TestStarlarkMacros|TestSizeHTML)$' -count=1` で3件とも再現した。`git ls-files --eol internal/sizehtml/page.html` は `i/lf w/crlf attr/text=auto`。
- [docs 36997918793](https://github.com/haramako/fc/actions/runs/36997918793)（`e1b70ab`）は `go:embed page.html` のファイル欠落で失敗。`5e783ca` で追加済みだが、docs の paths フィルタに `internal/sizehtml/**` がないため修正コミットでは docs が起動していない。
- 対応案: テストのパス期待値を OS 非依存にし、HTML 抽出で CRLF も受け付ける。docs は最新 main で workflow_dispatch し、fcc 全体に依存する生成処理に合わせて paths フィルタも見直す。古い失敗 run の再実行だけでは古い SHA を使うためファイル欠落は解消しない。

## 続けて依頼された main CI の修正

- `TestMacroServers` の期待パスに `filepath.FromSlash`、`TestStarlarkMacros` の診断内パスに `filepath.Join` を使用した。
- `TestSizeHTML` は LF / CRLF の両方で埋め込み JSON を抽出する。実装の出力や検査するデータの条件は変えない。
- 失敗していた3件を Windows で再実行し、すべて成功した。docs ワークフローは今回の修正対象に含めない。
- Windows の `go vet ./...`、`go test ./...`、`go build ./cmd/fcc` も成功した。
- DOX: テストの移植性だけの修正なので、既存の責務・構造・運用契約は変更なし。調査記録のインデックスと testing-and-fuzzing の注意点を更新した。
