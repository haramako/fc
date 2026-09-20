# FC3: slice・vector を使うライブラリ API の利用案

作成: 2026-09-20。[slice・vector 設計案](v3_slices_vector.md) の具体例。
**全て未実装の API 案。型・関数名・構文・失敗時の契約を確定したものではない。**
将来の `#fc 3` 向けの資料であり、現在の実装開始や既存ソースの書き換えを指示するものではない。
既存の標準ライブラリや castle にこの API が既に存在するという意味ではない。

主な利用例は実プロジェクト `C:\Work\castle\src` の `event.fc`、`subtext.fc`、`text.fc`、
`ppu.fc`、`fs.fc`、`common.fc`。バンク付きデータの生成は `tools/banked_buffer.rb` と `tools/converter.rb` も参照した。
castle のコードや生成物は変更せず、FC の言語・ライブラリ設計の資料として記録する。

## 1. 例の前提と役割分担

本書では具体的なコードを書くために、次の仮の規則を使う。
最終的な言語仕様の決定とは分けて扱う。

容量不足は通常版の API では STP 相当の停止を基本とする。
以下は回復する利用例も示すため、`try_append` や `{ok, data}` を返す版を多く使っている。
通常版と回復版の命名は今後統一する。全ての呼び出しで失敗判定を要求する案ではない。
`try!` / 後置 `!` は設計案の短いメモに留め、これらのコード例では前提にしない。

- `[]T` は slice、`[]const T` は読み取り専用の slice。`.len:uint16` と `.ptr` を持つ。
- `s[start..end]` は半開区間。`s[..]` は全体、`s[start..]` は残りの範囲。
- 配列から引数の slice 型へはビューを作り、書き込み可能から読み取り専用への変換を許す。
- `[?]T` は長さ推論配列、`[N?]T` は固定容量 vector の仮表記。
- 文字列・文字コード列の長さはバイト数。UTF-8 の文字数、画面上の幅、NES のタイル数とは別。
- slice のコピー・切り出しでは確保しない。標準の文字列操作も明示された宛先へ書く。
- scalar / slice / struct のみで戻り値を表し、ジェネリクス・タプル・例外・optional 型は要求しない。

| 層 | 担当すること | 例 |
|---|---|---|
| 言語・組み込み | 型ごとの配列 / vector 操作、slice の生成、要素コピー | `@copy`、`@try_append`、範囲式 |
| 標準ライブラリ | バイト列の比較、連結用バッファ、ASCII 数値化、終端 0 との接続 | `buffer`、`str` |
| ユーザーライブラリ | ゲームの文字コード、描画、転送キュー、数値 ID による ROM ファイルの読み出し | `game_text`、`ppu`、`fs`、`resource` |
| 利用者のコード | 使用する領域・寿命、失敗時の表示、バンクや割り込みとの調整 | イベント処理、メニュー処理 |

## 2. 標準ライブラリ: 書き込みバッファ

`buffer.ByteBuffer` は外部の領域を借り、使用長を持つ普通の struct。
容量付き slice の基本型や、自動的に領域を伸ばすコンテナは追加しない。

```fc
// buffer モジュールの API 案
public struct ByteBuffer {
    storage:[]uint8;
    used:uint16;
}

public struct WriteResult {
    ok:bool;
    data:[]const uint8;
}

public function init(storage:[]uint8):ByteBuffer;
public function clear(b:*ByteBuffer):void;
public function contents(b:*ByteBuffer):[]const uint8;
public function remaining(b:*ByteBuffer):[]uint8;
public function try_append(b:*ByteBuffer, src:[]const uint8):bool;
public function try_push(b:*ByteBuffer, value:uint8):bool;
public function try_advance(b:*ByteBuffer, written:uint16):bool;
```

| 操作 | 契約案 |
|---|---|
| `init` | `used = 0`。領域の確保も全体のゼロクリアもしない |
| `clear` | `used = 0` に戻す。過去のデータを消去する保証はない |
| `contents` | `storage[..used]` の読み取り専用ビュー。使用長は取得時点のもの |
| `remaining` | `storage[used..]` の書き込み可能ビュー。ファイル読み込みや直接生成の宛先に使う |
| `try_append` / `try_push` | 容量を先に検査。成功時だけデータと `used` を更新。失敗時は変更しない。終端 0 は付けない |
| `try_advance` | `remaining` へ直接書いたバイト数を `used` へ反映。残容量以下か検査し、失敗時は `used` を変えない |

