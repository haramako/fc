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
internal/r6502/   6502エミュレータ
fclib/            FC言語の標準ライブラリ
share/            ランタイムアセンブリ・リンカ設定
test/             FC言語のテストソース
testdata/golden/  golden データ (Ruby版から生成)
tools/            golden 生成ツール (Ruby)
ruby/             オリジナルの Ruby 実装 (凍結)
doc/              ドキュメント (言語仕様、Go移植計画など)
```

## License

MIT
