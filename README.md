# fc

NES(ファミコン)用のコンパイラです。C風の独自言語 (FC言語) を ca65 アセンブリにコンパイルし、
ld65 でリンクして NES ROM (.nes) または実験用バイナリ (emu ターゲット) を生成します。

本体は Go で実装されています (元の Ruby 実装からの移植。オリジナルは `ruby/` 以下と
タグ `ruby-frozen` に保存されています)。

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
実行出力) との差分比較で行われます。golden は Ruby 版 (オラクル) から生成されたものです:

```bash
ruby tools/gen_golden.rb   # ruby/ 以下の Ruby 版が必要 (ruby 3.x + racc)
```

## リポジトリ構成

```
cmd/fcc/          CLI
internal/fc/      コンパイラ本体 (レキサ/パーサ/HLC/アロケータ/LLC/ドライバ)
internal/r6502/   6502エミュレータ (emuターゲット実行用)
internal/nes/     ヘッドレスNESランナー (テスト用)
fclib/            FC言語の標準ライブラリ
share/            ランタイムアセンブリ・リンカ設定
test/             FC言語のテストソース
examples/         実プロジェクト由来の回帰テスト用サンプル (miku / castle)
testdata/golden/  golden データ
tools/            golden 生成・サンプル同期ツール
ruby/             オリジナルの Ruby 実装 (凍結)
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
