# ビルドと実行を分ける・基準ディレクトリの正規化

## 背景

ユーザーの指摘（2026-10-06）:

- `Compiler.Build()` が実行まで含めているのはおかしくないか
- driver の `""` を `"."` にするコードが散らばっているのを整理したほうがよくないか

調べると次のようになっていた。

- 実行は 2 か所に分かれていた: emu は driver の `build` の最後（`BuildOptions.Run`）、NES は `pkg/fc` の `Build` が driver から
  戻った後
- `BuildOptions` に実行の項目（`Run`・`Stdout`・`LogOut`・`MaxCycles`）、`Result` に `ExitCode`・`Cycles` があった
- `fcc test -t emu` には実行の上限が無かった（終わらないテストのプロセスが 23 時間残った件）
- `""` → `"."` は `newCompilation` と `defaultTarget` の 2 か所。`build` は前段に直す前の `opt.Dir` を、`check` は直した後の
  `c.dir` を渡していて食い違っていた

## 決定事項

- **基準ディレクトリは driver の入口 `newCompilation` で 1 度だけ正規化する**（ユーザーと相談）。
  - 理由: driver の入口（BuildContext・Check・Migrate）はみなここを通り、`pkg/fc` を通らずに driver を呼ぶテストのパッケージも
    多い（bench・doccheck・internal/nes・quicknes など）
  - 相対のまま（`filepath.Clean`）にする。生成物に埋め込むファイルの参照とエラーの位置は基準からの相対で出すため
  - 既定のターゲットも `newCompilation` で `c.dir` から決める
  - CLI の `splitSrc`（表示用の `posDir`）と sema の `Loader`（`sema.Compile` を直接呼ぶ入口）の `""` はそのまま
- **driver はビルドだけ、走らせるのは新しい `internal/runner`**（ユーザーの依頼で実装）。
  - runner は emu（内蔵の 6502）と nes（内蔵の NES のランナー）を選ぶ
  - runner は driver を使わない。`internal/nes` のテストが同じパッケージから driver を使うので、driver が nes を使うと import が輪になる
    （今まで NES の実行が `pkg/fc` にあった理由）。nes のテストで emu を走らせる所は、テストの補助関数で runner と同じ手順にした
  - `@log` の地点は `driver.Result.Log` で返し、出力は `fclog.Stepper`（r6502 に依存しない形）
  - `pkg/fc` は `Compiler.Build` と `Compiler.Run`。`Test` は Build と Run を続けて呼ぶ。`Options.Run` / `Options.Stdout` は無くした
  - `fcc test` の実行に上限を入れた（`TestCycles`: NES の CPU の 2 分ぶん、`TestFrames`: 7200 フレーム）
