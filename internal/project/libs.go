package project

// ライブラリの取り込み (シンプルなパッケージマネージャ。Agent/wiki/plans/v4-stdlib.md §9)。fc.toml の [lib.NAME] に、フォルダ (path) か git の
// リポジトリ (git / rev / dir) を書くと、そのモジュールを use できる。探索は「ソースのディレクトリ → fc.toml の順のライブラリ
// (それぞれ <lib> と <lib>/<target>) → fclib → fclib/<target>」で、ライブラリは fclib のモジュールを置き換えられる。
//
// git のライブラリは、ユーザーのキャッシュ (os.UserCacheDir()/fc/lib。FC_LIB_CACHE で変えられる) に、リポジトリごとの bare の写し
// (git/<キー>.git) と、コミットごとの取り出し (src/<キー>@<コミット>。中身は変わらないのでプロジェクトの間で共有する) を作る。
// rev (タグ・ブランチ・コミット。無ければ既定のブランチ) は取ってきたときのコミットを fc.lock に書き、以後のビルドはそのコミットを
// 使う (fcc lib update で進める)。ビルドはライブラリのコードを実行しない (コンパイラが読むのはソース・asm・データだけ)。

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/haramako/fc/internal/diag"
)

// LockName は git のライブラリのコミットを固定するファイル (fc.toml の隣)。
const LockName = "fc.lock"

// Lib は fc.toml の [lib.NAME] の 1 つ。
type Lib struct {
	Name string
	Path string // path: ライブラリのフォルダ (絶対パス)
	Git  string // git: リポジトリの URL (かローカルのパス)
	Rev  string // rev: タグ・ブランチ・コミット ("" なら既定のブランチ)
	Dir  string // dir: リポジトリの中のモジュールのある所
}

// Libs は fc.toml の [lib.*] を書いた順に返す。
func (cfg *ProjectConfig) Libs() ([]Lib, error) {
	var libs []Lib
	for _, s := range cfg.Order {
		name, ok := strings.CutPrefix(s, "lib.")
		if !ok {
			continue
		}
		kv := cfg.Sections[s]
		fail := func(msg string) error {
			return &diag.Error{Msg: fmt.Sprintf("%s: [lib.%s]: %s", cfg.Path, name, msg)}
		}
		if !validLibName(name) {
			return nil, fail("the library name must be letters, digits, _ or -")
		}
		for k := range kv {
			if k != "path" && k != "git" && k != "rev" && k != "dir" {
				return nil, fail(fmt.Sprintf("unknown key %s (path / git / rev / dir)", k))
			}
		}
		l := Lib{Name: name, Git: kv["git"], Rev: kv["rev"], Dir: kv["dir"]}
		switch p, hasPath := kv["path"]; {
		case hasPath && l.Git != "":
			return nil, fail("write either path or git, not both")
		case hasPath:
			if l.Rev != "" || l.Dir != "" {
				return nil, fail("rev and dir are for git libraries")
			}
			if !filepath.IsAbs(p) {
				p = filepath.Join(filepath.Dir(cfg.Path), p)
			}
			l.Path = filepath.Clean(p)
		case l.Git == "":
			return nil, fail("needs path = \"folder\" or git = \"url\"")
		}
		if strings.Contains(l.Dir, "..") || filepath.IsAbs(l.Dir) {
			return nil, fail("dir must be a path inside the repository")
		}
		libs = append(libs, l)
	}
	return libs, nil
}

var libNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func validLibName(s string) bool { return libNameRe.MatchString(s) }