`try_advance` はデータが初期化済みかまでは確認しない。実際に書いた数を渡すのは呼び出し側の責任。
各演算では、長さを先に加算してオーバーフローさせず、残容量との比較などで検査する。
`used <= storage.len` は常に必要。通常の struct なので、利用者がフィールドを直接変更した場合もこの条件を守る。

`try_append` は元と先が重なっても、指定した元のバイト列がコピーされる契約を候補とする。
必要なら memmove 相当で実装し、重なりがないと分かる場合は軽いコピーにする。
言語の値代入や汎用 `@copy` も同じ契約にするかは別途決める。

一連の複数の呼び出しはトランザクションではない。
例えば 1 回目の append が成功し 2 回目が失敗しても、1 回目の変更は残る。
失敗した組み立て結果を公開しないようにするか、全体の必要量を事前検査する。

## 3. 標準ライブラリ: 長さ付き文字列

まずは文字列専用型を増やさず、`[]const uint8` を受け取るバイト列処理として提供する。

```fc
// str モジュールの API 案
public function equal(a:[]const uint8, b:[]const uint8):bool;
public function starts_with(s:[]const uint8, prefix:[]const uint8):bool;
public function from_z(src:[]const uint8):[]const uint8;
public function to_z(dst:[]uint8, src:[]const uint8):void;
public function try_to_z(dst:[]uint8, src:[]const uint8):bool;
public function try_append_uint(b:*buffer.ByteBuffer, n:uint16):bool;
```

- `equal` / `starts_with` はバイト比較。文字コードの解釈やメモリ確保はしない。
- `from_z` は **src の範囲内だけ**を調べ、最初の 0 の直前までのビューを返す。
  0 がなければ全体を返す。文字列の妥当性を検証する関数ではない。
- `to_z` は `src` と終端 0 を宛先へ書く。余分な 1 バイトが必要で、容量不足なら停止。
  `try_to_z` は容量不足なら何も書かず false を返す版。
  埋め込み 0 はそのままコピーするため、旧 API では最初の 0 までしか読まれない。
  元と先の重なりは許し、元データをコピーした後で終端を書き込む契約を候補とする。
- `try_append_uint` は符号なし整数を ASCII の 10 進数字で追加する。0 は `"0"`。
  必要桁数を先に確認し、容量不足ならバッファを変更しない。実装は通常の関数でよい。

文字列リテラルや `textmap` 結果から slice へ変換したときに終端を含めるかはまだ未決。
本書の例では `from_z` を明示し、その決定に依存しないコードにする。
連結する各文字列の末尾に終端を含めないことが、途中の 0 による表示の打ち切りを防ぐ。

### 例: 確保せずにステータス文字列を組み立てる

```fc
// 関数とモジュール名は API 案
function format_hp(dst:[]uint8, hp:uint16):buffer.WriteResult
{
    var out = buffer.init(dst);
    if (!buffer.try_append(&out, str.from_z("HP: ")) ||
        !str.try_append_uint(&out, hp)) {
        return {ok: false, data: dst[..0]};
    }
    return {ok: true, data: buffer.contents(&out)};
}

// 呼び出し側の関数内
var storage:[32]uint8;
var result = format_hp(storage[..], hp);
if (result.ok) {
    // result.data は storage を参照する。ここで同期的に使うか、寿命を保って渡す
}
```

`format_hp` 自体は内部に大きな作業配列を持たず、呼び出し側の領域に直接書く。
失敗時の `data` は空だが、先行する処理で dst の一部が変更されている可能性はある。
`ok = true, data.len = 0` という正当な空の結果と、失敗を区別できる。

