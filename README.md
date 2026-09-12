# fc

NES(ファミコン)用のコンパイラです。C風の独自言語 (FC言語) を ca65 アセンブリにコンパイルし、
ld65 でリンクして NES ROM (.nes) または実験用バイナリ (emu ターゲット) を生成します。

本体は Go で実装されています (元の Ruby 実装からの移植。Ruby 版は削除済みで、
タグ `ruby-frozen` から参照できます: `git show ruby-frozen:ruby/lib/fc/hlc.rb` など)。

## 必要なもの

- Go 1.24+
- [cc65](https://cc65.github.io/) (ca65 / ld65 が PATH にあること)

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
```

オプション:

| オプション | 説明 |
|---|---|
| `-o FILE` | 出力ファイル名 |
| `-t TARGET` | ターゲット (`nes` / `emu`) |
| `-O LEVEL` | 最適化レベル (0-2, デフォルト 2) |
| `-e` | ビルド後にエミュレータで実行 |

言語仕様は [doc/language_reference.md](doc/language_reference.md) を参照してください。

## テスト

```bash
go test ./...
```

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
tools/            サンプル同期ツール
doc/              ドキュメント
```

## ドキュメント

| ファイル | 内容 |
|---|---|
| [doc/development_notes.md](doc/development_notes.md) | **開発時にまず読む**: 環境・ブランチ運用・テストの回し方・ハマりどころ |
| [doc/language_reference.md](doc/language_reference.md) | FC言語の仕様 |
| [doc/go_evolution_plan.md](doc/go_evolution_plan.md) | 今後の計画（Goらしい設計への転換・機能追加）と作業ログ |
| [doc/go_port_plan.md](doc/go_port_plan.md) | Ruby→Go 移植の記録（アーカイブ） |
| [doc/go_port_dump_format.md](doc/go_port_dump_format.md) | golden ダンプ正規形の仕様 |
| [doc/optimization.md](doc/optimization.md) / [doc/register_allocation.md](doc/register_allocation.md) | 最適化・レジスタ割付の解説 |
| [examples/README.md](examples/README.md) | サンプルの構成・同期方法・エミュレータテスト |

## License

MIT
