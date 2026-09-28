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

	scope        *ir.Scope
	loops        []breakable   // 囲んでいるループ/switch (内側が末尾)
	pendingLabel *syntax.Ident // 直前の `L:` ラベル。次に始まるループ/switch が引き取る
	fastCalling  bool
	groupBss     string // innermost placement block; module default is applied after declarations
	inStaticIf   bool   // トップレベルの @if の選ばれた側の宣言をコンパイル中 (@(build) の const は置けない)

	module *ir.Module
	lmd    *ir.Lambda
	curPos syntax.Position // 処理中の文/式の位置 (CompileError に位置が無いとき補完する)
	arith  map[*ir.Value]*arithNode // 式の中の算術の命令の結果 (A1 で広げる・fc 4 への書き換え: widen.go)

	// constEval のメモ。同一の未評価ノードが複数箇所から共有されるとき (`+=` の脱糖)、
	// 2 回目以降は 1 回目の評価結果を返す (旧実装の破壊的評価と同じ挙動)。文ごとにリセットする
	cmemo map[*cexpr]*cexpr
	// constSlice のメモ (同じ配列リテラルから無名の配列定数を 2 度作らない)。cmemo と一緒にリセットする
	sliceMemo map[sliceKey]*cexpr

	pendingLogs []*ir.LogPoint       // 次に出す命令に付ける @log (log.go)
	caseDecls   map[string]bool      // fc 3: switch の case の中で宣言した名前 (case の外で使ったときの案内。compileCaseBody)
	loopVars    map[*ir.Value]bool   // fc 3: for-each の変数 (読み取り専用。forin.go)
	exprAliases map[*ir.Value]*cexpr // fc 3: 式の別名になった名前 (for-each の要素のポインタ `&A[i]`。forin.go)
	aliasNodes  map[*cexpr]string    // 別名の評価済みの式 → 名前 (代入の検査。forin.go)
}

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

func (h *Hlc) defmacro(name string, fn MacroFn) *ir.Value {
	v := h.addVar(ir.NewGlobal(name, h.prog.Types.Macro(), ""))
	v.Public = true
	h.prog.macros[v] = fn
	return v
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
	h.scope = ir.NewScope(old)
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
func (h *Hlc) useModule(name string) *ir.ModuleInterface {
	m, err := h.deps.Module(name)
	if err != nil {
		panic(err)
	}
	return m.Interface()
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
	data, err := os.ReadFile(abs)
	if err != nil {
		panic(&diag.Error{Msg: err.Error()})
	}
	return data
}

// ---------------------------------------------------------------
// Lambdaのコンパイル
// ---------------------------------------------------------------

// switchTableMin はジャンプテーブルにする case の数の下限 (比較の連鎖と表の損益分岐点。language_reference.md §5)。
const switchTableMin = 10

func (h *Hlc) compileLambda(lmd *ir.Lambda) {
	oldLmd, oldLogs := h.lmd, h.pendingLogs
	h.lmd, h.pendingLogs = lmd, nil
	errs := len(h.prog.Errors)
	if len(h.loops) != 0 {
		panic("loops not empty")
	}
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

		if lmd.Body != nil {
			h.compileStmts(lmd.Body.Stmts)
			if lmd.Type.Base.Kind != types.Void && !h.terminates(lmd.Body) {
				// 終端に落ちると rts が無く次の関数へ流れて暴走する (fc 1 は黙って通していた)
				hint := ""
				if n := len(lmd.Body.Stmts); n > 0 && h.switchWithoutDefault(lmd.Body.Stmts[n-1]) {
					hint = "; the switch has no default, so a value matching no case (even for an enum: `5 as E`) falls through: add `default:`"
				}
				panic(&diag.Error{Msg: fmt.Sprintf("missing return at end of function %s (returns %s)%s", lmd.Name, lmd.Type.Base, hint),
					Pos: syntax.Position{Filename: lmd.Pos.Filename, Line: lmd.Body.Rbrace.Line, Col: lmd.Body.Rbrace.Col}})
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
		if lmd.Body != nil && len(h.prog.Errors) == errs {
			h.warnUninitialized(lmd) // エラーのあった関数は命令列が途中なので見ない
		}
	})
	h.lmd, h.pendingLogs = oldLmd, oldLogs
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

// isFarCall は呼び先 fn (関数のシンボルリテラル) が far call (farcall トランポリン経由) になるか (doc/v2_farcall.md §3.2):
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
	if v, ok := callee.Options.Get("abi"); ok && v.Text() == "cc65" {
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

var reAsmSymbol = regexp.MustCompile(`_[A-Za-z0-9_$]+`)

// ReadSource はソースファイルを読み込む。改行は CRLF → LF に正規化する (文字列リテラル内の改行が OS で変わらないように)。
func ReadSource(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}
