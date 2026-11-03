# raster: MMC3 の IRQ で画面を分ける（fclib/nes/raster）

確かめた日: 2026-11-03（Mesen 2.2.1 の Linux 版と QuickNES の libretro コアの画面で、internal/nes の TestRasterSplit）。

## 形

- `raster.split(i, B, x, y, nt)` は「次の表」（境目ごとに 5 バイト: B、`$2006` の 1 回目 = nt × 4、Y、X、`$2006` の 2 回目 =
  `((Y & $F8) << 2) | (X >> 3)`）に書く。4 つまで
- frame の NMI がスクロールを書いた後に `frame.raster`（`_raster_nmi`）を呼ぶ。X = そのフレームの ready（frame.asm が渡す。
  処理落ちの道は 0）。bit 7 のときだけ次の表を今の表に写し（vram の途中の flush の $40 では写さない: 書きかけの表を使わない）、
  `$C000 = B - 1` で仕掛ける。NMI の初めに置かないのは vblank の予算（BUDGET は残り約 177 サイクルしか見ていない）のため
- IRQ（`mmc3.irq_hook` = `_raster_irq`）: `$2006`（nt × 4）・`$2005`（Y）をすぐ書き、`RASTER_WAIT` = 5 回の dey / bne で
  境目の 1 本上の走査線の hblank まで待って `$2005`（X）・`$2006` を書く。次の境目まで d 走査線なら `$C000 = d - 2`（d ≥ 3）
- raster は mmc3 と別のモジュール: mmc3 に入れて `use frame;` すると、mmc3 だけを使う（自前の NMI の）プロジェクトに frame の
  `_interrupt` がリンクされてぶつかる

## 数と待ちを決めた方法

分割した画面を、分割しない 2 枚（上の帯のスクロール・境目から下のスクロール）を走査線で継いだものと画素で比べる。

- 仕掛けの数: 境目 2〜239 の全部で、`$C000 = B - 1` で境目がちょうど B になる（docs/reference/raster.md の「数え方」どおり。
  初めの版は `cmp #240` の後のキャリーを 1 と思い込んで `sbc #1` にしていて B - 2 になっていた。キャリーは 0）
- 待ち: 0〜14 を振ると、QuickNES は 3〜12、Mesen は 5〜6 で画素まで合う（Mesen の 4 は境目の 1 本上の右の端の 1 画素、7 は境目の
  左の端の 1 画素がずれる）。IRQ に入るまでのぶれは遅れる側にしか出ないので早い側の 5。7 サイクルの命令（inc abs,x）を回し続ける
  主の側で 20 フレーム続けて撮っても合う
- 連鎖の d - 2 は raster.md の「IRQ の中で次の IRQ を仕掛ける」の QuickNES の結果と同じで、Mesen でも合う
