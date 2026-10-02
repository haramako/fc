# Starlark のマクロ

2026-10-02。外部コマンドの定数マクロ（`[macro_server.*]`）は別の言語の環境（Go など）が要る。fcc だけで書けて、安全な
スクリプト言語を足せるかの相談。

## 検討した候補

Starlark（go.starlark.net）・Lua（gopher-lua）・JavaScript（goja）・式だけの言語（expr / cel-go）・Go（yaegi）・
WebAssembly（wazero）。Starlark は外界に触れない作り（ファイル・時刻・乱数・OS が無く、渡した組み込みだけ）で、同じ入力から
同じ結果になり、実行の手数の上限も付けられる。Python 風で表の生成に向く。pure Go。

## 決定

- **fcc 以外の依存を 0 にしたい用途として Starlark だけを入れる**（ユーザー）。外部コマンドは何でも書ける逃げ道として残す
- リポジトリで初めての外部の Go モジュール。go 1.24 のまま使える最後のコミット（ffb3f39、2026-03-24。次のコミットから go 1.25）に
  固定した。間接の依存に golang.org/x/sys（int の mmap）。fcc は 9.6 → 11.2 MB
- ファイルを入力にする（ユーザーの問い「指定したファイル（複数可）を入力とする機能は妥当か」）: `read(path)` と `glob(pattern)` を
  組み込みで渡す。読めるのは fc.toml のあるディレクトリの中だけ（`..`・絶対パス・外へのリンクはエラー）
- ライブラリの fc.toml の `[macro_script.*]` は次の段（外界に触れないので許してよい候補）
