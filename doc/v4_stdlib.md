# fc 4 の標準ライブラリ（fclib）の作り直しの計画

2026-09-29。[v4_plan.md](v4_plan.md) §3 の「標準ライブラリの大幅な拡充」の中身の案。**まだ案**で、§8 の「決めること」を決めてから
実装に入る。

前提（2026-09-29 ユーザー決定）:
- **今の fclib との互換はほぼ考えず、作り直す**（今の API は残さない。castle・miku などの呼び出しは新しい API に直す）。これで
  v4_plan §3 の「今の API を残すか」「`use mem;` を版で引き分けるか」は不要になった
- **書式は snprintf に当たるもの（バッファに書く）だけを作り、printf はそのラッパーにする**（§4）

---

## 1. 調べたこと

### 1.1 今の fclib（fc 4、377 行）

mem（set / zero / copy / compare は asm、strlen / strcpy）、math（sin / atan の i8 の表、表引きの rand、sign、abs）、pad（1P だけ、
`pushed` の立ち上がり）、stdio（emu はホストへ、NES は描画を止めている間に PPU へ直接書くデバッグ表示）、lzw / rle / inflate の
unpack（出力の大きさを受け取らない。inflate は public でなくテストも未完成）、unittest。どれもポインタ + 長さか終端 0 で、slice を
受け取るのは `print_slice` だけ（NES 版は 1 文字ずつ `print` を呼ぶ）。NES の printf の整数は 16 進（emu は 10 進）。組み込みが fclib の
名前に結びついている: `printf` → `stdio.print` / `print_slice` / `print_int16` / `print_sint16`、`@copy` → `mem.copy`、`cos` →
`math.sin`、`@run_tests` → `stdio.init` / `print` / `exit`。

### 1.2 castle・miku が自前で持っているもの（2026-09-29 に数えた）

どちらのゲームも fclib の stdio・nes の PPU まわりは使わず、自分の ppu・mmc3 モジュールと NMI を持っている。**slice は 1 か所も
使っていない**（配列 143、ポインタ + 長さの引数 16 関数、`&arr[i]` 99 か所、添字の for 99 か所）。printf も使っていない。

| 分類 | 中身 | 規模・重複 |
|---|---|---|
| VRAM の転送キュー | `ppu.put(addr, ptr, size, flag)`（8 件のキュー、横 / 縦 / 属性、満杯なら vblank を待つ）と NMI での吐き出し（asm）。キューはポインタを持つので、データを NMI まで生かすアリーナ `ppu.alloc`（castle 51 か所）が要る | 両方にほぼ同じコード |
| 描画を止めて書く | `lock` / `unlock`、`put_in_lock`、`fill_in_lock` | castle 45 + 16 か所、両方 |
| 画面の番地 | `ppu.pos(x, y)`（castle 56 か所）。ネームテーブルの `+0x400` などは手で足す（35 か所） | 両方 |
| 属性・メタタイル | 属性の 2 ビットの欄の差し込み（2 か所に重複）、16×16 の 2×2 タイル + 属性（番地の式が 4 か所に重複） | castle |
| スプライト | 自動割り付けの `sprite(x, y, pat, attr)`（castle は毎フレーム向きを変えてちらつかせ、先頭と末尾を予約）、使わない分を隠す、16×16 のメタスプライト（castle 約 180 か所） | 両方 |
| 文字・数 | 2 段組み（濁点が上の段）の文字の配置が 5 つほぼ重複、3 桁の 0 埋めの 10 進が 2 つ重複、`tail += mem.strcpy(...)` の文字列の組み立て。文字コードは独自（`@textmap`、数字の 0 がコード 5） | castle |
| 入力 | `pad.pushed` だけ（離した・押し続け・自動の繰り返しは無い）、メニューのカーソルの `(cur + 1) % n`（7 か所） | 両方 |
| 乱数 | `math.rand` を種なしで。`rand() % 2^k` が約 45 か所、重み付きの選択が 2 つ重複 | 両方 |
| 小数の移動 | 固定小数の型は無く、速度は i8 の 1/16 ピクセル、`pos += (v + giff) / 16`（`giff` はフレームで回すビット反転の表。表は両方で定義） | castle 約 40 か所、miku も |
| 三角関数 | 「相手への角度」`math.atan(ty/16 - y/16, tx/16 - x/16)`（14 か所。今の atan は ±15 までしか正しくないので 16 で割っている）、「角度へ進む」 | 両方 |
| 当たり判定 | `math.abs(a - b) < size`（29 か所。i8 の abs なので 128 未満だけ）、矩形の重なり、画面外 | 両方 |
| ビット | ビット集合の get / set / clear が 4 つ重複、2 ビットの欄 | castle |
| 待ち・フェード | N フレーム待つ for（13 か所）、パレットの明るさを上げ下げして送って待つ繰り返し（約 12 か所） | castle |
| バンク | MMC3 の `set_pbank` / `set_cbank`（シャドウ、同じなら書かない、NMI と衝突しない `select_shadow` の決まり）、「切り替えて呼んで戻す」（約 10 か所）、画面の状態の保存と復帰 | castle（miku の mmc3 は未使用） |
| そのほか | ヒープ（256 バイト、呼ぶのは文字列の複製だけ）、オブジェクトの表と関数ポインタの表、リングバッファ、停止 `@asm(".byte 2")`（assert の代わり） | castle |

