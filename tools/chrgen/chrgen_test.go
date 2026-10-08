package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/haramako/fc/pkg/fc"
)

var update = flag.Bool("update", false, "games/common/tiles.{chr,fc,png} を tiles.txt から書き直す")

const commonTiles = "../../games/common/tiles.txt"

// TestCommonTiles: games/common の生成物が tiles.txt と合っている (go test ./tools/chrgen -update で書き直す)。
func TestCommonTiles(t *testing.T) {
	out, err := outputs(commonTiles)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range out {
		if *update {
			if err := os.WriteFile(path, want, 0o666); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s が tiles.txt と違う (go test ./tools/chrgen -update か go run ./tools/chrgen games/common/tiles.txt)", path)
		}
	}
}

// TestCommonTilesBuild: tiles.fc と tiles.chr を使うプログラムが NES の ROM にビルドできる (定数・メタスプライトの表)。
func TestCommonTilesBuild(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"tiles.fc", "tiles.chr"} {
		b, err := os.ReadFile(filepath.Join("../../games/common", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	main := `#fc 4
use nes;
use frame;
use oam;
use tiles;
function main():void
{
	frame.init();
	frame.ctrl |= nes.CTRL_SPR_1000;
	oam.begin();
	oam.meta(100, 100, tiles.HERO_R0_M8, 0);
	oam.spr(20, 20, tiles.BALL, 0);
	oam.end();
	frame.wait();
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.fc"), []byte(main), 0o666); err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs("../..")
	c := fc.NewWithHome(root)
	defer c.Close()
	if _, err := c.Build(context.Background(), "main.fc", fc.Options{Target: fc.TargetNES, Dir: dir, Out: filepath.Join(dir, "a.nes")}); err != nil {
		t.Fatal(err)
	}
}