// AddLib は fc.toml (dir から親へ探す。無ければ dir に作る) に [lib.name] を書き足し、元の中身を返す (取ってくるのに失敗したら
// 呼ぶ側が書き戻す)。src が URL (`://`・`git@`・`.git` で終わる) なら git、フォルダなら path (fc.toml からの相対)。
func AddLib(dir, name, src, rev, sub string) (path string, old []byte, existed bool, err error) {
	if !validLibName(name) {
		return "", nil, false, &diag.Error{Msg: fmt.Sprintf("library name %q: use letters, digits, _ or -", name)}
	}
	cfg, err := FindConfig(dir)
	if err != nil {
		return "", nil, false, err
	}
	path, existed = cfg.Path, cfg.Path != ""
	if !existed {
		path = filepath.Join(dir, ConfigName)
	} else if old, err = os.ReadFile(path); err != nil {
		return "", nil, false, err
	}
	if _, dup := cfg.Sections["lib."+name]; dup {
		return "", nil, false, &diag.Error{Msg: fmt.Sprintf("%s: [lib.%s] is already there", path, name)}
	}
	var b strings.Builder
	b.Write(old)
	if len(old) > 0 && !bytes.HasSuffix(old, []byte("\n")) {
		b.WriteString("\n")
	}
	if len(old) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "[lib.%s]\n", name)
	if strings.Contains(src, "://") || strings.HasPrefix(src, "git@") || strings.HasSuffix(src, ".git") {
		fmt.Fprintf(&b, "git = %q\n", src)
		if rev != "" {
			fmt.Fprintf(&b, "rev = %q\n", rev)
		}
		if sub != "" {
			fmt.Fprintf(&b, "dir = %q\n", sub)
		}
	} else {
		abs, err := filepath.Abs(src)
		if err != nil {
			return "", nil, false, err
		}
		if st, err := os.Stat(abs); err != nil || !st.IsDir() {
			return "", nil, false, &diag.Error{Msg: fmt.Sprintf("%s is not a folder or a git URL", src)}
		}
		if rev != "" || sub != "" {
			return "", nil, false, &diag.Error{Msg: "--rev and --dir are for git libraries"}
		}
		rel, err := filepath.Rel(filepath.Dir(path), abs)
		if err != nil {
			rel = abs
		}
		fmt.Fprintf(&b, "path = %q\n", filepath.ToSlash(rel))
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o666); err != nil {
		return "", nil, false, err
	}
	return path, old, existed, nil
}

// LockEntry は fc.lock の 1 つ (git のライブラリ)。
type LockEntry struct {
	Git, Rev, Commit string
}

// ReadLock は fc.toml の隣の fc.lock を読む (無ければ空)。
func (cfg *ProjectConfig) ReadLock() (map[string]LockEntry, error) {
	m := map[string]LockEntry{}
	if cfg.Path == "" {
		return m, nil
	}
	p := filepath.Join(filepath.Dir(cfg.Path), LockName)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return m, nil
	} else if err != nil {
		return nil, err
	}
	lc, err := parseConfig(p, data)
	if err != nil {
		return nil, err
	}
	for s, kv := range lc.Sections {
		if name, ok := strings.CutPrefix(s, "lib."); ok {
			m[name] = LockEntry{Git: kv["git"], Rev: kv["rev"], Commit: kv["commit"]}
		}
	}
	return m, nil
}

// WriteLock は fc.lock を書く (名前の順。中身が同じなら書かない)。
func (cfg *ProjectConfig) WriteLock(m map[string]LockEntry) error {
	var names []string
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	var b bytes.Buffer
	b.WriteString("# fc.lock: fcc が書く (手で直さない)。fc.toml の [lib.*] の git のライブラリを、取ってきたときのコミットに固定する\n")
	b.WriteString("# (fcc lib update [名前] で進める。Agent/wiki/plans/v4-stdlib.md §9)\n")
	for _, n := range names {
		e := m[n]
		fmt.Fprintf(&b, "\n[lib.%s]\ngit = %q\nrev = %q\ncommit = %q\n", n, e.Git, e.Rev, e.Commit)
	}
	p := filepath.Join(filepath.Dir(cfg.Path), LockName)
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, b.Bytes()) {
		return nil
	}
	return os.WriteFile(p, b.Bytes(), 0o666)
}

// Resolver はライブラリのフォルダを決める (git のものは取ってくる)。
type Resolver struct {
	Offline bool     // git から取ってこない (キャッシュに無ければエラー)
	Update  []string // fc.lock を無視して rev の今のコミットに進めるライブラリ (Update に "*" があれば全部)
	Log     func(format string, args ...any)
}

// ResolvedLib は決まったライブラリ。
type ResolvedLib struct {
	Lib
	Root   string // モジュールのあるフォルダ (Dir まで含む)
	Commit string // git のライブラリのコミット
}

