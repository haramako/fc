# fuzz の通知（Slack の #自動通知）の確認と対応の記録

Nomad の試し（yuri の `~/nomad-lab`。仕組みは ../discussions/2026-10-22-nomad-lab.md）が fc の fuzz を回し、失敗すると Slack の
#自動通知（C0CBDEBTMHT、Nomad Notify）に投稿する。投稿は 1 回の実行ごと: テスト名・種の始まり・`fc@コミット`・`id=`（実行の id）と、
出力の場所 `~/nomad-lab/results/artifacts/fc-fuzz/<id>/output.txt`（yuri）。

## 確かめ方

1. #自動通知 を読み、✅ の付いていない通知を古い順に見る（下の「対応した通知」の一番上より新しいもの）
2. 出力の `--- FAIL` の種と `_test.go:` の行で失敗の中身を見る（`fc 2 のプログラムがビルドできない` などの Skip の行は失敗でない）
3. 同じ種を手元で `go test ./internal/driver -run 'Test名/seed種$' -randn … -randseed 種の始まり` で再現する（Test 名ごとに本数の
   決め方が違う: TestRandomMigrate は randn / 10）。再現したら、前のコミットで走らせて退行か前からかを決める（`git archive <commit>`
   で別のディレクトリに取り出す）
4. 直したら fuzz の発見の記録（../discussions/2026-09-19-fuzz-findings-log.md）と回帰テストを足す。直す必要の無いもの（直した後の
   コミットより前で走った再発・試しの仕組みの失敗など）は、下の表に理由を書く
5. Slack の通知に ✅（white_check_mark）を付け、下の表の一番上に足す

## 対応した通知（新しい順）

最後に確かめた: 2026-10-26 13:35 JST（10/25 12:25 の確認の後の実行は 606 回で、失敗は下の表の上の 2 回だけ。
`~/nomad-lab/results/results.jsonl` の `result.fail` で数えた）

| 通知の時刻 (JST) | id | テスト | fc | 扱い |
|---|---|---|---|---|
| 2026-10-26 13:01 | 20261026T035627-e3d0 | TestRandomMigrate | 45ffae7 | **直した**（58910b6）。種 20261209508175: 下の行と同じ |
| 2026-10-26 07:30 | 20261025T222601-f84f | TestRandomMigrate | 45ffae7 | **直した**（58910b6）。種 20261207236303: `u16 * (-40807)` の migrate が定数にだけ `as i16` を付け、fc 4 で符号の混ざった式になって `/` がエラー（502db9d から）。TestMigrateBeyond16Folded |
| 2026-10-25 11:50 | 20261025T024644-3aa7 | TestRandomFloatV4 | ea27721 | **直した**（f0aa4c8）。種 20261198352618: 両辺が単項の `-` の浮動小数の `>=` が NaN で真（型を決める段が `-x` の型を返さず `!(b < a)` に。93bae0c から）。testdata/floating_args.fc |
| 2026-10-24 20:55 | 20261024T115237-7eb0 | TestRandomMigrate | 521d94c | **直した**（8c086ff）。種 20261191179568: 両方が定数の算術で migrate が `as` を足さない（502db9d からの退行）。TestMigrateBeyond16Folded |
| 2026-10-24 05:00 | 20261023T195751-cb71 | TestLogZeroCost | a83d2ca | **直してあった**。種 20261079698865 は 8c32dab（@log の消えた常駐）で直したもの。a83d2ca はその前 |
| 2026-10-23 22:45 | 20261023T134535-4d59 | TestRandomPrograms | 00000000 | **fc の不具合でない**。試しの準備の失敗（コミットの指定が 0 のまま clone した: `fatal: invalid reference`）。その後の実行は実際のコミットで走っている |