castle のうち 256 バイトを超えるバッファ（`[:u16]` が要る）: savedata 512、ppu.gr_buf 512、bg.data_buf 2048、nsd_buf 343。

### 1.3 ほかの 6502 のコンパイラの標準ライブラリ

調べたもの: cc65（libc と nes）、neslib（Shiru）と nesdoug の拡張、llvm-mos SDK、NESFab、Millfork、prog8、Oscar64、KickC。
NES 向けのライブラリがおおむね共通に持っているもの（よく見るものから）:

1. NMI でのフレームの同期（`ppu_wait_nmi`）とフレームの数
2. $0200 の OAM のシャドウと NMI での DMA、`oam_spr`（次の番号を返すか、ゼロページのカーソル）・`hide_rest`・clear
3. パッドの読み取りと立ち上がり（held / pressed / released。DMC の読み間違いの対策）
4. PPU のレジスタの定数、番地の計算（`NTADR(x, y)`、属性の番地）
5. NMI で吐き出す VRAM の更新バッファ（番地 + 長さ + 横 / 縦、終端）と、描画を止めて直接書くもの
6. NMI で送るパレットのバッファと明るさ・フェード
7. 8 / 16 ビットの乱数（LFSR が多い）と種（フレームの数・入力のタイミング）
8. メタスプライト（ずれの並び。良いものは画面の端で切り、反転を扱う）
9. ネームテーブルの展開（RLE はどこにでもある）
10. マッパー（PRG / CHR の切り替え、バンクをまたぐ呼び出し、ミラーリング、MMC3 の走査線 IRQ）
11. 音のドライバの呼び出し口（NMI から）
12. 割り算を使わない 10 進の桁（得点）
13. 固定小数と、256 段の角度の sin / cos / atan2、乗除算

参考にしたい考え:
- **neslib**: パレットは明るさごとの 9 つの表を指すだけで、NMI が表を通して送る（フェードの 1 段が主の処理でただ）。VRAM の更新の
  形式は SMB の「stripe」（`[hi | 横 0x40 / 縦 0x80, lo, 長さ, データ…]`、終端 0xFF）。**バッファのあふれを検査しないのが一番よく
  言われる不具合**
- **llvm-mos**: NMI と初期化を、使う機能の断片だけリンクで組み立てる（使わない機能は NMI のサイクルも ROM も食わない）。スプライトの
  番号を引数でなくゼロページのカーソルにした（`oam_spr` が約 11% 速い）。静的フレームの重ね合わせは fc と同じ考え
- **NESFab**: `struct Pad {held, pressed, released}`、パッドの読み取りを OAM DMA に合わせて DMC の読み間違いを避ける、割り算の無い
  `uu_to_ddddd`、メタスプライトの反転を属性の XOR とずれの反転で、256 段の角度の `atan2(SS, SS)` と `point_dir`