// Resolve は cfg の [lib.*] を決め、git のものはキャッシュに揃えて fc.lock を書き直す。ライブラリの依存: ライブラリの一番上
// (path のフォルダ、git のリポジトリ) に fc.toml があれば、その [lib.*] も辿る (幅優先。探索の順もこの順)。同じ名前は、
// プロジェクトの fc.toml に書いたものが勝ち (依存の版を選べる)、依存どうしで場所が違えばエラー。fc.lock はプロジェクトの隣に
// 依存の git のライブラリも含めて書く。
func (r *Resolver) Resolve(cfg *ProjectConfig) ([]ResolvedLib, error) {
	libs, err := cfg.Libs()
	if err != nil || len(libs) == 0 {
		return nil, err
	}
	lock, err := cfg.ReadLock()
	if err != nil {
		return nil, err
	}
	type item struct {
		lib  Lib
		from string // 書いた fc.toml
		via  string // 依存のもとのライブラリ ("" はプロジェクトの fc.toml)
	}
	var queue []item
	for _, l := range libs {
		queue = append(queue, item{l, cfg.Path, ""})
	}
	seen := map[string]item{}
	newLock := map[string]LockEntry{}
	var res []ResolvedLib
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		l := it.lib
		if prev, ok := seen[l.Name]; ok {
			if prev.via == "" || sameLib(prev.lib, l) {
				continue // プロジェクトの fc.toml が勝つ / 同じもの
			}
			return nil, &diag.Error{Msg: fmt.Sprintf("library %s: %s (library %s) and %s (library %s) point to different places; choose one with [lib.%s] in %s",
				l.Name, prev.from, prev.via, it.from, it.via, l.Name, cfg.Path)}
		}
		seen[l.Name] = it
		top := l.Path // 依存の fc.toml を探す所 (ライブラリの一番上)
		if l.Git == "" {
			if st, err := os.Stat(l.Path); err != nil || !st.IsDir() {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: [lib.%s]: path %s is not a folder", it.from, l.Name, l.Path)}
			}
			res = append(res, ResolvedLib{Lib: l, Root: l.Path})
		} else {
			e, ok := lock[l.Name]
			commit := e.Commit
			if !ok || e.Git != l.Git || e.Rev != l.Rev || r.updating(l.Name) || commit == "" {
				commit = ""
			}
			dir, commit, err := r.fetch(l, commit)
			if err != nil {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: [lib.%s]: %v", it.from, l.Name, err)}
			}
			root := filepath.Join(dir, filepath.FromSlash(l.Dir))
			if st, err := os.Stat(root); err != nil || !st.IsDir() {
				return nil, &diag.Error{Msg: fmt.Sprintf("%s: [lib.%s]: dir %q is not in the repository", it.from, l.Name, l.Dir)}
			}
			newLock[l.Name] = LockEntry{Git: l.Git, Rev: l.Rev, Commit: commit}
			res = append(res, ResolvedLib{Lib: l, Root: root, Commit: commit})
			top = dir
		}
		dep := filepath.Join(top, ConfigName)
		data, err := os.ReadFile(dep)
		if err != nil {
			continue
		}
		dcfg, err := parseConfig(dep, data)
		if err != nil {
			return nil, err
		}
		deps, err := dcfg.Libs()
		if err != nil {
			return nil, err
		}
		for _, d := range deps {
			queue = append(queue, item{d, dep, l.Name})
		}
	}
	if len(newLock) > 0 || len(lock) > 0 {
		if err := cfg.WriteLock(newLock); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// sameLib は a と b が同じ場所か (path、または git の URL・rev・dir)。
func sameLib(a, b Lib) bool {
	return a.Path == b.Path && a.Git == b.Git && a.Rev == b.Rev && a.Dir == b.Dir
}

func (r *Resolver) updating(name string) bool {
	for _, u := range r.Update {
		if u == name || u == "*" {
			return true
		}
	}
	return false
}

func (r *Resolver) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(format, args...)
	}
}

// CacheDir は git のライブラリのキャッシュ (FC_LIB_CACHE か os.UserCacheDir()/fc/lib)。
func CacheDir() (string, error) {
	if d := os.Getenv("FC_LIB_CACHE"); d != "" {
		return d, nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "fc", "lib"), nil
}

// repoKey はリポジトリの URL をキャッシュの名前にする (github.com/owner/repo)。
func repoKey(u string) string {
	s := u
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		s = p.Host + "/" + strings.TrimPrefix(p.Path, "/")
	} else if i := strings.Index(u, "@"); i >= 0 && strings.Contains(u[i:], ":") && !filepath.IsAbs(u) {
		s = strings.Replace(u[i+1:], ":", "/", 1) // git@github.com:owner/repo
	}
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	return regexp.MustCompile(`[^A-Za-z0-9._/-]+`).ReplaceAllString(s, "_")
}

