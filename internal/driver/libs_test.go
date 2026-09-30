package driver

// fc.toml の [lib.*] のライブラリ (Agent/wiki/plans/v4-stdlib.md §9、internal/project/libs.go)。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haramako/fc/internal/project"
)

// writeTree は root の下に files (相対パス → 中身) を書く。
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o666); err != nil {
			t.Fatal(err)
		}
	}
}

// runProject は root/proj/t.fc を emu でビルドして走らせた出力。
func runProject(t *testing.T, root string, opt BuildOptions) (string, error) {
	t.Helper()
	var out strings.Builder
	dir := filepath.Join(root, "proj")
	opt.Target, opt.Dir, opt.BuildDir, opt.Out, opt.Run, opt.Stdout = "emu", dir, filepath.Join(dir, "b"), filepath.Join(dir, "t.bin"), true, &out
	_, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "t.fc", &opt)
	return out.String(), err
}

// TestLibPath: path のライブラリのモジュール・<lib>/<target> のモジュール・ライブラリの中の asm を使え、ライブラリは fclib の
// モジュール (rand) を置き換える。2 つのライブラリに同じ名前のモジュールがあればエラー。
func TestLibPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"proj/fc.toml": "[lib.util]\npath = \"../util\"   # コメント\n",
		"proj/t.fc": `#fc 4
use console;
use greet;
use tgt;
use rand;
function main():void
{
	@printf("{} {} {}\n", greet.n(), tgt.m(), rand.next_u8());
	console.exit(0);
}
`,
		"util/greet.fc":   "#fc 4\n@include(\"greet.asm\");\npublic function n():u8 @(abi: \"frame\");\n",
		"util/greet.asm":  "_greet_n:\n\tlda #42\n\tsta F_greet_n\n\trts\n",
		"util/emu/tgt.fc": "#fc 4\npublic function m():u8 { return 7; }\n",
		"util/rand.fc":    "#fc 4\npublic function next_u8():u8 { return 99; }   // fclib の rand を置き換える\n",
	})
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "42 7 99\n" {
		t.Fatalf("got %q, %v", out, err)
	}
	// fcc build -d の要約: ライブラリの場所と、使ったモジュール (fclib を置き換えたものに印)
	dir := filepath.Join(root, "proj")
	res, err := NewCompiler(absRepoRoot).BuildContext(t.Context(), "t.fc", &BuildOptions{Target: "emu", Dir: dir, BuildDir: filepath.Join(dir, "b"), Out: filepath.Join(dir, "t.bin")})
	if err != nil {
		t.Fatal(err)
	}
	if s := strings.Join(res.Libs, "\n"); !strings.Contains(s, "util: ") || !strings.Contains(s, "greet") || !strings.Contains(s, "tgt") ||
		!strings.Contains(s, "rand (replaces fclib)") || strings.Contains(s, "greet (replaces") {
		t.Errorf("要約:\n%s", s)
	}
	writeTree(t, root, map[string]string{
		"proj/fc.toml": "[lib.util]\npath = \"../util\"\n\n[lib.other]\npath = \"../other\"\n",
		"other/greet.fc": "#fc 4\npublic function n():u8 { return 1; }\n",
	})
	if _, err := runProject(t, root, BuildOptions{}); err == nil || !strings.Contains(err.Error(), "module greet is in both library util and library other") {
		t.Errorf("同じ名前のモジュール: %v", err)
	}
	writeTree(t, root, map[string]string{"proj/fc.toml": "[lib.util]\npath = \"../util\"\ngit = \"x\"\n"})
	if _, err := runProject(t, root, BuildOptions{}); err == nil || !strings.Contains(err.Error(), "either path or git") {
		t.Errorf("path と git: %v", err)
	}
}

