# 結果を使わない比較が残る・`x & 0` と定数の switch を畳まない（最適化の取りこぼし）

- 出典: ローカル（2026-10-18 のバグ調査の Phase 4。asm のフラグの検査 `verifyCompareFlags` を作ったときに見つけた）
- 状態: **修正済み**（2026-10-19。出力は正しく、サイクルとバイトの無駄だった）

## 見つけ方

比較（`cmp` / `cpx` / `cpy` / `bit`）の立てたフラグが 1 つも読まれずに全部上書きされる所を数えた。castle・miku・games の
217 ファイル（比較 2472 個）では 0 個。`TestRandom*` の 4000 本（`-randn 2000` を既定の種と `-randseed 20261018000` で）では
**37 本**に出た。37 本とも `-O 0` と `-O 2` の出力は一致する（誤ったコードではない）。すべて `-O 2` だけ。

## 形

生成器の `fp0[(… 比較 …) & 0]()`（関数ポインタの表を添字 0 で引く）から来るものが多い。種 79（TestRandomPrograms）の `t1`:

```text
; 0120: (:lt {l? $561 #"ubool8"} {l? l1 #"u8"} {l? $560 #"u8"} "w=1")
	lda 0+<S+4,x
	cmp 0+<L+0                 ← 結果の $561 を使う命令が無い
; 0122: (:switch {lit nil 0 #"u8"} {lit nil 0 #"u8"} "@dv_706 @dv_707")
	lda #0
	cmp #2                     ← 定数 0 の switch をジャンプテーブルで引いている
	bcs @161
	tay
	lda @160,y
	...
```

`& 0` の命令（0121）は後ろの段で畳まれて消えたが、その入力の `lt` は残っている（DCE が後で走らない）。さらに、タグが
定数の `switch` を直接のジャンプにしていない。

## 小さな再現（fc 4）

```fc
#fc 4
use console;
var a0:[16]u8;
var n:u8;
function f():void @(noinline) { n = n + 1; }
function g():void @(noinline) { n = n + 2; }
const fp0:[2]fn():void = [f, g];
function t(l1:u8):void @(noinline)
{
	fp0[((a0[7] <= l1) as u8) & 0]();
}
```

ここでは `& 0` 自体が畳まれず、比較 → 真偽値（`bcc` / `lda #1` / `lda #0`）→ `and #0` → ジャンプテーブルがそのまま出る
（2026-10-18 に `fcc build` の .s で確かめた）。

## 直したこと（2026-10-19）

- **`x & 0` を畳む**: ssa の `evalConst` で、`x & 0` / `x * 0` は 0、`x | ~0` は ~0 とする（もう片方が定数でなくてよい）。
  小さな再現の `t` は `jsr _main_f__direct` だけになった（タグが定数の switch は、ssa が元から `jmp` に畳んでいた）
- **使われなくなった比較を消す**: どの段が比較の結果の使い手を消すかを `FC_DISABLE` で段を 1 つずつ切って探すと `jumps`
  だった（分岐を消しても条件の比較が残る）。使われない値にも定義の所だけの生存範囲があり、割付の `DeleteUnuse` は消さない。
  `jumps` の最後に `dropDeadTemps` を足し、結果が誰にも使われない一時の値の純粋な命令を消す。直後の命令が C を読む
  （rol / ror / if_carry）ときは残す（最初は 2 バイトのシフトの下位を消して `TestSplitShiftCarry` が誤った値になった）。
  `@log` の注釈が読む値も残す
- `verifyCompareFlags` の「使われない比較」もエラーにした
- 確かめたこと: 37 本のうちビルドできる組で使われない比較 8 → 0、`-O 0` / `-O 2` の出力は全部一致。fuzz の
  `TestRandom*`（`-randn 2000` を既定の種と `-randseed 20261018000`）で使われない比較も誤ったコードも 0 本。
  castle の ROM は en3 / en4 だけ変わり（en4 の使われない `anim % 4` が消え、en3 は一時の値が減って `rest` が A に常駐）、
  `TestCastleFrameCycles` は同じ
- 回帰テスト `TestBugAndZeroDeadCompare`、bugzoo の `ssa-and-zero` と `jumps-dead-temps`

## 関連

- `internal/codegen/verify.go` の `verifyCompareFlags`
- `Agent/wiki/plans/bug-hunt.md`（Phase 4）
