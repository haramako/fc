package sema

// HLC: 構文木 (internal/syntax) → 中間コード (ir.Lambda.Ops)。lib/fc/hlc.rb 由来。
//
// 入力は型付き構文木で、定数評価は純関数 (構文木は変異しない。評価結果は cexpr に持つ)。
// tmp_count の採番順・エラーメッセージ文言は旧実装と同一で、生成される IR はバイト単位で一致する。
// プログラム横断の状態と 2 相コンパイルの駆動は program.go。

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/haramako/fc/internal/diag"
	"github.com/haramako/fc/internal/ir"
	"github.com/haramako/fc/internal/syntax"
	"github.com/haramako/fc/internal/types"
)

// Hlc はモジュール単位の意味解析コンテキスト (HLC = High Level Compiler、lib/fc/hlc.rb 由来の名)。
// プログラム横断の状態は prog に、依存モジュールの解決は deps に委ねる。
type Hlc struct {
	prog *Program
	deps Resolver

	// モジュール (と宣言) の間の状態
	module     *ir.Module
	scope      *Scope // 今のスコープ (ブロックに入るたびに子を作る: inScope)
	groupBss   string // innermost placement block; module default is applied after declarations
	inStaticIf bool   // トップレベルの @if の選ばれた側の宣言をコンパイル中 (@(build) の const は置けない)
	// constIndex は const の宣言の初期値を評価中 (fc 4): 定数の配列 (文字列) を定数の添字で引く式を畳む (`const C = "#"[0];`)。
	// 関数の中の式では畳まない (生成コードが変わる。fc 3 → 4 の migrate は ROM を変えない)
	constIndex bool
	// 型付きの定数の畳み込みと A1 (widen.go): 折り返した畳み込みの結果の定数 → 元の式。const の値として文・関数をまたいで
	// 使われるのでモジュールの間持つ
	taint  map[*ir.Value]*cexpr
	curPos syntax.Position // 処理中の文/式の位置 (CompileError に位置が無いとき補完する)

	fnState   // 関数をコンパイルしている間の状態 (compileLambda が入口で作り、出口で前のものに戻す)
	stmtState // 1 つの文の式を評価している間の状態 (文ごとに作り直す: resetStmt)
}

// fnState は 1 つの関数をコンパイルしている間の状態。関数の外 (宣言・const の評価) ではゼロ値。
type fnState struct {
	lmd          *ir.Lambda
	loops        []breakable   // 囲んでいるループ/switch (内側が末尾)
	pendingLabel *syntax.Ident // 直前の `L:` ラベル。次に始まるループ/switch が引き取る
	fastCalling  bool
	pendingLogs  []*ir.LogPoint           // 次に出す命令に付ける @log (log.go)
	arith        map[*ir.Value]*arithNode // 式の中の算術の命令の結果 (fc 4 への書き換え・符号の混ざった演算: widen.go)
	shifts       []*ir.Value              // 文の終わりに F2 (値が必ず 0 になるシフト) を見る、量が定数のシフトの結果 (intrules.go)
	caseDecls    map[string]bool          // fc 3: switch の case の中で宣言した名前 (case の外で使ったときの案内。compileCaseBody)
	loopVars     map[*ir.Value]bool       // fc 3: for-each の変数 (読み取り専用。forin.go)
	exprAliases  map[*ir.Value]*cexpr     // fc 3: 式の別名になった名前 (for-each の要素のポインタ `&A[i]`。forin.go)
	aliasNodes   map[*cexpr]string        // 別名の評価済みの式 → 名前 (代入の検査。forin.go)
}