// CachedSource はキャッシュの中の、リポジトリ gitURL のコミット commit を取り出したフォルダ。
func CachedSource(cache, gitURL, commit string) string {
	return filepath.Join(cache, "src", filepath.FromSlash(repoKey(gitURL))+"@"+commit)
}

// fetch は l のコミット (commit が "" なら rev を今のコミットに決める) を取り出したフォルダとコミットを返す。
func (r *Resolver) fetch(l Lib, commit string) (string, string, error) {
	cache, err := CacheDir()
	if err != nil {
		return "", "", err
	}
	key := repoKey(l.Git)
	src := func(c string) string { return CachedSource(cache, l.Git, c) }
	if commit != "" {
		if _, err := os.Stat(src(commit)); err == nil {
			return src(commit), commit, nil
		}
	}
	if r.Offline {
		return "", "", fmt.Errorf("%s is not in the cache (%s); build without --offline to fetch it", l.Git, cache)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "", "", fmt.Errorf("git is needed to fetch %s", l.Git)
	}
	run := func(dir string, args ...string) (string, error) {
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	mirror := filepath.Join(cache, "git", filepath.FromSlash(key)+".git")
	if _, err := os.Stat(mirror); err != nil {
		if err := os.MkdirAll(filepath.Dir(mirror), 0o777); err != nil {
			return "", "", err
		}
		r.logf("fetching %s", l.Git)
		tmp := mirror + ".tmp"
		os.RemoveAll(tmp)
		if _, err := run(filepath.Dir(mirror), "clone", "--bare", "--quiet", l.Git, tmp); err != nil {
			return "", "", err
		}
		if err := os.Rename(tmp, mirror); err != nil {
			return "", "", err
		}
	} else if commit == "" || !r.hasCommit(run, mirror, commit) {
		r.logf("updating %s", l.Git)
		if _, err := run(mirror, "fetch", "--quiet", "--prune", "--tags", "origin", "+refs/heads/*:refs/heads/*"); err != nil {
			return "", "", err
		}
	}
	if commit == "" {
		rev := l.Rev
		if rev == "" {
			rev = "HEAD"
		}
		if commit, err = run(mirror, "rev-parse", "--verify", "--quiet", rev+"^{commit}"); err != nil || commit == "" {
			return "", "", fmt.Errorf("rev %q is not in %s", l.Rev, l.Git)
		}
	}
	dst := src(commit)
	if _, err := os.Stat(dst); err == nil {
		return dst, commit, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o777); err != nil {
		return "", "", err
	}
	tmp := dst + ".tmp"
	os.RemoveAll(tmp)
	if _, err := run(filepath.Dir(dst), "clone", "--quiet", "--shared", "--no-checkout", mirror, tmp); err != nil {
		return "", "", err
	}
	if _, err := run(tmp, "checkout", "--quiet", "--detach", commit); err != nil {
		os.RemoveAll(tmp)
		return "", "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.RemoveAll(tmp)
		if _, err2 := os.Stat(dst); err2 != nil { // 同時に取ってきた別のビルドが先に置いたなら、それを使う
			return "", "", err
		}
	}
	return dst, commit, nil
}

func (r *Resolver) hasCommit(run func(string, ...string) (string, error), mirror, commit string) bool {
	_, err := run(mirror, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// LibDirs は決まったライブラリの探索先 (それぞれ <lib> と、あれば <lib>/<target>)。2 つのライブラリに同じ名前のモジュールがあれば
// エラー (どれを使うかが fc.toml の順で黙って決まらないように)。
func LibDirs(libs []ResolvedLib, target string) ([]string, error) {
	var dirs []string
	owner := map[string]string{}
	for _, l := range libs {
		for _, d := range []string{l.Root, filepath.Join(l.Root, target)} {
			st, err := os.Stat(d)
			if err != nil || !st.IsDir() {
				continue
			}
			dirs = append(dirs, d)
			files, _ := filepath.Glob(filepath.Join(d, "*.fc"))
			for _, f := range files {
				m := strings.TrimSuffix(filepath.Base(f), ".fc")
				if o, ok := owner[m]; ok && o != l.Name {
					return nil, &diag.Error{Msg: fmt.Sprintf("module %s is in both library %s and library %s (rename one, or remove one of the libraries from fc.toml)", m, o, l.Name)}
				}
				owner[m] = l.Name
			}
		}
	}
	return dirs, nil
}