既存の `stdio.print(*int)` のような終端方式の API へ接続する場合は、
境界のラッパが `to_z` を使って互換バッファへ変換する案がある。
slice の `.ptr` をそのまま旧 API へ渡してよいとはしない。
将来 `[:0]const uint8` のような終端保証を採用する場合は、その保証を持つビューから
コピーせず旧 API に渡す方法も候補になる。通常の部分 slice は終端保証を持たない。
`finish_z` などによる連結後の接続例は [sentinel の調査](v3_slice_tradeoffs.md#3-sentinel-終端の調査) を参照。
新しい長さ付き出力 API を作る場合も、emu / NES の既存実装が長さを受け取れるか確認して実装する。

### 例: 所有する固定容量 vector と、文字列ビューの一覧

```fc
// vector の仮表記。終端 0 のない ASCII バイト列から初期化
var line:[32?]uint8 = [72, 80, 58];  // "HP:"
if (!@try_append(&line, str.from_z(" 100"))) {
    // 容量不足。直前の line は維持される
}
var text:[]const uint8 = line[..];   // 現在の長さだけを見る

var labels:[4?][]const uint8 = [
    str.from_z("Start"),
    str.from_z("Continue"),
];
```

`line` は文字の領域を所有する。一方 `labels` は slice の記述子を所有するだけで、
元の文字列はコピーしない。ROM のマッピングも含め、文字列の有効性は使用時まで保つ。
vector の slice を ByteBuffer へ渡しても、vector の長さフィールドと `used` は自動同期しない。
vector を伸ばす用途は `@try_append`、外部領域を使う用途は ByteBuffer と使い分ける。

## 4. ユーザーライブラリ: 数値 ID の readonly ファイルシステム

対象は ROM に組み込んだデータの読み出し。実行時のファイル名・パス文字列による検索や、
OS 的な open / close、書き込み API は導入しない。
`resource` モジュールに生成された数値 ID を使う。
現状の `fs_config.fc` は 255 を超える ID を含むため、初版は `uint16` でよい。enum は前提にしない。

```fc
// fs モジュールの API 案
public struct FileInfo {
    ok:bool;
    size:uint16;
}

public struct ReadResult {
    ok:bool;
    data:[]const uint8;
}

public function info(id:uint16):FileInfo;
public function read_into(id:uint16, dst:[]uint8):ReadResult;
public function read_at(id:uint16, offset:uint16, dst:[]uint8):ReadResult;
```

| 操作 | 契約案 |
|---|---|
| `info` | 数値 ID を検査し、ファイル全体のバイト数を返す。無効な ID は `ok = false` |
| `read_into` | ファイル全体を dst の先頭へコピー。ID や容量をコピー前に検査し、失敗なら dst を変更しない |
| `read_at` | offset から最大 dst.len バイトを読む。ファイル終端までで短くなることを許す。offset が終端と等しければ成功かつ空、終端より先なら失敗 |

成功時の `data` は **コピー先の RAM のうち、実際に書いた範囲**を参照する。
余った dst の末尾は変更しない。失敗時の `data` は `dst[..0]` という空のビューを返す案。
不正な ID / offset のときは dst を変更せず、全経路で元のバンク状態を復帰させる。
`info` が配置表を見るために切り替えたバンクも同様に戻す。

fs はバイト列をそのまま扱い、終端 0 の追加・除去、文字コード変換、圧縮の展開はしない。
readonly は ROM の元データを書き換える API がないという意味で、コピー先の RAM を不変にする保証ではない。
返却値を読み取り専用ビューにしても、呼び出し側の dst から書き換えることはできる。

### 例: ファイル全体を読む

```fc
var storage:[64]uint8;
var result = fs.read_into(resource.AREA_NAME_BASE + (area as uint16), storage[..]);
if (!result.ok) {
    return false;
}
var name = str.from_z(result.data); // この ID 群が終端方式の文字列であることはゲーム側が知っている
```

`resource.AREA_NAME_BASE` はビルド時に生成する定数。文字列ファイル名による検索は行わない。
ゲーム側で area の有効範囲も検査する。FS 全体に存在する ID でも、別カテゴリの ID を指す間違いはあり得る。

### 例: 大きいファイルの一部分を読む

```fc
var chunk:[256]uint8;
var result = fs.read_at(resource.MINIMAP_BG, 256, chunk[..]);
if (!result.ok) {
    return false;
}
// result.data.len は 0〜256。後段へ渡す前に空かどうかも分かる
```

この API は 8 ビットの入出力サイズへ暗黙に切り詰めない。
現状の `tools/banked_buffer.rb` はデータを 8KB バンクへ配置し、跨ぎを避けるためのパディングを入れている。
初版はファイルが 1 バンク内に収まることを生成側で検査する案が単純。
将来バンクをまたぐファイルを許すなら `read_at` 内部で分割するか、生成時に拒否するかを明確にする。
これはバンク配置のライブラリ規約であり、slice 自体の仕様にはしない。

### ROM を直接参照する API は別途必要になったときに

`fs.view(id)` で ROM の slice を返してからバンクを元へ戻す形は、通常の read API にしない。
返却時には別のデータが同じ CPU アドレスに見える可能性がある。
コピーを省く必要が実測で出た場合に、明示的な map / unmap などの上級 API を別途検討する。

バンクの一時切替と復帰は fs の通常の関数で実装し、言語に汎用のバンク寿命管理を追加しない。
割り込み側も同じマッパを操作する場合の同期・復帰規約はゲーム側で整える必要がある。

## 5. ユーザーライブラリ: ゲームの文字描画と PPU 転送

```fc
// game_text モジュール: 符号化済み文字列を RAM のタイル列へ同期的に描画する案
public function render_into(dst:[]uint8, codes:[]const uint8,
    width:uint8, height:uint8):buffer.WriteResult;

// ppu モジュール: 既存のリングバッファと転送キューを包む案
public function alloc_slice(size:uint16):[]uint8;
public function put_slice(addr:uint16, src:[]const uint8, flags:uint8):bool;
```

- `game_text.render_into` は呼び出し中に codes を読み、dst に書いた範囲を返す。内部で確保しない。
  表示領域不足や不正な制御コードを失敗にするかは実装時に詰める。失敗時に dst の途中まで書かれている可能性は許し、結果を転送しない。
- castle の描画を包む場合、表示 1 行につきタイル 2 行を使うなどの規則は game_text 側の担当。
  `width * height * 2` の必要サイズはオーバーフローしない幅で計算する。
- `str.try_append_uint` は ASCII 用。castle の文字コードへ数字を追加する関数は game_text 側に置く。
  `textmap` の文字コード列に ASCII の数字を直接混ぜない。
- `ppu.alloc_slice` は要求長のビューを返し、既存リングの周回方式を維持する案。
  無効なサイズは停止などで明示的に拒否する。失敗を返したい場合は別途結果 struct の API を設ける。
  サイズ 0 はカーソルを進めない空のビューとする案。256 バイト要求を許すかは既存のリング規約と合わせて決める。
- `put_slice` はポインタをキューへ保存するため、参照先は **転送の完了まで**有効でなければならない。
  空は何もせず成功。長さや flags の組み合わせが非対応ならキューを変更せず false。
  256 の特殊表現への変換は対応する転送経路だけで行い、それ以上の分割転送を提供するかは別途決める。
- `[]const uint8` は他の参照やリングの周回から内容を守らない。転送完了前の上書きやバンク切替は引き続き利用者が管理する。

## 6. 例: castle のアイテム取得メッセージを組み立てて描画する

現状の `event.run_get_item` の「文字列 → ファイル内のアイテム名 → 文字列」を、
外部領域へ直接書く例。数値 ID でファイルを参照し、文字列ファイル名は使わない。
`_T` はゲームの文字コードへ変換する既存の textmap を想定する。

```fc
function make_item_message(dst:[]uint8, item:uint8):buffer.WriteResult
{
    var out = buffer.init(dst);
    if (!buffer.try_append(&out, str.from_z(_T("「")))) {
        return {ok: false, data: dst[..0]};
    }

    // item はゲーム側で検査済みとする。ファイル内容を残りの領域へ直接読む
    var id = resource.ITEM_NAME_BASE + (item as uint16);
    var loaded = fs.read_into(id, buffer.remaining(&out));
    if (!loaded.ok) {
        return {ok: false, data: dst[..0]};
    }

    // ファイルは生バイト列として読み、文字列としての終端処理はここで行う
    var name = str.from_z(loaded.data);
    if (!buffer.try_advance(&out, name.len) ||
        !buffer.try_append(&out, str.from_z(_T("」を手に入れた")))) {
        return {ok: false, data: dst[..0]};
    }
    return {ok: true, data: buffer.contents(&out)};
}

function show_item_message(addr:uint16, item:uint8):bool
{
    var message_store = ppu.alloc_slice(32);
    var message = make_item_message(message_store, item);
    if (!message.ok) {
        return false;
    }

    var tile_store = ppu.alloc_slice(64);
    var tiles = game_text.render_into(tile_store, message.data, 32, 1);
    if (!tiles.ok) {
        return false;
    }
    return ppu.put_slice(addr, tiles.data, 0);
}
```

アイテム名は ROM から最終的な文字列の領域へ直接コピーされる。中間の所有 vector や文字列の再コピーは不要。
終端 0 を読んだ場合は使用長に含めず、後続文字列がその位置を上書きする。
読み出し時点ではファイルの終端 0 も含む物理サイズが残容量に収まる必要がある。

この例の戻り値は「組み立て・描画・転送予約ができたか」。画面への転送完了を意味しない。
呼び出し側は他の予約済みデータも含め、リングの周回が未消費の領域を上書きしないようにする。
同期描画が済んだ後は元の message は不要だが、tiles は割り込み側が消費するまで必要。
文字列リテラルや `_T` のデータがある ROM バンクも、各同期コピーが読む間はマップされている必要がある。

関数は失敗時に一部書かれた文字列やタイル列を公開しないが、リングのカーソルを巻き戻す保証はしない。
失敗時の表示省略・代替表示・停止はゲーム側で決める。

## 7. 例: イベントをまたいで保持する文字列

既存の `common.heap_strcpy` と `event2` の解放に相当する用途は、
汎用 allocator を言語へ導入する前に、ゲーム固有の通常の関数で表せる。

```fc
// heap モジュールの API 案
public function clone(src:[]const uint8):[]uint8;
public function release(block:[]uint8):bool;
```

- `clone` は明示的に一度確保してコピーする。返す長さはデータの長さで、追加用の容量は公開しない。
- 既存 heap に合わせて不足時に停止する版から始められる。失敗を返す版が必要なら `try_clone` と結果 struct を追加する。
- `release` には clone が返した領域全体をそのまま渡す。部分 slice や ROM のビューを解放しない。
  型だけで所有権・二重解放を検査する仕組みは導入しない。
- 空の clone は確保しない空の結果、空の release は何もせず成功、とする規約を候補とする。
- 長さ付き API だけで使うなら終端 0 は不要。旧描画 API へ渡す場合は、互換ラッパか終端付きコピーが別途必要。

利用者はイベント開始前に clone し、イベント状態に元の領域を保持する。
読み取り側へは `[]const uint8` のビューを渡し、イベント終了時に元の領域を release する。
この操作では容量を増やさないため、容量付き slice や自動再確保は要らない。

## 8. 設計を具体化するときの確認点

- 生ファイルのサイズと文字列の長さを分ける。生成側には textmap 変換結果と明示的な 0 追加の両方があり、
  ファイル種別ごとの終端規約を確認して移行する。fs 層で推測して取り除かない。
- 終端なし・埋め込み 0・空・容量ちょうど・容量不足、複数 append の途中失敗を API 単位で確認する。
- 無効な ID / offset、空ファイル、末尾の短い読み出し、バンク復帰、256 バイト以上の扱いを確認する。
- ByteBuffer の直接書き込みと commit、vector の独立した使用長、slice の浅いコピーを混同しない。
- PPU キューにローカルフレームを指す slice を保存して関数から戻る使い方を避ける。
  描画用リングの領域でも、後続の確保・再利用で破壊されないことが必要。
- 実装段階で、既存の `mem.strcpy`＋ポインタ更新とのコードサイズ・サイクル数・RAM 使用量を比較する。
  本書の例は API の利用イメージであり、コンパイル・実行・性能検証済みのコードではない。