// stmtState は 1 つの文の式を評価している間の状態。文の始まりと、エラーで抜けた文の後始末で捨てる (resetStmt)。
type stmtState struct {
	// constEval のメモ。同一の未評価ノードが複数箇所から共有されるとき (`+=` の脱糖)、
	// 2 回目以降は 1 回目の評価結果を返す (旧実装の破壊的評価と同じ挙動)
	cmemo map[*cexpr]*cexpr
	// constSlice のメモ (同じ配列リテラルから無名の配列定数を 2 度作らない)
	sliceMemo   map[sliceKey]*cexpr
	macroCallee *cexpr // 実行中のマクロの呼び出しの関数の式 (fc 3 → 4 の書き換えで名前を置き換える: printf → @printf)
	// 型を決める段 (typing.go) のメモ: 式の節点 → 型。expansions は IR を出さないマクロの呼び出し (評価済みの節点) → 展開
	// (型を決める段と lval が同じ展開を使う)
	typed      map[*cexpr]typedMemo
	expansions map[*cexpr]macroResult
	// condStores は条件式の値を集めた一時変数 → 枝ごとの書き込み (condValue)。変数の初期化 `var x = c ? a : b` は書き先を
	// 変数に替えて、一時変数からの写しを出さない (retargetCond)
	condStores map[*ir.Value][]*ir.Op
}

// resetStmt は文の状態を捨てる (文の始まりと、エラーで抜けた文の後始末)。
func (h *Hlc) resetStmt() { h.stmtState = stmtState{} }

// MacroFn は組み込みマクロ (printf / unittest_run_tests / textmap など。builtins.go)。
// args は評価済みの実引数、block は呼び出しの後置ブロック。
type MacroFn func(h *Hlc, args []*cexpr, block *syntax.Block) macroResult

// macroResult はマクロの展開結果。expr (式として展開) と stmts (式文の列として展開) は排他。
// どちらも nil なら何も展開しない (asm マクロのように副作用のみ)。
type macroResult struct {
	expr  *cexpr
	stmts []*cexpr
}

// ---------------------------------------------------------------
// ユーティリティ
// ---------------------------------------------------------------

// updatePos は現在処理中の文の位置を記録する (CompileError に付与するため)。
// 位置を持たない合成ノード (while/for の脱糖) では更新しない。
func (h *Hlc) updatePos(s syntax.Node) {
	if p := s.Pos(); p.IsValid() {
		h.curPos = syntax.At(h.module.Path, p)
	}
}

// enterExpr は式の評価中だけ現在位置をその式に移す。返り値を defer で呼んで元に戻す。
// 位置を持たない式 (マクロ展開や合成ノード) では何もしない。
// 評価中に位置のない CompileError が panic で通過したら、そのときの位置を付けてから伝搬させる
// (巻き戻し中に位置が親へ戻ってしまい、最終的に文頭しか指せなくなるのを防ぐ)。
func (h *Hlc) enterExpr(pos syntax.Pos) func() {
	if !pos.IsValid() {
		return func() {}
	}
	old := h.curPos
	h.curPos = syntax.At(h.module.Path, pos)
	return func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*diag.Error); ok && !ce.Pos.IsValid() {
				ce.Pos = h.curPos
			}
			h.curPos = old
			panic(r)
		}
		h.curPos = old
	}
}

// tmpCount はモジュール内の連番を進めて返す。
// モジュール単位で閉じているので、モジュールを単独で再コンパイルしても同じ名前になる (C5)。
func (h *Hlc) tmpCount() int {
	h.module.Seq++
	return h.module.Seq
}

func (h *Hlc) tmpName(prefix string) string {
	return fmt.Sprintf("%s%d", prefix, h.tmpCount())
}

func (h *Hlc) defmacro(name string, fn MacroFn) *macroDef {
	return h.declareMacro(&macroDef{name: name, fn: fn})
}

// defmacroTyped は型を決める段が呼び出しの型を知る方法 (typing.go の macroTyping) を添えて defmacro する。
func (h *Hlc) defmacroTyped(name string, mt macroTyping, fn MacroFn) *macroDef {
	return h.declareMacro(&macroDef{name: name, fn: fn, typing: &mt})
}

// declareMacro はマクロ m を今のスコープに public で宣言する (IR の変数ではない)。
func (h *Hlc) declareMacro(m *macroDef) *macroDef {
	h.scope.DeclareSym(&Symbol{Name: m.name, Macro: m, public: true})
	return m
}