- **prog8**: モジュールの分け方（`sys` / `txt` / `math` / `strings` / `conv`）と、数の文字列化の名前の格子
  （`str_{ub,uw,b,w}{,0,hex,bin}`）
- **cc65 の nes は反面教師**: 端末のような作り（768 バイトのリングバッファが $0200 の OAM とぶつかる、読み込み式のドライバ）で、
  実際には誰もが neslib に置き換えている

fc は静的フレーム・inline・asm の `abi: "frame"`・使わない関数の除去を既に持っているので、llvm-mos や Oscar64 がコンパイラに
作り込んだものの多くはライブラリの側で要らない。

---

## 2. 方針

1. **slice で受け取る**。データの引数は `[]const T` / `[]T`、256 バイトを超えうるもの（mem・展開）は `[:u16]T`（`[]T` から暗黙に
   なる）。書いた分は長さ（`u8` / `u16`）で返す（slice を返すと 1 回約 +20 サイクル: v3_slice_tradeoffs.md §2）
2. **確保しない**。書き先は呼ぶ側が渡す。足りなければ普通の版は止まり、`try_` の版は `bool` を返して何も変えない
   （v3_slices_vector.md §1 の決定）
3. **generics は無いので**、型によらない操作は組み込み（`@copy`・`@format`・`@min` / `@max` / `@clamp`・`@len`）、ライブラリの関数は型ごとに
   名前を分ける（`_u8` / `_u16` / `_i8` / `_i16`。prog8 の格子に近い）
4. **使わなければただ**: 使わない関数は出力されない（既にそう）。NMI は使う機能だけが動くようにする（§6.2）
5. **速さの要る所は asm（`abi: "frame"`）、それ以外は fc**。asm の関数のフレームはゼロページを食うので小さく保つ
6. **NMI の中で fc の関数を動かさない**: 割り込みから呼ぶ fc のコードは乗除算・ポインタ経由の読み書きが共有の `reg` を壊しうる
   （v3_plan.md §6、直さない）。NMI 側はライブラリの asm だけで、ゲームの処理は主の側から「次の NMI で送る」ものを積む
7. **デバッグの表示とゲームの文字を分ける**: `console`（ホストか、描画を止めた画面へのデバッグ出力。printf の先）と、ゲームの画面へは
   `vram.put(addr, @format(buf, ...))` のように slice を渡す
8. **固定のバンク**（`@(bank: -1)`）に置く。バンクをまたぐデータを読む関数は、読む間のバンクは呼ぶ側の責任（今までどおり）

---

## 3. モジュールの構成（案）

### 3.1 どのターゲットでも使うもの

| モジュール | 中身 | 今の fclib から |
|---|---|---|
| `mem` | fill / zero / copy / move（重なってよい）/ equal / compare / find | mem を slice に |
| `fmt` | 数を文字にする（10 進は割り算を使わない、16 進、2 進、幅と埋め）。`@format` の中身 | 新規 |
| `str` | バイト列としての文字列: equal / starts_with / find / 終端 0 との橋渡し / 文字コードの変換（独自のフォント） | strlen / strcpy を置き換え |
| `buf` | 書き足していくバッファ（`struct Buf`）: push / append / try_ / 中身の slice | 新規（castle の `tail += strcpy` の置き換え） |
| `math` | abs / sign（型ごと）、256 段の sin / cos、全範囲の atan2・「相手への角度」、8×8 → 16 の掛け算、平方根、カーソルの wrap、1/16 ピクセルの速度の丸め | math を作り直し |
| `rand` | 16 ビットの乱数（seed / 8 ビット / 16 ビット / `below(n)` / `chance(p)` / 重み付きの選択） | math.rand を置き換え |
| `bits` | ビット集合（get / set / clear / flip / count）、2 ビットの欄 | 新規（castle の 4 つの重複） |
| `hit` | 1 軸の重なり、矩形の重なり、近さ（u8 の座標で符号の問題を起こさない書き方） | 新規（castle 29 か所・miku） |
| `lzw` / `rle` | 展開。書き先の slice を受け取り、足りなければ止まる / `try_`。rle は NES Screen Tool の形式 | 今のものを slice に。inflate は外す（案） |
| `console` | デバッグ出力（write / newline / exit）。emu はホスト、NES は描画を止めた画面に内蔵のフォントで。printf の先 | stdio を置き換え |
| `test` | assert（`@format` の文言）、テストの実行（`@run_tests`） | unittest を置き換え |
| `sys` | `panic(msg)` / `assert(cond, msg)`（emu は表示して終了、NES は console に出して止まる）、止める命令 | 新規（`@asm(".byte 2")` の置き換え） |

