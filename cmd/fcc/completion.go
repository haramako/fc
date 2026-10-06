package main

// fcc completion: シェルの補完。`fcc completion zsh|bash` が出すスクリプトは、補完のたびに `fcc __complete 語...` を呼び、
// fcc が kong のコマンドの木 (cli の構造体) から候補を返す (コマンドやオプションを足してもスクリプトは書き直さなくてよい)。
//
// __complete の出力は 1 行 1 候補の `語<TAB>説明`、または次の指示:
//
//	:files [GLOB]   ファイルを補完する (GLOB があればそれに合うものとディレクトリ)
//	:dirs           ディレクトリを補完する
//
// オプションの値と位置の引数の候補はタグ `complete:"..."` で決める: `files` / `files=GLOB` / `dirs` / `none` / `値,値,...`。
// 無ければ enum の値、プレースホルダが FILE ならファイル・DIR ならディレクトリ、位置の引数は `*.fc` のファイル。

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
)

type completionCmd struct {
	Shell string `arg:"" enum:"zsh,bash" help:"The shell (zsh / bash)."`
}

func (c *completionCmd) Help() string {
	return `Prints a completion script. To enable it:
  zsh:  fcc completion zsh > "${fpath[1]}/_fcc"   (or add 'source <(fcc completion zsh)' to ~/.zshrc after compinit)
  bash: add 'source <(fcc completion bash)' to ~/.bashrc`
}

func (c *completionCmd) run() int {
	if c.Shell == "zsh" {
		fmt.Print(zshCompletion)
	} else {
		fmt.Print(bashCompletion)
	}
	return 0
}

const zshCompletion = `#compdef fcc
# fcc の補完 (fcc completion zsh が出す)。候補は fcc __complete が返す。
_fcc() {
  local -a out vals
  local line w d
  out=("${(@f)$(${words[1]} __complete "${(@)words[2,CURRENT]}" 2>/dev/null)}")
  for line in "${out[@]}"; do
    case $line in
      (':files '*) _files -g "${line#:files }" ;;
      (':files') _files ;;
      (':dirs') _files -/ ;;
      ('') ;;
      (*) w=${${line%%$'\t'*}//:/\\:} d=${line#*$'\t'}
          if [[ -n $d && $d != $line ]]; then vals+=("$w:$d"); else vals+=("$w"); fi ;;
    esac
  done
  (( ${#vals} )) && _describe -t values fcc vals
  return 0
}
if [[ "$funcstack[1]" = "_fcc" ]]; then
  _fcc "$@"
else
  compdef _fcc fcc
fi
`

const bashCompletion = `# fcc の補完 (fcc completion bash が出す)。候補は fcc __complete が返す。
_fcc() {
  local cur="${COMP_WORDS[COMP_CWORD]}" line g
  local IFS=$'\n'
  COMPREPLY=()
  for line in $("${COMP_WORDS[0]}" __complete "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null); do
    case "$line" in
      ':files '*) g="${line#:files }"; COMPREPLY+=($(compgen -d -- "$cur") $(compgen -f -X "!$g" -- "$cur")) ;;
      ':files') COMPREPLY+=($(compgen -f -- "$cur")) ;;
      ':dirs') COMPREPLY+=($(compgen -d -- "$cur")) ;;
      *) COMPREPLY+=("${line%%$'\t'*}") ;;
    esac
  done
}
complete -o filenames -F _fcc fcc
`

// printCompletions は fcc __complete の出力。
func printCompletions(root *kong.Node, words []string) {
	if len(words) == 0 {
		words = []string{""}
	}
	for _, c := range complete(root, words) {
		fmt.Fprintln(os.Stdout, c)
	}
}

