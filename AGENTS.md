# fc

**fc** は NES（ファミコン）用のコンパイラ。C 風の独自言語（FC 言語）を ca65 アセンブリにコンパイルし、ld65 でリンクして NES ROM（`.nes`）または実験用バイナリ（emu ターゲット）を作る。本体は Go（`cmd/fcc` の CLI と `pkg/fc` のライブラリ）。人間向けの説明は `README.md` と `doc/`。

**エージェント（Claude / Codex 等）は、この後に続く `Agent/AGENTS.md` も必ず読むこと**（作業領域の規約。Claude Code は末尾の `@Agent/AGENTS.md` で自動で読み込む）。

## Project-Wide Rules

- **コミットメッセージは `<パッケージや領域>: <説明>`**（例: `regalloc: …`、`fclib/nes/vram: …`、`doc: …`）。`[AI]` 接頭辞は付けない
- **`doc/` は人間向けのドキュメント**（言語仕様など）。エージェント向けの知識・経緯・計画は `Agent/` に置く
- **`Agent/` はエージェントの作業領域**（規約は `Agent/AGENTS.md`）。**会話の中で重要な決定・知見が出たら、エージェントは自動で `Agent/discussions/` に記録する**（指示を待たない）
- **サブフォルダに `AGENTS.md` を追加・新設したら、同じ階層に内容 `@AGENTS.md` のみの `CLAUDE.md` スタブを置く**（Claude Code がそのサブツリーで作業を始めた時に DOX 契約を自動ロードさせるため）。既に中身のある `CLAUDE.md` がある場合は内容を `AGENTS.md` に統合し、`CLAUDE.md` は `@AGENTS.md` のみにする
- 生成物は直接編集しない: `internal/syntax/parser.go`（`parser.y` から goyacc）、`testdata/golden/`（`-update` で再生成）
- **push 注意**: origin は公開の github.com/haramako/fc。`examples/castle` は製品コード（ゲームテキスト・リソース含む）なので、**push する前に公開可否の判断が必要**

## Build & Commands（リポジトリルートで実行）

```bash
go build -o fcc ./cmd/fcc                                       # ビルド（fclib/ と share/ は埋め込み）
go test ./...                                                   # 全テスト（golden + examples + NES スモーク + 差分 fuzz の既定本数）
go test ./internal/driver -run 'TestGolden|TestExample' -update # golden の再生成
fcc fmt -w src.fc                                               # FC ソースの整形
```

テストの層と fuzz の回し方は `Agent/wiki/testing-and-fuzzing.md`。

# DOX framework

- DOX is highly performant AGENTS.md hierarchy installed here
- Agent must follow DOX instructions across any edits

## Core Contract

- AGENTS.md files are binding work contracts for their subtrees
- Work products, source materials, instructions, records, assets, and durable docs must stay understandable from the nearest applicable AGENTS.md plus every parent AGENTS.md above it

## Read Before Editing

1. Read the root AGENTS.md
2. Identify every file or folder you expect to touch
3. Walk from the repository root to each target path
4. Read every AGENTS.md found along each route
5. If a parent AGENTS.md lists a child AGENTS.md whose scope contains the path, read that child and continue from there
6. Use the nearest AGENTS.md as the local contract and parent docs for repo-wide rules
7. If docs conflict, the closer doc controls local work details, but no child doc may weaken DOX

Do not rely on memory. Re-read the applicable DOX chain in the current session before editing.

## Update After Editing

Every meaningful change requires a DOX pass before the task is done.

Update the closest owning AGENTS.md when a change affects:

- purpose, scope, ownership, or responsibilities
- durable structure, contracts, workflows, or operating rules
- required inputs, outputs, permissions, constraints, side effects, or artifacts
- user preferences about behavior, communication, process, organization, or quality
- AGENTS.md creation, deletion, move, rename, or index contents

Update parent docs when parent-level structure, ownership, workflow, or child index changes. Update child docs when parent changes alter local rules. Remove stale or contradictory text immediately. Small edits that do not change behavior or contracts may leave docs unchanged, but the DOX pass still must happen.

## Hierarchy

- Root AGENTS.md is the DOX rail: project-wide instructions, global preferences, durable workflow rules, and the top-level Child DOX Index
- Child AGENTS.md files own domain-specific instructions and their own Child DOX Index
- Each parent explains what its direct children cover and what stays owned by the parent
- The closer a doc is to the work, the more specific and practical it must be

## Child Doc Shape

- Create a child AGENTS.md when a folder becomes a durable boundary with its own purpose, rules, responsibilities, workflow, materials, or quality standards
- Work Guidance must reflect the current standards of the project or user instructions; if there are no specific standards or instructions yet, leave it empty
- Verification must reflect an existing check; if no verification framework exists yet, leave it empty and update it when one exists

Default section order:
- Purpose
- Ownership
- Local Contracts
- Work Guidance
- Verification
- Child DOX Index

## Style

- Keep docs concise, current, and operational
- Document stable contracts, not diary entries
- Put broad rules in parent docs and concrete details in child docs
- Prefer direct bullets with explicit names
- Do not duplicate rules across many files unless each scope needs a local version
- Delete stale notes instead of explaining history
- Trim obvious statements, repeated rules, misplaced detail, and warnings for risks that no longer exist

## Closeout

1. Re-check changed paths against the DOX chain
2. Update nearest owning docs and any affected parents or children
3. Refresh every affected Child DOX Index
4. Remove stale or contradictory text
5. Run existing verification when relevant
6. Report any docs intentionally left unchanged and why

## User Preferences

When the user requests a durable behavior change, record it here or in the relevant child AGENTS.md

## Child DOX Index

- `Agent/AGENTS.md` — エージェント作業の蓄積領域。配下に `wiki/`（横断知識・設計・進行中の計画）・`discussions/`（会話と決定の歴史）・`issues/`（Issue 記録）・`scripts/`（補助スクリプト）

Owned directly by root (no child doc): `cmd/`・`pkg/`・`internal/`（コンパイラ本体。パッケージの地図は `Agent/wiki/code-structure.md`）、`fclib/`（FC の標準ライブラリ）、`share/`（ランタイムアセンブリ・リンカ設定）、`test/`（FC のテストソース）、`testdata/`（golden）、`examples/`（実プロジェクト由来の回帰サンプル。`examples/README.md`）、`bench/`（生成コードのベンチ。`bench/README.md`）、`editors/`（エディタ拡張）、`tools/`（開発用ツール）、`doc/`（人間向けドキュメント）、`README.md`。

以下で `Agent/AGENTS.md` を毎セッション読み込む（Claude Code の import。他エージェントはこの行を「必読ファイルの指示」として読むこと。DOX の「Read Before Editing」は `Agent/` 配下を触らないセッションでは読まれないため、discussions 自動記録を毎セッション発火させる目的で強制ロードする）：

@Agent/AGENTS.md
