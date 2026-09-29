# fc

NES(ファミコン)用のコンパイラです。C風の独自言語 (FC言語) を ca65 アセンブリにコンパイルし、
ld65 でリンクして NES ROM (.nes) または実験用バイナリ (emu ターゲット) を生成します。

本体は Go で実装されています。

## 必要なもの

- Go 1.24+
- [cc65](https://cc65.github.io/) の ca65 / ld65（ソースからビルドする場合は PATH に置く。
  [リリース](https://github.com/haramako/fc/releases)の Windows / Linux(amd64) 版には同梱してあり、`fcc` と同じ
  ディレクトリのものが優先される。`FC_CC65_BIN=<dir>` で差し替え可。`fcc version` がどれを使うか表示する）

## ビルド

```bash
go build -o fcc ./cmd/fcc
```

`fclib/` と `share/` はバイナリに埋め込まれるため、生成した `fcc` は単体で配布できます
(リポジトリ内で実行する場合はリポジトリのファイルがそのまま使われます)。

## 使い方

```bash
fcc build src.fc            # ビルド (emuターゲット, a.bin)
fcc build -t nes src.fc     # NES ROM を生成 (a.nes)
fcc run src.fc              # ビルドして内蔵6502エミュレータで実行
fcc compile src.fc          # コンパイルのみ
fcc fmt -w src.fc           # ソースを整形 (-l: 変わるファイルを列挙, -d: 差分表示)
fcc doc vram                # 標準ライブラリのモジュールの説明 (fcc doc だけなら一覧)
```

オプション:

| オプション | 説明 |
|---|---|
| `-o FILE` | 出力ファイル名 |
| `-t TARGET` | ターゲット (`nes` / `emu`) |
| `-O LEVEL` | 最適化レベル (0-2, デフォルト 2) |
| `-e` | ビルド後にエミュレータで実行 |

言語仕様は [docs/language_reference.md](docs/language_reference.md) を参照してください。

## テスト

```bash
go test ./...
```

fc のモジュールのテスト（`@(test)` の関数。`@assert` / `@assert_eq`）は `fcc test mod.fc` で走らせる（emu で実行し、全部
通れば終了コード 0。`-t nes` なら内蔵の NES のランナーで。language_reference.md §7）。

ほかのフォルダや git のリポジトリのモジュールは、fc.toml の `[lib.NAME]`（`path = "../lib"` か `git = "URL"` / `rev` / `dir`）で
使える。git のものはビルドのときにユーザーのキャッシュへ取ってきて、コミットを `fc.lock` に固定する（`fcc lib fetch` /
`update` / `list`。language_reference.md §1.1）。

テストは `testdata/golden/` の golden データ (AST / IR / 割付後IR / アセンブリ / バイナリ /
実行出力) との差分比較で行われます。golden は Go 自身の出力のスナップショットで、次で再生成します:

```bash
go test ./internal/driver -run 'TestGolden|TestExample' -update
```

## リポジトリ構成

```
cmd/fcc/          CLI (pkg/fc の薄い皮)
pkg/fc/           ライブラリとしての公開 API (New / Build / Options / Result / Error)
internal/syntax/  字句解析・構文解析・構文木 (他の internal に依存しない)
internal/types/   型とインターン
internal/diag/    診断 (エラー) 型
internal/ir/      中間表現 (Value / Op / Lambda / Module) とダンプ
internal/sema/    意味解析 (構文木 → IR)、組み込みマクロ
internal/regalloc/ レジスタ割付
internal/codegen/ IR → ca65 アセンブリ
internal/driver/  パイプライン統括 (ca65 / ld65 の起動、リンク、emu 実行)。golden / examples テストもここ
internal/r6502/   6502エミュレータ (emuターゲット実行用)
internal/nes/     ヘッドレスNESランナー (テスト用)
fclib/            FC言語の標準ライブラリ
share/            ランタイムアセンブリ・リンカ設定
test/             FC言語のテストソース
examples/         実プロジェクト由来の回帰テスト用サンプル (miku / castle)
testdata/golden/  golden データ
tools/            開発用ツール、VS Code 拡張 (tools/vscode-fc。ハイライト / fcc fmt / fcc check)
docs/             利用者向けドキュメントのサイト (VitePress。GitHub Pages)
Agent/            開発の知識・設計・経緯・計画 (主にエージェント向け。AGENTS.md)
```

## ドキュメント

| ファイル | 内容 |
|---|---|
| [docs/language_reference.md](docs/language_reference.md) | FC言語の仕様 |
| [examples/README.md](examples/README.md) | サンプルの構成・同期方法・エミュレータテスト |
| [bench/README.md](bench/README.md) | 生成コードのベンチマークと他コンパイラとの比較 |

開発者向けの知識・設計・経緯・計画は [Agent/](Agent/AGENTS.md) にまとめている（主にコーディングエージェント向け。規約はルートの
[AGENTS.md](AGENTS.md)）。入口は [Agent/wiki/AGENTS.md](Agent/wiki/AGENTS.md)（テストと fuzz、コードの構造、設計、ロードマップ）。

## License

MIT