// complete は words (fcc の後ろの語。最後が補完する語で、空でもよい) の候補。
func complete(root *kong.Node, words []string) []string {
	cur := words[len(words)-1]
	node, npos := root, 0
	var want *kong.Flag // 次の語を値に取るオプション
	dashdash := false
	for _, w := range words[:len(words)-1] {
		switch {
		case want != nil:
			want = nil
		case dashdash:
			npos++
		case w == "--":
			dashdash = true
		case strings.HasPrefix(w, "--"):
			if f := findFlag(node, w[2:], 0); f != nil && !f.IsBool() && !strings.Contains(w, "=") {
				want = f
			}
		case strings.HasPrefix(w, "-") && len(w) > 1:
			// 短いオプションの並び (`-gt`)。値を取るものが最後の文字なら、次の語がその値 (`-O1` は値まで)
			rs := []rune(w[1:])
			for i, r := range rs {
				if f := findFlag(node, "", r); f != nil && !f.IsBool() {
					if i == len(rs)-1 {
						want = f
					}
					break
				}
			}
		default:
			if ch := findCmd(node, w); ch != nil && npos == 0 {
				node = ch
				continue
			}
			npos++
		}
	}
	switch {
	case want != nil:
		return valueCompletions(want.Value, want.PlaceHolder, cur)
	case !dashdash && strings.HasPrefix(cur, "-"):
		return flagCompletions(node, cur)
	case len(node.Children) > 0 && npos == 0:
		var r []string
		for _, ch := range node.Children {
			if !ch.Hidden && strings.HasPrefix(ch.Name, cur) {
				r = append(r, ch.Name+"\t"+summary(ch.Help))
			}
		}
		return r
	}
	ps := node.Positional
	if len(ps) == 0 {
		return nil
	}
	p := ps[min(npos, len(ps)-1)]
	if npos >= len(ps) && !p.IsSlice() {
		return nil
	}
	return valueCompletions(p, "", cur)
}

// findFlag は node とその親のオプションから、長い名前 name か短い名前 short のものを探す。
func findFlag(node *kong.Node, name string, short rune) *kong.Flag {
	name, _, _ = strings.Cut(name, "=")
	for n := node; n != nil; n = n.Parent {
		for _, f := range n.Flags {
			if (name != "" && f.Name == name) || (short != 0 && f.Short == short) {
				return f
			}
		}
	}
	return nil
}

func findCmd(node *kong.Node, name string) *kong.Node {
	for _, ch := range node.Children {
		if ch.Name == name {
			return ch
		}
		for _, a := range ch.Aliases {
			if a == name {
				return ch
			}
		}
	}
	return nil
}

func flagCompletions(node *kong.Node, cur string) []string {
	var r []string
	for n := node; n != nil; n = n.Parent {
		for _, f := range n.Flags {
			if f.Hidden {
				continue
			}
			help := summary(f.Help)
			if long := "--" + f.Name; strings.HasPrefix(long, cur) {
				r = append(r, long+"\t"+help)
			}
			if f.Short != 0 && !strings.HasPrefix(cur, "--") && strings.HasPrefix("-"+string(f.Short), cur) {
				r = append(r, "-"+string(f.Short)+"\t"+help)
			}
		}
	}
	return r
}

// valueCompletions はオプションの値か位置の引数の候補 (ファイルシステムのものは指示の行)。
func valueCompletions(v *kong.Value, placeholder, cur string) []string {
	spec := v.Tag.Get("complete")
	switch {
	case spec != "":
	case v.Enum != "":
		spec = v.Enum
	case placeholder == "FILE":
		spec = "files"
	case placeholder == "DIR":
		spec = "dirs"
	case v.Tag.Arg:
		spec = "files=*.fc"
	default:
		return nil
	}
	switch {
	case spec == "none":
		return nil
	case spec == "files":
		return []string{":files"}
	case strings.HasPrefix(spec, "files="):
		return []string{":files " + strings.TrimPrefix(spec, "files=")}
	case spec == "dirs":
		return []string{":dirs"}
	}
	var r []string
	for _, s := range strings.Split(spec, ",") {
		if s != "" && strings.HasPrefix(s, cur) {
			r = append(r, s+"\t")
		}
	}
	return r
}

// summary は help の最初の文 (補完の説明は短く)。
func summary(help string) string {
	help, _, _ = strings.Cut(help, "\n")
	if i := strings.Index(help, ". "); i >= 0 {
		help = help[:i]
	}
	return strings.TrimSuffix(help, ".")
}