// TestLibGit: git のライブラリを取ってきて fc.lock にコミットを書き、リポジトリが進んでも fc.lock のコミットを使う。update で
// 進め、rev のタグも使える。--offline はキャッシュに無ければエラー。
func TestLibGit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git が無い")
	}
	root := t.TempDir()
	t.Setenv("FC_LIB_CACHE", filepath.Join(root, "cache"))
	repo := filepath.Join(root, "repo")
	gitRun := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	version := func(v string) string {
		writeTree(t, repo, map[string]string{"src/ver.fc": "#fc 4\npublic function get():u8 { return " + v + "; }\n"})
		gitRun("add", "-A")
		gitRun("commit", "-q", "-m", "v"+v)
		return gitRun("rev-parse", "HEAD")
	}
	if err := os.MkdirAll(repo, 0o777); err != nil {
		t.Fatal(err)
	}
	gitRun("init", "-q", "-b", "main")
	c1 := version("1")
	gitRun("tag", "v1")
	writeTree(t, root, map[string]string{
		"proj/fc.toml": "[lib.ver]\ngit = \"" + filepath.ToSlash(repo) + "\"\ndir = \"src\"\n",
		"proj/t.fc":    "#fc 4\nuse console;\nuse ver;\nfunction main():void { @printf(\"{}\\n\", ver.get()); console.exit(0); }\n",
	})
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "1\n" {
		t.Fatalf("取ってくる: %q, %v", out, err)
	}
	cfg, err := project.FindConfig(filepath.Join(root, "proj"))
	if err != nil {
		t.Fatal(err)
	}
	lock, err := cfg.ReadLock()
	if err != nil || lock["ver"].Commit != c1 {
		t.Fatalf("fc.lock: %v, %v", lock, err)
	}
	// リポジトリが進んでも fc.lock のコミット
	c2 := version("2")
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "1\n" {
		t.Errorf("fc.lock のコミット: %q, %v", out, err)
	}
	// update で進める
	if _, err := (&project.Resolver{Update: []string{"ver"}}).Resolve(cfg); err != nil {
		t.Fatal(err)
	}
	if lock, _ := cfg.ReadLock(); lock["ver"].Commit != c2 {
		t.Errorf("update の後の fc.lock: %v", lock)
	}
	if out, err := runProject(t, root, BuildOptions{Offline: true}); err != nil || out != "2\n" {
		t.Errorf("update の後: %q, %v", out, err)
	}
	// rev のタグ (fc.toml の rev が変われば取り直す)
	writeTree(t, root, map[string]string{"proj/fc.toml": "[lib.ver]\ngit = \"" + filepath.ToSlash(repo) + "\"\nrev = \"v1\"\ndir = \"src\"\n"})
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "1\n" {
		t.Errorf("rev のタグ: %q, %v", out, err)
	}
	// --offline: キャッシュに無ければエラー
	t.Setenv("FC_LIB_CACHE", filepath.Join(root, "empty"))
	if _, err := runProject(t, root, BuildOptions{Offline: true}); err == nil || !strings.Contains(err.Error(), "not in the cache") {
		t.Errorf("--offline: %v", err)
	}
}

// TestLibDeps: ライブラリの一番上の fc.toml の [lib.*] も辿る (ライブラリが別のライブラリを使う)。依存どうしで同じ名前の
// 場所が違えばエラーで、プロジェクトの fc.toml に書けばそれが勝つ。
func TestLibDeps(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"proj/fc.toml": "[lib.a]\npath = \"../a\"\n",
		"proj/t.fc": `#fc 4
use console;
use ua;
function main():void
{
	@printf("{}\n", ua.f());
	console.exit(0);
}
`,
		"a/fc.toml": "[lib.b]\npath = \"../b\"\n",
		"a/ua.fc":   "#fc 4\nuse ub;\npublic function f():u8 { return ub.g() + 1; }\n",
		"b/ub.fc":   "#fc 4\npublic function g():u8 { return 10; }\n",
		"b2/ub.fc":  "#fc 4\npublic function g():u8 { return 20; }\n",
		"c/fc.toml": "[lib.b]\npath = \"../b2\"\n",
		"c/uc.fc":   "#fc 4\npublic function h():u8 { return 0; }\n",
	})
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "11\n" {
		t.Fatalf("依存を辿る: %q %v", out, err)
	}
	writeTree(t, root, map[string]string{"proj/fc.toml": "[lib.a]\npath = \"../a\"\n[lib.c]\npath = \"../c\"\n"})
	if _, err := runProject(t, root, BuildOptions{}); err == nil || !strings.Contains(err.Error(), "library b:") || !strings.Contains(err.Error(), "point to different places") {
		t.Errorf("依存どうしの食い違い: %v", err)
	}
	writeTree(t, root, map[string]string{"proj/fc.toml": "[lib.a]\npath = \"../a\"\n[lib.c]\npath = \"../c\"\n[lib.b]\npath = \"../b2\"\n"})
	if out, err := runProject(t, root, BuildOptions{}); err != nil || out != "21\n" {
		t.Errorf("プロジェクトの fc.toml が勝つ: %q %v", out, err)
	}
}
