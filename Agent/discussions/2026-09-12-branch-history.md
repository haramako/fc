# ブランチ運用の経緯（〜2026-09-19）

`doc/development_notes.md` の「ブランチ運用」節をそのまま残したもの（feature/v2・static-frame 時代の記述で、現在の運用はルートの `AGENTS.md`）。

- 開発は **`feature/v2`**（2026-09-12〜。文法 v2 / struct・soa / far call / ベンチ / 最適化）で行う。
  **安定するまで master へはマージしない**（一度マージしたが取り消し済み。master = 8358b15 のまま）。
  `agent/golang` は Go 移植のブランチで、feature/v2 の親。最適化（第 3 弾〜）は **`feature/static-frame`**
  （feature/v2 から分岐）で進めている。**実プロジェクト castle への反映と feature/v2 へのマージは SSA
  （第 4 弾の本命）が終わってからまとめて行う**（2026-09-19 決定）。それまで examples/castle が castle 側の
  変更（data.asm の FC_SZP / FC_SRAM / FC_SP、mmc3.fc の options、ppu.fc のスプライト消去）の置き場
- タグ: `v0.0.2`（2026-09-15、最適化前のベースライン）、`ruby-frozen`（Go 移植前の Ruby 版。
  `fclib/math.fc` / `share/runtime.asm` の sin / atan / rand / 乗算テーブルは `misc/table.rb` の生成物でタグから参照できる）、
  `go-strict-clone`（移植直後の基準点）
- **push 注意**: origin は公開の github.com/haramako/fc。
  `examples/castle` は製品コード（ゲームテキスト・リソース含む）なので、
  **push する前に公開可否の判断が必要**
