# インストール

fc を使うには、コンパイラの `fcc` と、アセンブラとリンカ（cc65 の `ca65` と `ld65`）が要ります。
`fcc` は fc のソースを 6502 のアセンブリにし、`ca65` と `ld65` を呼んで ROM にします。

::: info fc 4 はまだリリースしていません
リリースのページにあるバイナリは fc 4 より前のものです。fc 4 は GitHub の `main` ブランチのソースから `fcc` をビルドします。
:::

## Go と cc65 を用意する

- **Go** 1.24 以上（[go.dev/dl](https://go.dev/dl/)）。`fcc` のビルドに使います
- **cc65**（`ca65` と `ld65`）
  - Windows: [cc65 のスナップショット](https://sourceforge.net/projects/cc65/files/cc65-snapshot-win32.zip)を展開し、中の `bin` にパスを通します
  - macOS: `brew install cc65`
  - Linux（Debian / Ubuntu）: `sudo apt install cc65`

## fcc をビルドする

```bash
go install github.com/haramako/fc/cmd/fcc@main
```

`fcc` は `go env GOPATH` の `bin` にできます（そこにパスを通してください）。`fcc` は標準ライブラリを中に持っているので、
1 つのファイルだけでどこでも動きます。

ソースを手元に置いてビルドするなら:

```bash
git clone https://github.com/haramako/fc.git
cd fc
go build -o fcc ./cmd/fcc
```

Windows では `-o fcc.exe` にします。

## 確かめる

```bash
fcc version
```

`fcc` の版と、使う `ca65` / `ld65` の場所が出ます。`fcc` は次の順に `ca65` / `ld65` を探します。

1. 環境変数 `FC_CC65_BIN` のディレクトリ
2. `fcc` と同じディレクトリ
3. パスの通った場所

## エミュレータ

NES の ROM を動かすには、NES のエミュレータを使います。[Mesen](https://www.mesen.ca/) を勧めます。
`fcc build -g` で作るデバッグ情報を自動で読み込み、デバッガで fc のソースの行にブレークポイントを置いてステップ実行でき、
関数や変数に名前が付きます。

次は[最初のプログラム](./hello-emu)を書きます。
