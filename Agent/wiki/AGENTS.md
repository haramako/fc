# Wiki

## Purpose

各ディレクトリの `AGENTS.md` には載らない、**複数ディレクトリにまたがる横断的なコードベース知識**を記事として蓄積する。

## Local Contracts

- **ここに書くもの**: 設計の全体像、レイヤーをまたぐデータフロー、命名・パターンの慣習、デバッグの勘所など、特定ディレクトリに帰属しない知識
- **ここに書かないもの**: 特定ディレクトリに閉じた知識（→ そのディレクトリの `AGENTS.md`）、会話の経緯・決定理由（→ `../discussions/`）、Issue の作業記録（→ `../issues/`）
- 記事ファイル名は内容を表す kebab-case（例: `data-pipeline-overview.md`）
- サブフォルダ: `design/` … 実装済み機能の設計（仕様と理由。コードのコメントから `§` 付きで参照される）、`plans/` … 進行中の計画・候補（済んだ項目は消すか `design/` へ昇格し、経緯は `../discussions/` へ）
- コードのコメントから参照されている記事を改名・移動したら、参照元（`grep -rn "Agent/wiki/"`）も直す
- 記事を追加・改名・削除したら、この `AGENTS.md` の **Articles** インデックスを更新する
- 記事の内容が特定ディレクトリに帰属すると判明したら、その `AGENTS.md` へ移す
- 記事がコードと矛盾していると気づいたら、記事側を直すか削除する（コードが常に正・古い記事を放置しない）

## Articles

- [testing-and-fuzzing.md](testing-and-fuzzing.md) — `go test ./...` の層、差分 fuzz・Go native fuzz・バンク切替 fuzz の回し方、失敗した種と実プロジェクトの退行の調べ方、ベンチ
- [code-structure.md](code-structure.md) — パッケージの地図（syntax → sema → opt → regalloc → codegen → driver）と IR の約束（命令の性質・幅と符号・常駐の印・opt の段・検証器）
- [implementation-notes.md](implementation-notes.md) — 機能別の実装の要点とハマりどころ（常駐の正しさ、cast の正規形、far call、エラー報告、ツールの探索、CI、フレームの静的割付、最適化のパイプライン、インライン展開、ca65 の再利用）
- [log-annotation-rules.md](log-annotation-rules.md) — `@log` の注釈を最適化のパスで落とさないための規則
- [mesen-and-debugging.md](mesen-and-debugging.md) — MesenCE の導入と罠、`fcc build -g` のソースレベルデバッグ、エディタ連携、コードサイズ
- [placement-debugging.md](placement-debugging.md) — 配置指定・Mesen のスタック表示・一時ディレクトリの検討
- [golden-dump-format.md](golden-dump-format.md) — golden ダンプの正規形の仕様

### design/（実装済み機能の設計）

- [design/ir-memops.md](design/ir-memops.md) — メモリアクセス命令の集約（`load_mem` / `store_mem`）
- [design/ssa.md](design/ssa.md) — SSA による定数伝播・コピー伝播・DCE
- [design/regalloc.md](design/regalloc.md) — レジスタ割付: ループ内の A / Y 常駐
- [design/frame-alloc.md](design/frame-alloc.md) — フレーム割付（frame size over の解消と静的フレーム、Y での引数渡し）
- [design/farcall.md](design/farcall.md) — far call（バンクをまたぐ呼び出し）
- [design/far-function-pointers.md](design/far-function-pointers.md) — farcall 対応の関数ポインタ
- [design/bss.md](design/bss.md) — モジュール・ブロックの BSS 配置
- [design/storage-alias.md](design/storage-alias.md) — 型付きストレージ alias
- [design/types-struct.md](design/types-struct.md) — 型構文（Go/Zig 順）と struct / soa

### plans/（進行中の計画）

- [plans/v4-plan.md](plans/v4-plan.md) — fc 4 の言語の変更（版と migrate を含む）
- [plans/v4-stdlib.md](plans/v4-stdlib.md) — fc 4 の標準ライブラリ（fclib）の作り直し
- [plans/roadmap.md](plans/roadmap.md) — 未着手・未決の項目（最適化・言語機能・ツール）
- [plans/language-feature-candidates.md](plans/language-feature-candidates.md) — 言語機能の追加候補