API の素案（名前・引数は §8 で詰める）:

```fc
// mem
public function fill(dst:[:u16]u8, v:u8):void;
public function zero(dst:[:u16]u8):void;
public function copy(dst:[:u16]u8, src:[:u16]const u8):u16;   // 短いほうの長さ。前から (dst が src より後ろで重なるなら move)
public function move(dst:[:u16]u8, src:[:u16]const u8):u16;   // 重なってよい
public function equal(a:[:u16]const u8, b:[:u16]const u8):bool;
public function compare(a:[:u16]const u8, b:[:u16]const u8):i8;
public function find(s:[:u16]const u8, v:u8):u16;              // 無ければ @len(s)

// fmt (書いた長さを返す。dst が足りなければ止まる)
public function dec_u8(dst:[]u8, n:u8, width:u8, fill:u8):u8;
public function dec_u16(dst:[]u8, n:u16, width:u8, fill:u8):u8;
public function dec_i8(dst:[]u8, n:i8, width:u8, fill:u8):u8;
public function dec_i16(dst:[]u8, n:i16, width:u8, fill:u8):u8;
public function hex_u8(dst:[]u8, n:u8, upper:bool):u8;
public function hex_u16(dst:[]u8, n:u16, width:u8, upper:bool):u8;
public function bin_u8(dst:[]u8, n:u8):u8;

// buf
public struct Buf { data:[:u16]u8; len:u16; }
public function init(storage:[:u16]u8):Buf;
public function bytes(b:*const Buf):[:u16]const u8;
public function push(b:*Buf, c:u8):void;
public function append(b:*Buf, s:[:u16]const u8):void;
public function try_append(b:*Buf, s:[:u16]const u8):bool;

// math
public function sin(a:u8):i8;                 // 256 段の角度、±127 (cos は a + 64)
public function atan2(dy:i16, dx:i16):u8;     // 全範囲 (今の atan は ±15 まで)
public function angle_to(x0:u8, y0:u8, x1:u8, y1:u8):u8;
public function abs_i8(x:i8):u8;
public function abs_i16(x:i16):u16;
public function sign_i8(x:i8):i8;
public function mul_u8(a:u8, b:u8):u16;       // 8×8 → 16 (ランタイムの __mul_8t16)
public function sqrt_u16(n:u16):u8;
public function wrap_inc(x:u8, n:u8):u8;      // メニューのカーソル
public function wrap_dec(x:u8, n:u8):u8;
public function subpixel(v:i8, phase:u8):i8;  // (v + 表[phase]) / 16: 1/16 ピクセルの速度をフレームで散らして丸める

// rand (16 ビットの xorshift か Galois の LFSR: §8)
public function seed(s:u16):void;
public function next_u8():u8;
public function next_u16():u16;
public function below(n:u8):u8;               // 0..n-1 ((next_u8 * n) >> 8)
public function chance(p:u8):bool;            // p / 256
public function pick(weights:[]const u8):u8;  // 重み付き

// bits
public function get(set:[]const u8, i:u8):bool;
public function set(set:[]u8, i:u8):void;
public function clear(set:[]u8, i:u8):void;

// hit
public function span(a:u8, aw:u8, b:u8, bw:u8):bool;                 // [a, a+aw) と [b, b+bw) が重なる (`b + bw - a` の符号なしの比較)
public function box(ax:u8, ay:u8, aw:u8, ah:u8, bx:u8, by:u8, bw:u8, bh:u8):bool;
public function near(a:u8, b:u8, d:u8):bool;                         // |a - b| < d (u8 のまま)
```

