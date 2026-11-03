# MMC3 の画面分割を fclib の部品にする（raster モジュール）

## 背景

fc でゲームを作るエージェントが MMC3 の IRQ の画面分割（HUD を止めて下をスクロール）に苦労していた（fc-gradius の split.asm。
主の側で仕掛けると処理落ちのフレームだけずれる、縦のスクロールの 4 回の書き込みの時機）。Issue #15 では文書（raster.md）だけを
入れていた。ユーザーと相談して部品を作る（2026-11-03）。

## 決めたこと

- 主の側は「境目の走査線とその下のスクロール」を書くだけ。IRQ の中身・時機合わせ・処理落ちのフレームの仕掛けは fclib の asm
- BG の更新（VRAM のキュー）とは独立。結合は frame の NMI の順序だけ（スクロールを書いた後に仕掛ける・処理落ちの道も通る・
  wait の後のフレームだけ表を入れ替える）。制約は「NMI（frame.hook を含む）が最初の境目の走査線より前に終わる」
- mmc3 ではなく別のモジュール raster（mmc3 に入れて frame を use すると、自前の NMI のプロジェクトで _interrupt がぶつかる）
- 時機は Mesen と QuickNES の画面の画素の比較で決め、テスト（TestRasterSplit）で見張る。CI では内蔵のランナーの IRQ の回数
  （TestRasterIrqChain）
- fcmesen.py に --png / --shot（Mesen の emu.takeScreenshot）を足した（時機の比較に使った）

設計と数の決め方は wiki/design/raster-split.md。
