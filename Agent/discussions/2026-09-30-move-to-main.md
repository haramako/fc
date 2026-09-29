# 開発を main に移す

日付: 2026-09-30

## 背景

`go install github.com/haramako/fc/cmd/fcc@feature/v4` を Go が受け付けない（ブランチ名の `/`）ので、利用者向けドキュメントの
インストールの手順に `go install` を書けなかった。

## 決定事項

- **開発を `main` に移す**。それまでの `main`（fc 3）に `v0.0.3` のタグを付け、`feature/v4` を `main` にマージした
  （`main` にだけあった 3 つのコミットは同じ内容で `feature/v4` に入っていたので、マージの結果は `feature/v4` と同じ）
- 作業は今のブランチ（`main`）に直接コミットしてよい（[2026-09-30-direct-commit-to-current-branch.md](2026-09-30-direct-commit-to-current-branch.md)）
- ドキュメントのサイトは `main` から公開する（`.github/workflows/docs.yml`）。インストールは `go install …@main`
- ルートの `AGENTS.md` の「Branches & Environment」を更新した
