

## 文法

- 型定義をgo/zigの順序にする。現在の int*[] から []*int の形にする


[ ] struct（レコード型）— memo にはないが 6502 ゲーム開発で最も効く追加。 castle の並行配列パターン（en1〜en8等）を置き換えられる

- ca65相当のアセンブラの組み込み(cgoとか？)

- bool(true/false)の追加 ✅ 2026-09-14（比較の結果型はまだ uint8）

- nullの追加 ✅ 2026-09-14（SoA ハンドルには無し）
- void* のようなポインタのtop型を追加する ✅ 2026-09-14（`*void`、暗黙変換あり）


- interbank call を実装する ✅ 2026-09-15（far call。呼ぶ側の構文は無しで自動。doc/v2_farcall.md）
  - `add(1,2)` のかわりに bank(3).add(1,2)

- cc65 の呼び出し規約（`__fastcall__`）の extern 関数を直接呼べるようにする（2026-09-15 案）。
  NSD（castle の `nsd/include/nsd.inc`）のように asm ライブラリが cc65 の規約で書かれていて、今は `sound.asm` で
  A/X に詰め替えるグルーを手書きしている。案: `function nsd_play_bgm(p:*void):void options(abi: "cc65");`
  - 対応範囲: 引数 0〜1 個（最後の引数を A (8bit) / A,X (16bit: A=下位, X=上位) で渡す）、戻り値 void / 8bit (A) / 16bit (A,X)。
    引数 2 個以上は cc65 のパラメータスタック（`sp` と `pushax`）が要るので対象外（cc65 の C ランタイムはリンクしていない）
  - 呼び先は A/X/Y を壊すので、呼び出し側で X（フレームポインタ）を退避・復帰する（`txa; pha … pla; tax`）
  - far call と組み合わせる場合、参考トランポリンは A/Y を壊すので、A/X を FC_FARCALL の後ろ（+3,+4）に置いて
    トランポリンがジャンプ直前に `lda`/`ldx` する変種（`farcall_ax`）が要る。または cc65 規約の関数は固定バンク限定にする

structに関しては、AoS（通常の構造体）だけではなくSoAの形も対応したい。

```
// 通常のstruct
struct Point {
  x: int;
  y: int;
}

// 通常の array of struct
// メモリ上は、[x0, y0, x1, y1, x2, y2,...] に並ぶ
var soa_points: [4]Point; 

// sparse struct (struct of array)の定義
type AoSPoint = sparse [4]Point; // 文法は仮、いいアイデアある？


sparse struct は、配列のコンテナも固定して含む型扱い。つまり型と変数（const）が１対１対応する。
AoSPointは、メモリ上は、[x0, x1, ..., y0, y1, ...] に並び、固定のメモリアドレスも

AoSPointは、ポインタのみを使用できて、ポインタの実態は実際にはuint8（つまり要素数は256個以下限定）

var p: *AosPoint = 0 as *AosPoint;

p.x = 1;
p.y = 2;

p++;
p.x = 3;
p.y = 4;

sparse array はbyte単位で細切れになる。
これにより, 構造体メンバーにX or Yレジスタで効率的にアクセスできる

ldy ?                   ;; ?は、aos_pointsのインデックス
lda AoSPoint_Base+0, y  ;; p.x
lda AoSPoint_Base+1, y  ;; p.y


```


## FC BUG

追加要望
* fc: min, max, clamp の実装 ✅ 2026-09-15（組み込み。型ごとの関数は不要）
* fc: frame size overの制限を緩める => ちゃんとregister spillを実装する
* fc: switch文の問題 => まとめて対処したいので保留
  * fc: switchのcaseがかぶってもエラーにならない
  * fc: switchのcaseが空文だとエラー
    * switchの二分探索/jump table化もやりたい

未確認
* fcのバグ: sintのかけ算がだめ。特定の順序だとだめ => 確認できず
* fc: functionのnull対応 => 言ってる意味が自分でもわからん
* fc: if( vy > 0 || boxelev.box_idx == -1){ ... の || の挙動がおかしい、両方falseなのに、then節が実行されることがあった => intの比較のバグだった

