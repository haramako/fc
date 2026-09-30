package fc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLibAdd: fcc lib add は fc.toml (無ければ作る) に [lib.NAME] を書き足して取ってくる。フォルダは fc.toml からの相対の
// path、git の URL は git / rev / dir。同じ名前・フォルダでも URL でもないものはエラーで、fc.toml は変わらない。
func TestLibAdd(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	util := filepath.Join(root, "util")
	for _, d := range []string{proj, util} {
		if err := os.MkdirAll(d, 0o777); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(util, "greet.fc"), []byte("#fc 4\npublic function n():u8 { return 42; }\n"), 0o666)
	os.WriteFile(filepath.Join(proj, "t.fc"), []byte("#fc 4\nuse console;\nuse greet;\nfunction main():void\n{\n\t@printf(\"{}\\n\", greet.n());\n\tconsole.exit(0);\n}\n"), 0o666)

	infos, err := LibAdd(proj, "util", util, "", "")
	if err != nil || len(infos) != 1 || infos[0].Name != "util" {
		t.Fatalf("add: %v %v", infos, err)
	}
	toml, _ := os.ReadFile(filepath.Join(proj, "fc.toml"))
	if string(toml) != "[lib.util]\npath = \"../util\"\n" {
		t.Errorf("fc.toml: %q", toml)
	}
	c, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	var out strings.Builder
	if _, err := c.Build(context.Background(), "t.fc", Options{Dir: proj, Out: filepath.Join(proj, "t.bin"), Run: true, Stdout: &out}); err != nil || out.String() != "42\n" {
		t.Fatalf("build: %q %v", out.String(), err)
	}
	for _, bad := range []struct{ name, src, msg string }{
		{"util", util, "[lib.util] is already there"},
		{"x y", util, "use letters, digits"},
		{"other", filepath.Join(root, "nope"), "is not a folder or a git URL"},
	} {
		if _, err := LibAdd(proj, bad.name, bad.src, "", ""); err == nil || !strings.Contains(err.Error(), bad.msg) {
			t.Errorf("%s %s: %v", bad.name, bad.src, err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(proj, "fc.toml")); string(got) != string(toml) {
		t.Errorf("エラーの後の fc.toml: %q", got)
	}

	// git (ローカルのリポジトリを file:// で): 取ってきて fc.lock を書く。取れなければ fc.toml を元に戻す
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git が無い")
	}
	t.Setenv("FC_LIB_CACHE", filepath.Join(root, "cache"))
	repo := filepath.Join(root, "repo")
	os.MkdirAll(filepath.Join(repo, "src"), 0o777)
	os.WriteFile(filepath.Join(repo, "src", "tool.fc"), []byte("#fc 4\npublic function k():u8 { return 7; }\n"), 0o666)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "a"}, {"tag", "v1"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	url := "file://" + filepath.ToSlash(repo)
	if !strings.HasPrefix(url, "file:///") {
		url = "file:///" + filepath.ToSlash(repo)
	}
	if infos, err = LibAdd(proj, "tool", url, "v1", "src"); err != nil || len(infos) != 2 || infos[1].Commit == "" {
		t.Fatalf("git add: %v %v", infos, err)
	}
	toml, _ = os.ReadFile(filepath.Join(proj, "fc.toml"))
	if !strings.Contains(string(toml), "\n\n[lib.tool]\ngit = \""+url+"\"\nrev = \"v1\"\ndir = \"src\"\n") {
		t.Errorf("fc.toml: %q", toml)
	}
	if _, err := os.Stat(filepath.Join(proj, "fc.lock")); err != nil {
		t.Errorf("fc.lock: %v", err)
	}
	if _, err := LibAdd(proj, "gone", url+"-missing.git", "", ""); err == nil {
		t.Errorf("無いリポジトリを足せた")
	}
	if got, _ := os.ReadFile(filepath.Join(proj, "fc.toml")); string(got) != string(toml) {
		t.Errorf("取れなかった後の fc.toml: %q", got)
	}
}
