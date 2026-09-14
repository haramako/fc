

## 文法

- 型定義をgo/zigの順序にする。現在の int*[] から []*int の形にする


[ ] struct（レコード型）— memo にはないが 6502 ゲーム開発で最も効く追加。 castle の並行配列パターン（en1〜en8等）を置き換えられる

- ca65相当のアセンブラの組み込み(cgoとか？)

- bool(true/false)の追加 ✅ 2026-09-14（比較の結果型はまだ uint8）

- nullの追加 ✅ 2026-09-14（SoA ハンドルには無し）
- void* のようなポインタのtop型を追加する ✅ 2026-09-14（`*void`、暗黙変換あり）


- interbank call を実装する
  - `add(1,2)` のかわりに bank(3).add(1,2)

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

