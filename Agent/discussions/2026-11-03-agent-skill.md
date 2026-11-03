# fc でゲームを作るエージェント向けのスキルと、fcc・サイトからの導線

## 背景

公式の文書を見るエージェントや fcc だけを入れたエージェントが、デバッグの道具（tools/nesrun）に気付けない。ユーザーから
「A 道具の手に入れ方・B fcc からの案内・C llms.txt」と、弱め（30B 級）の LLM 向けのスキルの依頼（2026-11-03）。スキルは
細かい情報を公式のサイトに任せてよい（fc の更新があるので。ユーザー）。

## 決めたこと

- スキルは `skills/fc-nes-game/SKILL.md` の 1 ファイル（Agent Skills の形。curl 1 行で入る）。中身は手順・まちがえやすい所・
  デバッグの表・不具合を報告する前の手順だけ。API は `fcc doc`（手元の版と一致）、仕様は公式のサイトを引かせる
- 入れ方は `docs/start/agents.md`（pi などは `.agents/skills/`、Claude Code は `.claude/skills/`、全部のプロジェクトならホーム）
- 道具（tools/nesrun）はスキルに写さず、raw の URL から取ってくる（写しが古くならないように）
- fcc から案内: `fcc --help` の説明に文書の URL、`-g` で `.fclog.lua` を書いたら読み方、`fcc run -t nes` の終わらなかった
  ときの文言に tools/nesrun
- `docs/public/llms.txt`（エージェント向けの目次）
- `TestSkills` がスキルと llms.txt の参照（見出しの言葉・ページ・道具のファイル）を見張る

## 確かめたこと

- pi 1.5.0 はプロジェクトの `.agents/skills/` を、プロジェクトを信頼したときだけ読む（`pi -p` は `-a` が要る）。読む道具（read）を
  外すとスキルの一覧が出ない（`--no-tools` で「No skills」）
- pi（qwen3.8-flash-next）に「16 進 2 桁で書く @format」を聞くと、指示しなくても SKILL.md を読み、`{:02X}` と答えた

## pi での試し（2026-11-03）

スキルだけを入れた空のプロジェクトで、pi（qwen3.8-flash-next）に「hello world・60 フレームごとの @log・画面の確認」をさせた
（fcc は今の版、`FC_MESEN` あり、tools/nesrun は渡さない）。

- 1 回目（約 5 分で完了）: 道具を curl で取り、fcmesen.py で @log とネームテーブル、fcmesen.py と fcretro.py の PNG で画面を確かめた。
  つまずき: `-o build/game.nes` の build/ が無いと ld65 が `Cannot open 'build/game.nes.part'`（→ fcc が作るように直した）、
  `fcc fmt` が書き換えない（-w）、`-g` が無いと @log が無音、fc.toml の最小の形が分からず examples を写した（→ スキルに足した）
- 2 回目（直した後）: fc.toml を作らず、出力先の問題も無く完了。つまずき: @log の引数に式を書けない（仕様。文言は分かりやすい
  → スキルと debug.md に書いた）、`--frames` と `frame.count` が起動の分ずれる（→ debug.md に書いた）
