# 結果を使わない比較が残る・`x & 0` と定数の switch を畳まない（最適化の取りこぼし）

- 出典: ローカル（2026-10-18 のバグ調査の Phase 4。asm のフラグの検査 `verifyCompareFlags` を作ったときに見つけた）
- 状態: **未修正**（出力は正しい。サイクルとバイトの無駄）

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

## 直すときの候補

- ssa の定数畳み込みで `x & 0` → 0（`x | 0xff…`、`x * 0` も）
- 畳み込みで入力が要らなくなった後にも DCE を回す（どの段が `& 0` を消したかは未調査。`FC_DISABLE` で段を切って探す）
- タグが定数の `switch` を `jmp` に
- 直したら、`verifyCompareFlags` の「使われない比較」もエラーにできる（今はエラーにしていない）

## 関連

- `internal/codegen/verify.go` の `verifyCompareFlags`
- `Agent/wiki/plans/bug-hunt.md`（Phase 4）
