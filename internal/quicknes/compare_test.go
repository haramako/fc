package quicknes

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// TestComparePlay: 2 つの ROM (FC_QN_OLD と FC_QN_NEW。コンパイラを変える前と後の castle など) に同じ入力を与えて、30 フレーム
// ごとの画面を比べる (違う枚数を出し、1 枚でも違えば失敗)。入力は internal/nes の TestCastleFrameCycles と同じ筋書き (タイトル →
// A で開始 → 右へ歩きながら跳ぶ)。環境変数が無ければ飛ばす:
//
//	FC_QN_OLD=old.nes FC_QN_NEW=new.nes go test ./internal/quicknes -run ComparePlay -v
func TestComparePlay(t *testing.T) {
	oldPath, newPath := os.Getenv("FC_QN_OLD"), os.Getenv("FC_QN_NEW")
	if oldPath == "" || newPath == "" {
		t.Skip("FC_QN_OLD / FC_QN_NEW が無い")
	}
	shots := func(path string) [][]byte {
		rom, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Open(rom)
		if errors.Is(err, ErrUnavailable) {
			t.Skip(err)
		} else if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		var r [][]byte
		run := func(n int) {
			for i := 0; i < n; i++ {
				m.RunFrames(1)
				if m.Frames()%30 == 0 {
					r = append(r, append([]byte(nil), m.Image().Pix...))
				}
			}
		}
		run(180)
		m.SetButtons(0, ButtonA)
		run(10)
		m.SetButtons(0, 0)
		run(300)
		for cycle := 0; cycle < 14; cycle++ {
			m.SetButtons(0, ButtonRight)
			run(60)
			m.SetButtons(0, ButtonRight|ButtonA)
			run(60)
			m.SetButtons(0, ButtonRight)
			run(60)
		}
		return r
	}
	a, b := shots(oldPath), shots(newPath)
	diff := 0
	for i := range min(len(a), len(b)) {
		if !bytes.Equal(a[i], b[i]) {
			if diff == 0 {
				t.Logf("最初の違い: %d フレーム目", (i+1)*30)
			}
			diff++
		}
	}
	distinct := map[string]bool{}
	for _, s := range b {
		distinct[string(s)] = true
	}
	t.Logf("%d 枚中 %d 枚が違う (新しい方の画面は %d 種類)", min(len(a), len(b)), diff, len(distinct))
	if diff != 0 || len(a) != len(b) {
		t.Fail()
	}
}
