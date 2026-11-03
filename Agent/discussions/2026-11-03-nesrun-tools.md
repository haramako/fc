# ROM を画面なしで動かす道具（Mesen・libretro）とデバッグのページ

## 背景

fc でゲームを作るエージェントから「QuickNES の libretro コアは RAM を見られない（大きさ 0）ので、実行時のデバッグは画面に
@format して出すしかない」という報告（2026-11-03）。リポジトリに libretro や @log の読み方の利用者向けのガイドも道具も無かった
（`internal/quicknes` はテスト用の Windows の DLL だけ、@log の Mesen の使い方は Agent/ の wiki だけ）。

## 決めたこと（ユーザーと）

- 利用者向けのデバッグのページ（`docs/reference/debug.md`）を作る
- 内蔵の NES のランナーで @log を出す案（`fcc run -t nes -g`）はやらない（ユーザー: fcc run -t nes はあまり使わない）
- libretro の道具と、Mesen をヘッドレスで動かして @log を読む道具と文書を作る（Mesen → libretro の順）

## 分かったこと

- 「RAM が 0」は相手の `qnes.py` が `retro_get_memory_size()` に種類の id を渡していなかったため（2 を渡せば 2048）
- MesenCE の Linux 版は画面の無い機械で `--testrunner` が動く（wiki/mesen-and-debugging.md に確かめたこと）
- libretro には命令ごとのフックが無いので @log は読めない。@log は Mesen（`.fclog.lua`）で

## やったこと

- `tools/nesrun/fcmesen.py`: Lua を作って Mesen の `--testrunner`。@log（`.fclog.lua`）、fc のランナーの番地 $4018 / $4019 の
  書き込み（console の出力・panic の理由・終了コード）、入力の台本、`--until`、`--peek`（`.dbg` の番地）
- `tools/nesrun/fcretro.py`: Python の ctypes で QuickNES。入力の台本、`--watch` / `--peek`、PNG（zlib で自前に書く）
- `internal/nes` の TestNesrunMesen / TestNesrunRetro（道具を壊すと落ちることを確かめた）
