# bench — fc が生成するコードのベンチマーク

最適化の効果と退行を、決定的な数字（エミュレータのサイクル数とコードサイズ）で追うためのプログラム群。

```bash
go test ./bench            # results.json と比べる（出力・サイクル数・サイズ）
go test ./bench -v         # 一覧表を表示
go test ./bench -update    # 現在の値で results.json を書き換える（意図した変化のとき）
```

## 仕組み

- 各 `*.fc` は emu ターゲットで動く独立したプログラム。処理本体を `bench_start()` / `bench_end()`（`fclib/emu/stdio.fc`）で
  囲み、最後にチェックサムを `printf` して `exit(0)` する
- サイクル数はエミュレータ（`internal/r6502`）が数える。ページクロスと分岐成立のペナルティ込みで、同じバイナリなら必ず同じ値
- サイズは ld65 の map から、そのモジュールのセグメント（コード + ROM 上の定数）のバイト数
- `results.json` が基準値。出力が変わったらコンパイラのバグ（テスト失敗）、サイクル数・サイズが変わったら
  最適化の効果か退行（`-update` で受け入れる。golden と同じ運用）
- 各プログラムの答えは Python などで同じ計算を再現して照合してある（新しいベンチを足すときも同じようにする）

## プログラム

### A. 移植ベンチ（他のコンパイラの公開結果と比べられる）

[c-bench-64](https://github.com/thred/c-bench-64) / [millfork-benchmarks](https://github.com/KarolS/millfork-benchmarks) の
アルゴリズムを fc に移した。規模（配列の大きさ・繰り返し回数）は emu の RAM に合わせて変えてあるので、他社の数字とは
「1 要素あたり」の比較になる。

| 名前 | 内容 | 何を測るか |
|---|---|---|
| `sieve` | エラトステネスの篩、2048 フラグ × 10 回 | ポインタを進めるループ、16 ビットの比較・加算 |
| `crc8` | CRC-8（テーブル無し）、256 バイト × 16 回 | 1 バイトのシフト・XOR・分岐 |
| `crc16` | CRC-16 CCITT、256 バイト × 8 回 | 16 ビットのシフト・XOR・比較 |
| `plasma` | cc65 samples/plasma.c 由来、32x30 × 50 フレーム | sin テーブル引き + バイト加算 + 行ポインタ更新 |
| `linkedlist` | 200 ノードの単方向リストの走査 × 20 回 | struct のポインタ経由フィールド参照 |
| `fib` | 再帰 fib(18)、約 8,000 呼び出し | 関数呼び出し・フレーム確保・戻り値 |

### B. ゲーム形ベンチ（castle の処理を模したもの）

castle（`examples/castle`）のホットパスと同じ形の処理。数字の意味はこちらの方が重い。

| 名前 | 相当する castle のコード | 内容 |
|---|---|---|
| `entities` | `en.process` | SoA の 32 体を毎フレーム: 状態機械 → 移動 → タイルマップ衝突で反射 → アニメ。100 フレーム |
| `bgdecode` | `bg.fetch_area` / `scroll` | RLE の列を展開 → 属性テーブル → PPU 転送バッファ。64 列 × 4 周 |
| `oam` | `my.draw` / en の描画 | 2x2 メタスプライト（反転あり）24 個から OAM 256 バイトを構築。100 フレーム |
| `textprint` | `text.print` / `print_num` | 文字表で変換しながら幅で折り返し、16 ビット値の 10 進化。200 回 |
| `calls` | en_vtbl の `PROCESS[t](i)` など | 通常呼び出し / fastcall / 関数ポインタ表 / 4 段の入れ子、各 500 回 |
| `math16` | `my.fc` / `boxelev.fc` の物理 | 8.8 固定小数の位置・速度・重力・跳ね返り、距離、定数の乗除。64 個 × 100 フレーム |

## ベンチを足すとき

1. `bench/<name>.fc` を書く（`main` の中で `bench_start()` … `bench_end()`、チェックサムを `printf`、`exit(0)`）
2. 別の言語で同じ計算をして答えを照合する
3. `go test ./bench -update` で `results.json` に登録し、この表に 1 行足す

## まだ無いもの

- **マクロベンチ**: `examples/castle` を `internal/nes` で N フレーム走らせて 1 フレームあたりの CPU サイクルを出す
  （`TestPlayCastle` の拡張。ゲーム全体の KPI）
- 他コンパイラとの直接比較（同じソースを cc65 / Oscar64 / llvm-mos に通して同じエミュレータで測る）
