# リンクの失敗を fc の言葉にする（2026-10-08）

[デバッグ用のゲームで見つかったこと](2026-10-08-debug-games-findings.md) の 2。ユーザーが案のとおり進めることにした。

- **区画があふれたとき**: ld65 の `Segment … overflows memory area … by N bytes` を読み、ld65.cfg から区画の番地と大きさ、失敗しても
  ld65 が書く .map の Modules list から、その区画に載るセグメントのモジュールごとの量を足して大きい順に示す（`internal/driver/linkerr.go`）。
  案内は区画で変える: BSS は配列を小さく・別の RAM（`[ram.NAME]` と `@(segment:)`）、ゼロページは `static_zp` / `fastcall_reg`、
  ROM はコード・データを小さく・別のバンク・PRG とマッパー。位置は入口のソース。リンクの前に fc が自分で量を足す案もあったが、
  asm（runtime・include した .asm）の量も入る ld65 の .map を読むほうが確か
- **CHR RAM（chr = 0）で .chr を @include**: sema が `@include` の位置でエラーにする（`Program.ChrRAM`。`@incbin` で読んで送る案内）
- **失敗したビルドの ROM**: ld65 にはいったん `<出力>.part` に書かせ、成功したときだけ置き換える（失敗すると壊れた .nes が残っていた）
- ほかの ld65 の失敗は「link failed: ld65 returns N」と頭に付け、ld65 の出力はそのまま