// addVar は変数/定数を追加する。
func (h *Hlc) addVar(v *ir.Value) *ir.Value {
	if h.lmd != nil {
		h.lmd.Vars = append(h.lmd.Vars, v)
	} else if h.module != nil {
		h.module.Vars = append(h.module.Vars, v)
	}
	h.scope.Declare(v)
	return v
}

// setPublic は今のスコープで宣言した値 v の名前を public にする / しない (public は名前の表の Symbol が持つ)。
func (h *Hlc) setPublic(v *ir.Value, b bool) {
	sym := h.scope.declares[v.Name]
	if sym == nil || sym.Val != v {
		panic(fmt.Sprintf("internal: setPublic: %s is not declared in this scope", v.Name))
	}
	sym.SetPublic(b)
}

func defsFind(defs []*ir.Def, symbol string) bool {
	for _, d := range defs {
		if d.Sym == symbol {
			return true
		}
	}
	return false
}

// addDef は現在の関数 (またはモジュール) に定義を追加し、そのシンボル名を返す。
// モジュールレベルでは `_<module>_<name>` にマングルされる。同名が既にあれば追加しない。
func (h *Hlc) addDef(name string, d *ir.Def) string {
	if h.lmd != nil {
		d.Sym = name
		if !strings.HasPrefix(name, "_") && !strings.HasPrefix(name, "@") {
			// 関数の中の const の表の名前はそのまま ca65 のラベルになる: `z:` / `a:` / `f:` (`zp:` / `abs:` / `far:` なども) は番地の
			// 大きさの指定なので、`const Z = [...]` の `Z:` がアセンブルできなかった。前に付けて ca65 の言葉とぶつからないように
			d.Sym = "_L_" + name
		}
		if !defsFind(h.lmd.Defs, d.Sym) {
			h.lmd.Defs = append(h.lmd.Defs, d)
		}
	} else {
		d.Sym = fmt.Sprintf("_%s_%s", h.module.Id, name)
		if !defsFind(h.module.Defs, d.Sym) {
			h.module.Defs = append(h.module.Defs, d)
		}
	}
	return d.Sym
}

// addDefModule はモジュールにシンボル名そのままの定義を追加する (関数用)。
func (h *Hlc) addDefModule(d *ir.Def) {
	if !defsFind(h.module.Defs, d.Sym) {
		h.module.Defs = append(h.module.Defs, d)
	}
}

func (h *Hlc) inScope(f func()) {
	old := h.scope
	h.scope = NewScope(old)
	f()
	h.scope = old
}

// mustValue は評価済みの値であることを要求する (マクロ引数など)。
func mustValue(c *cexpr) *ir.Value {
	if c.kind != cValue {
		panic(&diag.Error{Msg: "constant value required (got an expression that is evaluated at runtime)"})
	}
	return c.val
}

// mustString は文字列リテラル由来の値であることを要求し、その文字列を返す。
func mustString(c *cexpr) string {
	v := mustValue(c)
	if !v.IsString {
		panic(&diag.Error{Msg: fmt.Sprintf("string literal required (got %s)", describe(v))})
	}
	return v.Str
}

// describe はエラーメッセージ用の値の表示 (名前があれば名前、リテラルは値、それ以外は型)。
func describe(v ir.Operand) string {
	if val := ir.UnderlyingValue(v); val != nil {
		switch {
		case val.Name != "":
			return "`" + val.Name + "`"
		case val.Kind == ir.KindLiteral && val.IsInt:
			return fmt.Sprintf("%d", val.Int)
		case val.IsString:
			return fmt.Sprintf("%q", val.Str)
		}
	}
	return "expression of type " + ir.ValType(v).String()
}

// useModule obtains the interface; top-level names may still resolve on demand.
// importer が触れるのは ModuleInterface だけ (C4)。
func (h *Hlc) useModule(name string) *ModuleInterface {
	m, err := h.deps.Module(name)
	if err != nil {
		panic(err)
	}
	return h.prog.iface(m)
}

// resolveFile は include / incbin のファイル名を解決する (ref: 生成物に埋め込む参照形、abs: 読み込み用)。
func (h *Hlc) resolveFile(name string) (ref, abs string) {
	ref, abs, err := h.deps.File(name)
	if err != nil {
		panic(err)
	}
	return ref, abs
}