### 3.2 NES（`nes/`）

| モジュール | 中身 | castle・miku の相当 |
|---|---|---|
| `nes` | レジスタ（今の名前 `PPUCTRL` / `PPUMASK` / `PPUSTATUS` / `OAMADDR` / `PPUSCROLL` / `PPUADDR` / `PPUDATA` / `OAMDMA`、APU、`JOY1` / `JOY2`）とビットの定数 | nes.fc |
| `frame` | NMI（asm）とフレームの同期: init（NMI を有効に）、wait（次の NMI まで。その NMI で OAM・パレット・VRAM のキューを送る）、wait_n、count、render_off / render_on、scroll | `wait_vsync_with_flag`、lock / unlock、NMI の asm |
| `vram` | 番地（`addr(nt, x, y)`・属性の番地）、キュー（put 横 / put_v 縦 / fill / set / try_ / room）、描画を止めている間の直接書き（write_now / fill_now）、属性の 2 ビットの欄の更新、2×2 のメタタイル | ppu.put / alloc / pos / put_in_lock |
| `pal` | 32 バイトのパレットを NMI で送る: set_all / set、明るさ（neslib の表を指すだけの方式）、fade（待つ版） | pal_down / pal_up と繰り返し |
| `oam` | $0200（案）のシャドウ: begin（カーソルを戻す。ちらつかせの向きの切り替え）、spr、meta（画面の端で切る・反転）、固定の番号、reserve、hide_rest（frame.wait が呼ぶ） | sprite / gr_sprite2 / clear |
| `pad` | 2 つのパッド、`struct Pad {held, pressed, released}`、poll（2 回読んで一致するまで: DMC）、ボタンの定数、（後で）押し続けの繰り返し | pad.fc |
| `mmc3` / `mmc1` / `uxrom` | PRG / CHR の切り替え（前の値を返す: 「切り替えて呼んで戻す」）、ミラーリング、MMC3 の走査線 IRQ、WRAM。far call のトランポリン（`farcall_*.asm`）とシャドウ・`select_shadow` の決まりを共有 | castle の mmc3.fc / macro.asm |
| `sound` | 最初は NMI から呼ぶ音のドライバの呼び出し口だけ（NSD・FamiStudio などは利用者が持つ） | castle の sound.fc |

```fc
// frame
public var count:u16;
public function init():void;
public function wait():void;
public function wait_n(n:u8):void;
public function render_off():void;
public function render_on():void;
public function scroll(x:u8, y:u8, nt:u8):void;

// vram (キューはデータを写す stripe 形式: 2026-09-29 決定。§8 の 4)
public function addr(nt:u8, x:u8, y:u8):u16;
public function attr_addr(nt:u8, x:u8, y:u8):u16;
public function put(addr:u16, data:[]const u8):void;     // 横。満杯なら次の NMI を待ってから積む
public function put_v(addr:u16, data:[]const u8):void;   // 縦
public function fill(addr:u16, v:u8, n:u8):void;
public function try_put(addr:u16, data:[]const u8):bool;
public function reserve(addr:u16, n:u8):[]u8;           // キューの中に n バイトの場所を取って返す (そこへ直接書く: 写しが要らない)
public function room():u8;
public function write_now(addr:u16, data:[:u16]const u8):void;   // 描画を止めている間
public function fill_now(addr:u16, v:u8, n:u16):void;

// pal
public function set_all(p:[]const u8):void;
public function set(i:u8, c:u8):void;
public function bright(level:u8):void;       // 0 (黒) .. 4 (そのまま) .. 8 (白)
public function fade(to:u8, frames:u8):void; // 1 段ごとに frames フレーム待つ

// oam
public function begin():void;
public function spr(x:u8, y:u8, tile:u8, attr:u8):bool;   // 満杯なら false
public function meta(x:i16, y:i16, m:[]const u8, flip:u8):void;   // {dx, dy, tile, attr} の並び
public function reserve(n:u8):void;

// pad
public struct Pad { held:u8; pressed:u8; released:u8; }
public var p1:Pad;
public var p2:Pad;
public function poll():void;
```

