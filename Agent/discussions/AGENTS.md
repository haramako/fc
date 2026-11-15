# Discussions

## Purpose

エージェントとユーザーの会話における**相談内容・決定事項・経緯**を歴史ログとして蓄積する。Wiki や各 `AGENTS.md`（＝「現在の正しい状態」を書く場所）には載らない、**「なぜそうなったか」という歴史的事項**を残す。

## Local Contracts

- 1トピックにつき1ファイル。ファイル名は `YYYY-MM-DD-<slug>.md`（`<slug>` は内容を表す kebab-case。例: `2026-06-21-dox-tree-bootstrap.md`）
- 日付はその相談・決定が行われた日
- 推奨内容: 背景／相談したこと／選択肢と検討／**決定事項**／理由
- **追記型・不変**: 過去ログは原則書き換えない。方針が変わったら新しい日付のファイルを追加する。例外は、後の決定で覆った記録の冒頭に
  `→ 後の決定: [題](YYYY-MM-DD-<slug>.md)` の 1 行を足すことだけ
- **ファイルはこのフォルダに平らに置く**（月や年のフォルダに分けない。古い記録へのリンクが切れないように。新旧はファイル名の日付で分かる）
- 決定がコードや運用の現在ルールに昇格する場合は、該当する `AGENTS.md`（または `../wiki/`）にも反映し、ここには経緯として残す
- **索引**: ファイルを追加したら、この `AGENTS.md` の **Log** の先頭に 1 行足す（新しい順）。Log は直近の分だけを置く: このファイルが
  大きさの目安（`internal/doccheck` の `TestAgentBudget` の表）を超えたら、古い行から `log-YYYY-MM.md`（その月の索引。新しい順）の
  先頭へ移す。どの記録も Log か月の索引のどちらかにちょうど 1 回載る（`TestAgentIndex` が見る）

## Log

- 2026-11-15 [今の fuzz と自動テストで検出できない誤りを検出する方法](2026-11-15-undetected-errors.md) — すり抜けの 6 つの型と方法 A〜F。A（実行のときの検査 internal/san）を作った、E（Skip されるテストを Nomad で）は Nomad のエージェントへ。フレームを重ねる対照で 8 → 114 本
- 2026-11-03 [MMC3 の画面分割を fclib の部品にする](2026-11-03-raster-split.md) — raster.split(i, 走査線, x, y, nt)。IRQ と時機は fclib の asm、frame の NMI がスクロールの後に仕掛ける。mmc3 と別のモジュール。Mesen と QuickNES の画素の比較で時機を決めた
- 2026-11-03 [エージェント向けのスキルと、fcc・サイトからの導線](2026-11-03-agent-skill.md) — skills/fc-nes-game（1 ファイル、細かい所は fcc doc と公式のサイト）、start/agents の入れ方、llms.txt、fcc のヘルプとメッセージから案内、TestSkills。pi は信頼したプロジェクトの .agents/skills だけ読む
- 2026-11-03 [VRAM キューの HUD の最後の 1 文字が出ない件の切り分け](2026-11-03-vram-queue-handoff.md) — fc-gradius の 520a009 でも作業ツリーでも再現せず、fclib の分ける経路は総当たり（対照つき）で正しい。後半の観察は描画中の read_row で無効。相手の qnes.py は上下左右 8 ドットを切り落とす
- 2026-11-03 [ROM を画面なしで動かす道具とデバッグのページ](2026-11-03-nesrun-tools.md) — tools/nesrun の fcmesen.py（ヘッドレス Mesen で @log・panic）と fcretro.py（QuickNES で画面・RAM）。「libretro で RAM が 0」は相手のスクリプトが id を渡していなかった