// readFile は検索パス上のファイルを読む (incbin、マクロの外部表など)。
func (h *Hlc) readFile(name string) []byte {
	_, abs := h.resolveFile(name)
	data, err := h.deps.ReadFile(abs)
	if err != nil {
		panic(&diag.Error{Msg: err.Error()})
	}
	return data
}

// ---------------------------------------------------------------
// Lambdaのコンパイル
// ---------------------------------------------------------------

// switchTableMin はジャンプテーブルにする case の数の下限 (比較の連鎖と表の損益分岐点。docs/reference/language.md の「switch」)。
const switchTableMin = 10

func (h *Hlc) compileLambda(lmd *ir.Lambda) {
	oldFn, oldStmt := h.fnState, h.stmtState
	h.fnState, h.stmtState = fnState{lmd: lmd}, stmtState{}
	defer func() { h.fnState, h.stmtState = oldFn, oldStmt }()
	errs := len(h.prog.Errors)
	h.inScope(func() {
		// 帰り値の追加
		if lmd.Type.Base.Kind != types.Void {
			rt, ro := h.storageType(lmd.Type.Base)
			lmd.Result = ir.NewLocal("$result", rt, ir.LTResult)
			lmd.Result.ReadOnly = ro
			lmd.Vars = append([]*ir.Value{lmd.Result}, lmd.Vars...)
		}

		// 引数の追加
		lmd.Args = make([]*ir.Value, len(lmd.Params))
		for i, p := range lmd.Params {
			pt, ro := h.storageType(p.Type)
			lmd.Args[i] = h.addVar(ir.NewLocal(p.Name, pt, ir.LTArg))
			lmd.Args[i].ReadOnly = ro
		}

		if body := h.prog.bodies[lmd]; body != nil {
			h.compileStmts(body.Stmts)
			if lmd.Type.Base.Kind != types.Void && !h.terminates(body) {
				// 終端に落ちると rts が無く次の関数へ流れて暴走する (fc 1 は黙って通していた)
				hint := ""
				if n := len(body.Stmts); n > 0 && h.switchWithoutDefault(body.Stmts[n-1]) {
					hint = "; the switch has no default, so a value matching no case (even for an enum: `5 as E`) falls through: add `default:`"
				}
				panic(&diag.Error{Msg: fmt.Sprintf("missing return at end of function %s (returns %s)%s", lmd.Name, lmd.Type.Base, hint),
					Pos: syntax.Position{Filename: lmd.Pos.Filename, Line: body.Rbrace.Line, Col: body.Rbrace.Col}})
			}
		}

		// returnを追加する
		var last *ir.Op
		if len(lmd.Ops) > 0 {
			last = lmd.Ops[len(lmd.Ops)-1]
		}
		if last == nil || last.Code != ir.OpReturn {
			if lmd.Type.Base.Kind == types.Void {
				h.emit(&ir.Op{Code: ir.OpReturn})
			}
		}
		for _, p := range h.pendingLogs {
			h.prog.Warnings = append(h.prog.Warnings, diag.Warning{Msg: "@log after the last statement is never reached", Pos: p.Pos})
		}
		if !lmd.Extern && len(h.prog.Errors) == errs {
			h.warnUninitialized(lmd) // エラーのあった関数は命令列が途中なので見ない
		}
	})
}

// ---------------------------------------------------------------
// 文のコンパイル
// ---------------------------------------------------------------

// asmSymbolRe は options(symbol: "...") に書けるシンボル名 (ca65 の識別子。`@` で始まる局所シンボルは除く)。
var asmSymbolRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// warn は警告を記録する (位置は処理中の文/式)。
func (h *Hlc) warn(format string, args ...any) {
	h.prog.Warnings = append(h.prog.Warnings, diag.Warning{Msg: fmt.Sprintf(format, args...), Pos: h.curPos})
}

