#!/bin/bash
# origin/main の最新で差分 fuzz (internal/driver の TestRandom*) を回し続ける。失敗したら止まる (終了コード 1)。
# 使い方は同じフォルダの README.md。
set -u

WORK=${FC_FUZZ_DIR:-$HOME/.cache/fc-fuzz}
REPO=${FC_FUZZ_REPO:-https://github.com/haramako/fc.git}
RANDN=${FC_FUZZ_RANDN:-4000}
GO=${FC_FUZZ_GO:-go}
# 1 回がこれより短く終わったら、何も回っていないとみなして止める (空回りで summary.log を埋めない)
MIN_SECONDS=${FC_FUZZ_MIN_SECONDS:-60}

case "$RANDN" in
'' | *[!0-9]*)
	echo "FC_FUZZ_RANDN must be a number: $RANDN" >&2
	exit 2
	;;
esac

mkdir -p "$WORK/logs"
if [ ! -d "$WORK/src/.git" ]; then
	git clone -q "$REPO" "$WORK/src" || exit 2
fi

while :; do
	# 取れなければ (ネットワークが無いなど) 待ってやり直す
	if ! git -C "$WORK/src" fetch -q origin || ! git -C "$WORK/src" checkout -q --detach origin/main; then
		sleep 600
		continue
	fi
	rev=$(git -C "$WORK/src" rev-parse --short HEAD)
	# 分まで入れた種。CI の夜間 (YYYYMMDD000 から) とも前の回とも重ならない
	seed=$(date +%Y%m%d%H%M)0000
	log="$WORK/logs/$seed-$rev.log"
	echo "$(date '+%F %T') start rev $rev seed $seed randn $RANDN" >>"$WORK/summary.log"
	started=$(date +%s)
	(cd "$WORK/src" && FUZZ_STATS="$WORK/logs/$seed-$rev.jsonl" "$GO" test ./internal/driver -run TestRandom \
		-randn "$RANDN" -foldn $((RANDN / 4)) -metan $((RANDN / 10)) -mutn $((RANDN * 3 / 20)) \
		-randseed "$seed" -timeout 300m) >"$log" 2>&1
	st=$?
	echo "$(date '+%F %T') end   rev $rev seed $seed exit $st" >>"$WORK/summary.log"
	if [ $st -ne 0 ]; then
		echo "fuzz failed: $log"
		exit 1
	fi
	if [ $(($(date +%s) - started)) -lt "$MIN_SECONDS" ]; then
		echo "fuzz finished too quickly (under $MIN_SECONDS s): $log"
		exit 3
	fi
	rm -f "$log" "$WORK/logs/$seed-$rev.jsonl"
done