---

## 4. 書式: `@format` と printf

**`@format(dst, "書式", 引数...)`**（組み込み）: `dst`（`[]u8`）に書き、書いた部分の slice を返す（snprintf に当たる。Zig の
`bufPrint`）。足りなければ止まる。`@try_format` は書けなければ `false` の版（形は §8）。
- 書式は `@log` と同じ（`{}` / `{0}` / `{:x}` / `{:04X}` / `{:b}` / `{:c}` / `{:d}` / `{:5}`、`{{` / `}}`）。**書式は定数で、
  コンパイル時に分解**して、文字の部分の写しと `fmt.*` の呼び出しの並びにする（実行時に書式を読まない。引数の型の誤りはコンパイル
  エラー）
- 引数: 整数（型で 10 進の符号の有無が決まる）、bool、enum（数か名前か: §8）、`[]const u8`（そのまま写す）、`{:c}` は 1 バイト
- 使う例: `vram.put(vram.addr(0, 2, 3), @format(line, "HP {:3}/{}", hp, max_hp));`
- 展開のイメージ（`hp:u8`、`max_hp:u16`）。書式文字列そのものは ROM に入らず、文字の部分だけが入る。定数の引数はコンパイル時に文字に
  して文字の部分に畳み込める。書き先が足りなければ各関数が止まる（`@try_format` は失敗を返す）:

  ```fc
  var s = @format(buf, "HP {:3}/{}", hp, max_hp);
  // ↓
  var n:u8 = 0;
  n += mem.copy(buf[n..], "HP ");                // 文字の部分を写す (短ければ 1 バイトずつの代入に)
  n += fmt.dec_u8(buf[n..], hp, 3, ' ');         // {:3}: 幅 3 の 10 進
  buf[n] = '/'; n += 1;                          // 1 文字の部分は直接代入
  n += fmt.dec_u16(buf[n..], max_hp, 0, ' ');    // {}: 型が u16 なので dec_u16
  s = buf[..n];
  ```
- **実行時に書式を読む整形器は作らない**（2026-09-29 決定。実行時に言語を切り替えるなど、要るようになったら別のライブラリにする）

**printf**: `printf("書式", 引数...)` は `console` に出す。`@format` と同じ書式・同じ `fmt` の関数で、実装は小さな一時バッファに書いて
`console.write` に渡すラッパー（部品ごとに書けば一時バッファは数の桁の分、8 バイトほどで済む）。NES でも 10 進になる（今は 16 進）。

**独自のフォント**: `@format` は ASCII を書く。文字コードが ASCII と違うゲームは `str.map(s, 表)` でその場で変換する（castle の
`@textmap`）。数字だけ違うなら `fmt` の関数の `zero` の引数の案もある（§8）。

---

## 5. コンパイラとの結びつき

作り直しに合わせて、fclib の名前に結びついた組み込みを直す:

| 組み込み | 今 | 案 |
|---|---|---|
| `printf` | `stdio.print` / `print_slice` / `print_int16` / `print_sint16` を型で呼び分け | 書式をコンパイル時に分解し、`fmt.*` と `console.write` を呼ぶ |
| `@format` | 無い | 新規（上） |
| `@copy` | `mem.copy`（`use mem;` が要る） | `mem.copy`（slice の版）。`use` を要らなくするか（§8） |
| `cos` | `math.sin(x + 64)` のマクロ | 新しい `math.sin` に |
| `@run_tests` | `stdio.init` / `print` / `exit` | `test` / `console` に |
| `@log` | エミュレータ側の表示 | そのまま（書式の解析 `sema/log.go` の `parseLogFormat` / `parseLogSpec` を `@format` と共有する） |

---

## 6. メモリと NMI

### 6.1 RAM とゼロページ