func (h *Hlc) emit(op *ir.Op) {
	h.requireFunction()
	ir.InferWidthSign(op) // 比較の幅と符号・除算とシフトの符号は、ここで入力の型から決めて命令に持たせる (ir/sign.go)
	op.Pos = h.curPos
	if len(h.pendingLogs) > 0 {
		op.Logs = append(op.Logs, h.pendingLogs...)
		h.pendingLogs = nil
	}
	h.lmd.Ops = append(h.lmd.Ops, op)
}

func (h *Hlc) newLabel(name string) string {
	return h.newLabels(name)[0]
}

func (h *Hlc) newLabels(names ...string) []string {
	r := make([]string, len(names))
	for i, n := range names {
		r[i] = "@" + n + "_" + fmt.Sprintf("%d", h.tmpCount())
	}
	return r
}

func (h *Hlc) newTmp(typ *types.Type) *ir.Value {
	return h.addVar(ir.NewLocal(h.tmpName("$"), typ, ir.LTTemp))
}

// isFarCall は呼び先 fn (関数のシンボルリテラル) が far call (farcall トランポリン経由) になるか (Agent/wiki/design/farcall.md §3.2):
// options(farcall: true) が有効で、呼び先が別モジュールの切替バンクの関数で、options(near: true) が付いていないとき。
// farfn は明示的に far call を選ぶ。通常の fn は呼ぶ側がバンクを管理する。
func (h *Hlc) isFarCall(fn ir.Operand) bool {
	if ir.ValType(fn).IsFarFunc() {
		target := "indirect " + ir.ValType(fn).String()
		if lit := ir.ValLiteral(fn); lit != nil && lit.Kind == ir.KindLiteral && !lit.IsInt {
			target = lit.Symbol
		}
		h.prog.FarCalls = append(h.prog.FarCalls, FarCall{Pos: h.curPos, Caller: h.lmd.Id, Callee: target})
		return true
	}
	if !h.prog.FarCallEnabled() {
		return false
	}
	lit := ir.ValLiteral(fn)
	if lit == nil || lit.Kind != ir.KindLiteral || lit.IsInt {
		return false
	}
	callee, ok := h.prog.lambdas[lit.Symbol]
	if !ok || callee.Options.Flag("near") {
		return false
	}
	if callee.Options.Text("abi") == "cc65" {
		// cc65 規約は A/X で引数を渡すが far call のトランポリンは A/Y を壊す。同じバンクに置くか near を付ける
		if calleeAt, callerAt := h.placementOf(callee), h.placementOf(h.lmd); calleeAt != nil && calleeAt != callerAt && calleeAt.Switchable() {
			panic(&diag.Error{Msg: fmt.Sprintf("cannot call cc65 abi function %s across banks (declare it options(near: true) if it is always mapped, or call it from its module)", callee.Name)})
		}
		return false
	}
	// 置き場所はモジュールのセグメントだが、options(segment: X) で別のモジュールのセグメントに置いた関数はそちらに従う
	// (X がモジュール名でなければ配置は手動なので near)
	calleeAt := h.placementOf(callee)
	callerAt := h.placementOf(h.lmd)
	if calleeAt == nil || calleeAt == callerAt || !calleeAt.Switchable() {
		return false
	}
	h.prog.FarCalls = append(h.prog.FarCalls, FarCall{Pos: h.curPos, Caller: h.lmd.Id, Callee: callee.Id})
	return true
}

// placementOf は関数が置かれるモジュール (= セグメント)。options(segment: X) があれば X という id のモジュール、
// X がモジュールでなければ nil (fc の管理外のセグメント)。
func (h *Hlc) placementOf(lmd *ir.Lambda) *ir.Module {
	if seg := lmd.Segment(); seg != "" {
		if m, ok := h.prog.Modules.Get(seg); ok {
			return m
		}
		return nil
	}
	return lmd.Module
}

// ReadSource はソースファイルを読み込む。改行は CRLF → LF に正規化する (文字列リテラル内の改行が OS で変わらないように)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return normalizeSource(b), nil
}

// normalizeSource はソースの改行を CRLF → LF に正規化する。
func normalizeSource(b []byte) []byte { return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")) }
