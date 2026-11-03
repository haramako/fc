package driver

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOutputDirCreated: -o の先のディレクトリが無ければ作る (`-o build/game.nes` で build/ が無いと、ld65 が
// 「Cannot open 'build/game.nes.part'」で落ちていた。pi のエージェントの試しで見つかった)。
func TestOutputDirCreated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"t.fc": "#fc 4\nfunction main():void\n{\n}\n"})
	out := filepath.Join(dir, "build", "sub", "game.nes")
	if _, err := buildCode(t, NewCompiler(absRepoRoot), "t.fc", &BuildOptions{Target: "nes", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: out}, false, nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}