ライブラリが持つ RAM（案）: OAM のシャドウ 256（ページの先頭。DMA のため）、VRAM のキュー 128〜192（NMI の時間で送れるのは
約 160 バイト + OAM DMA）、パレット 32、パッド 6、フレームの数 2、乱数 2、NMI の印 1。OAM のシャドウの位置はハードウェアでは
自由（$4014 に書いたページ N の $N00〜$NFF を送る。256 バイト境界にそろっていればどこでもよい。$0200 は neslib などの慣習）だが、
fc の既定の配置では fc が $0200〜$06FF を使うので、空いているのは $0700 かカートリッジの RAM だけ（language_reference §4.5。
castle・miku も $0700）。fc には変数を境界にそろえる指定が無いので、**既定は $0700 に固定し、`@(build)` の定数（`-D` で変えられる）で
ページを変えられるようにする**（案。§8）。
ゼロページは asm の関数のフレーム（`abi: "frame"`）とキューの位置くらいにとどめる。

### 6.2 NMI

ライブラリが NMI を持つ（案）。順に: OAM DMA（`oam` を使っていれば）→ パレット（変わっていれば）→ VRAM のキュー → スクロール・
PPUCTRL / PPUMASK → フレームの数 → 利用者の asm の呼び出し口（音のドライバ・ラスター割り込みの準備）。主の側は `frame.wait` で
NMI を待つだけ。**OAM と VRAM のキューを送るのは、主の側が `frame.wait` で「このフレームの分ができた」印を立てたときだけ**にする
（処理が間に合わず並べている途中で NMI が来たフレームは送らない。PPU の OAM には前の完成したフレームが残るので、OAM を 2 ページの
ダブルバッファにしなくても半端なスプライト・半端なキューの項目を見せない。スクロールと音は毎フレーム）。使わない機能を NMI から外すのは、最初は実行時の印（neslib と同じ）で、あとで llvm-mos のようにリンクで組み立てる
方式を考える。今の「stdio の NMI と利用者の NMI がぶつかる」問題（roadmap の「fclib と NES の開発の流れ」）もここで片づける。
castle の raster IRQ（irqcmd）のような凝ったものは、利用者の asm の呼び出し口で続けられるようにする。

---

## 7. 進め方

1. **決める**: §8 を決め、この文書を仕様にする
2. **どのターゲットでも使うもの**（emu で確かめられる）: mem / fmt / `@format` と printf / str / buf / math / rand / bits / hit /
   sys / test / console（emu）。組み込み（§5）を直し、test/ の golden を更新する。`fmt` と `@format` は Go の `strconv` を参照にした
   fuzz、mem は参照の実装との比較、math は表をすべての入力で確かめる
3. **NES の土台**: nes / frame / vram / pal / oam / pad / console（NES、内蔵のフォントの CHR）。「hello world」の最小の例と、内蔵の
   NES ランナーでの確かめ（PPU の中身、NMI のサイクル数）
4. **広げる**: mmc3 / mmc1 / uxrom（far call と合わせて）、lzw / rle の slice の版、メタスプライト・メタタイル・属性、フェード、音の
   呼び出し口
5. **移す**: miku を新しい API で書き直して確かめる（小さいので最初の実例に）。castle をどうするか（製品のコード）は別に決める。
   古い fclib を消す

---

## 8. 決めること

1. **モジュールの分け方と名前**（§3）。とくに `console`（今の stdio）、`fmt` と `str` と `buf` を分けるか
2. **`@format` の形** → **書式文字列（`@log` と同じ `{}`）に決定（2026-09-29）**。以下は決める前の検討: 書式文字列（推し）か、引数を並べて修飾する形（`@format(buf, "HP ", @dec(hp, 3))`）か。
   国際化には書式文字列が向く（言語で数の位置が動く `"Got {} coins"` / `"コインを {} まい"`、位置指定 `{1}` で引数の順も文字列の側で
   変えられる、文言を表として翻訳者に渡せる）。書式はコンパイル時に分解するので定数に限る: 言語ごとに ROM を作る（`@(build)` の
   定数で選ぶ。ファミコンでは普通）なら問題ない。実行時に言語を切り替えるなら、定数の書式の表（`MSG[lang]`）をコンパイル時に全部
   展開して言語で分岐するか、実行時に書式を読む小さな整形器を別に作る（どちらも要るときに。実行時の整形器は別のライブラリにする:
   2026-09-29）。並べる形にすると、今の test/ と fuzz の
   printf（並べる形）はそのまま使えるが、書式文字列にするなら migrate の規則で機械的に直す
   `@try_format` の返し方（`bool` と書いた長さの struct か、長さ 0 で失敗か）。`Buf` に書き足す版（`@format(&b, ...)`）も要るか
3. **独自のフォントの文字**: `@format` がコンパイル時に書式の文字の部分を `@textmap` で変換する（`{}` を解析してから。実行時の
   手間が無く、国際化の文言もそのまま書ける。推し）か、ASCII で書いて `str.map` で実行時に変換するか、`fmt` に `zero` の文字を渡すか
4. **VRAM のキュー** → **データを写す stripe 形式に決定（2026-09-29）**。以下は決める前の検討: データを写す stripe 形式（推し。アリーナとデータの寿命の問題が無くなる。castle の `en7.fc` のアリーナのあふれの
   ような誤りが起きない）か、castle と同じくポインタを積む形か。見積もり（命令表から。実測は §7 の 3 で）: NMI の側は写す形のほうが
   速い（castle の `lda (from),y` / `sta` / `iny` / `cpy` / `bne` は 1 バイト約 17 サイクル、キューが固定の番地なら `lda buf,x` / `sta` /
   `inx` / `dey` / `bne` で約 15、展開すればさらに縮む。vblank は OAM DMA を除いて約 1700 サイクルで、送れるのは約 100 バイト）。主の
   側は写す分（1 バイト約 15〜20 サイクル）が増えるが、castle はバンクの都合で既に `ppu.alloc` のアリーナへ写していて、それの置き換えに
   なる。その場で作るデータ（文字・数・埋め）は `vram.reserve` でキューの中へ直接書けば写しも無い（`@format(vram.reserve(a, 8), ...)`）。
   写す形が損なのは固定のバンクの大きな表を毎フレーム送るときだけで、要ればポインタを積む種類の項目を後で足す
5. **OAM のページの位置**: 既定を $0700（castle・miku と同じ。fc の既定の配置で空いている唯一のページ）にして `@(build)` の定数で
   変えられるようにする（推し）か、fc の既定の配置をずらして慣習の $0200 を空けるか
6. **NMI の持ち主**: ライブラリが持って呼び出し口を出す（推し）か、NESFab のように利用者が NMI を書いてライブラリの送る関数を呼ぶか
7. **slice の幅**: mem と展開は `[:u16]`、文字・VRAM・数は `[]`（推し）でよいか
8. **失敗の扱いの名前**: `put`（止まる）/ `try_put`（`bool`）でそろえるか。止まるときの動き（emu は表示して終了、NES は console に
   出して止まる）
9. **乱数の方式**: 16 ビットの xorshift / Galois の LFSR（種を入れられる）か、今の 256 バイトの表（速いが種の意味が薄い）も残すか
10. **展開の形式**: lzw（castle が使う）と rle（NES Screen Tool）を残し、inflate（未完成）を外すか
11. **固定小数**: 1/16 ピクセルの速度を散らして丸める `math.subpixel`（castle・miku の書き方）だけにするか、8.8 の補助（上位 / 下位、
    掛け算）も入れるか
12. **castle・miku をいつ・どう移すか**: miku を最初の実例に書き直す案。castle は製品のコードなので判断をもらう

---

## 付録: 調べている途中で見つけた不具合（castle・miku。製品のコードなので直していない）

- castle `heap.fc:68` / `:77`: 空きかどうかを印のビットでなく印のバイト全体で比べていて、解放したブロックが隣とつながらない
- castle `en7.fc:814`: アリーナから 2 バイト取って 4 バイト写している（823・838 行）
- castle / miku の `ppu.wait()`: `i -= i` で待たない（どこからも呼ばれていない）
- miku `ppu.asm:15`: `sta` のはずの所が `lda _nes_PPU_SPR_ADDR`
- miku `ppu.asm:167`: 残った `sty $100` がスタックのページに書く
