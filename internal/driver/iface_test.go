package driver

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// ifaceTask は soa の interface と、2 つのモジュールの実装 (手動の ID と自動の ID、u16 のフィールド、既定の本体を使う実装)。
var ifaceTask = map[string]string{
	"task.fc": `#fc 4
public soa interface Task {
	x:u8;
	y:u8;
	function init(self:*Task, p:u8):void;
	function process(self:*Task):void;
	function score(self:*Task):u8 { return self.x / 2; }
}
public soa Tasks:[8]Task;
public var log:[8]u8;
`,
	"en1.fc": `#fc 4
use task;
public struct Slime: task.Task = 3 {
	dir:u8;
	speed:u8;
}
public function Slime.init(self:*Slime, p:u8):void { self.dir = p; self.speed = 2; }
public function Slime.process(self:*Slime):void { self.x += self.dir * self.speed; self.step(); }
public function Slime.score(self:*Slime):u8 { return self.x + 100; }
function Slime.step(self:*Slime):void { task.log[0] += 1; }
`,
	"en2.fc": `#fc 4
use task;
public struct Bubble: task.Task {
	life:u16;
	kind:i8;
}
public function Bubble.init(self:*Bubble, p:u8):void { self.life = 1000 + p; self.kind = -3; }
public function Bubble.process(self:*Bubble):void { self.life -= 1; self.y = self.life as u8; self.kind = self.kind_of() - 1; task.log[1] = self.kind as u8; }
function Bubble.kind_of(self:*Bubble):i8 { return self.kind; }
`,
}

// TestInterfaceSoa: soa の interface (sema/iface.go)。データの番号から @set_id と init で作る、ID で振り分けて呼ぶ、.none と実装に
// 無いメソッドは既定の本体か何もしない、共通のフィールド・実装のフィールド (重ねて置く列)、Task.Id の enum、実装のハンドルでの
// 直接の呼び出し。
func TestInterfaceSoa(t *testing.T) {
	t.Parallel()
	files := map[string]string{"t.fc": `#fc 4
use console;
use task;
use en1;
use en2;
use Task from task;
const MAP:[?]u8 = [3, 1, 0, 3];
function main():void
{
	for (var i, k in MAP) {
		var e = &task.Tasks[i];
		@set_id(e, k as Task.Id);
		e.x = 10 * i;
		e.init(i + 1);
	}
	for (var n = 0; n < 3; n++) {
		for (var i = 0; i < 4; i++) {
			task.Tasks[i].process();
		}
	}
	for (var i = 0; i < 4; i++) {
		var e = &task.Tasks[i];
		@printf("{} {} {} {} {}\n", @id_of(e) as u8, e.x, e.y, e.score(), @id_of(e) == .Slime);
	}
	@printf("{} {} {} {} {}\n", Task.Id.Bubble, @len(Task.Id), Task.Id.none, task.log[1] as i8, task.log[0]);
	@set_id(&task.Tasks[0], .none);
	task.Tasks[0].process();
	@printf("{} {}\n", task.Tasks[0].x, task.log[0]);
	task.Tasks[5] = en1.Slime{x: 7, y: 1, dir: 2, speed: 3};
	task.Tasks[6] = en1.Slime{8, 2, 1, 1};
	var v = 9;
	task.Tasks[7] = en2.Bubble{x: v, life: 300};
	for (var i = 5; i < 8; i++) {
		task.Tasks[i].process();
		var e = &task.Tasks[i];
		@printf("{} {} {} {}\n", @id_of(e) as u8, e.x, e.y, e.score());
	}
	console.exit(0);
}
`}
	for k, v := range ifaceTask {
		files[k] = v
	}
	_, res, err := buildFilesDefs(t, files, nil)
	if err != nil || len(res.Interfaces) != 1 || res.Interfaces[0] != "  task.Task (soa Tasks): none = 0, Bubble = 1 (en2), Slime = 3 (en1)" {
		t.Errorf("interfaces (fcc build -d): %q, %v", res.Interfaces, err)
	}
	out, err := buildBothLevels(t, files)
	want := "3 6 0 106 true\n1 10 231 5 false\n0 20 0 10 false\n3 54 0 154 true\n1 3 0 -6 6\n6 6\n3 13 1 113\n3 9 2 109\n1 9 43 4\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestInterfacePlain: 普通の (soa でない) interface。要素は ID・共通のフィールド・実装のフィールドを重ねた塊 (大きさは一番大きい実装)。
func TestInterfacePlain(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
interface Shape {
	x:u8;
	function area(self:*Shape):u16 { return 0; }
	function grow(self:*Shape, d:u8):void;
}
struct Circle: Shape { r:u8; }
struct Rect: Shape = 5 { w:u8; h:u16; }
function Circle.area(self:*Circle):u16 { return self.r * self.r * 3; }
function Circle.grow(self:*Circle, d:u8):void { self.r += d; }
function Rect.area(self:*Rect):u16 { return self.w * self.h; }
function Rect.grow(self:*Rect, d:u8):void { self.w += d; self.h += d; }
var shapes:[3]Shape;
function main():void
{
	@set_id(&shapes[0], .Circle);
	@set_id(&shapes[1], .Rect);
	shapes[0].grow(4);
	shapes[1].grow(10);
	var p = &shapes[1];
	p.grow(1);
	var c:Shape = shapes[0];
	shapes[2] = Rect{x: 1, w: 3, h: 4};
	@printf("{} {} {} {} {}\n", shapes[0].area(), p.area(), shapes[2].area(), c.area(), @sizeof(Shape));
	@printf("{} {}\n", @id_of(p) as u8, @id_of(&shapes[0]) == .Circle);
	console.exit(0);
}
`})
	want := "48 121 12 48 5\n5 true\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestInterfaceFarNES: far の interface (`@(far)`)。実装を別々のバンクに置き、ID で引いた表 (farfn) でバンクを切り替えて呼ぶ。
func TestInterfaceFarNES(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[target]\nmapper = \"UxROM\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n",
		"task.fc": `#fc 4
public soa interface Task @(far) {
	x:u8;
	function process(self:*Task, d:u8):u8;
}
public soa Tasks:[4]Task;
`,
		"ea.fc": `#fc 4
@(bank: "a");
use task;
const T:[2]u8 = [10, 20];
public struct A: task.Task { n:u8; }
public function A.process(self:*A, d:u8):u8 { self.n += d; return self.n + T[1]; }
`,
		"eb.fc": `#fc 4
@(bank: "b");
use task;
const T:[2]u8 = [50, 60];
public struct B: task.Task { m:u16; }
public function B.process(self:*B, d:u8):u8 { self.m += d * 100; return (self.m >> 8) as u8 + T[0]; }
`,
		"main.fc": `#fc 4
@(farcall);
use uxrom;
use task;
use ea;
use eb;
use Task from task;
public var out:[16]u8;
public var done:u8;
function main():void
{
	uxrom.init();
	uxrom.prg(@bank("a"));
	@set_id(&task.Tasks[0], .A);
	@set_id(&task.Tasks[1], .B);
	@set_id(&task.Tasks[2], .A);
	var k:u8 = 0;
	for (var n = 0; n < 3; n++) {
		for (var i = 0; i < 3; i++) {
			out[k] = task.Tasks[i].process(i + 1);
			k++;
		}
	}
	out[9] = uxrom.bank;
	done = 1;
	while (true) {
	}
}
`,
	}
	for _, level := range []int{-1, 0, 2} {
		out, done, asm := runNes(t, files, level, 10)
		if !strings.Contains(asm, "farcall") {
			t.Errorf("-O %d: the dispatch does not use far calls", level)
		}
		// A: n = 1, 2, 3 (+20)、B: m = 200, 400, 600 → 上位 0, 1, 2 (+50)、2 つ目の A: n = 3, 6, 9 (+20)。バンクは a (0) に戻る
		if got := strings.Trim(fmtInts(out[:10]), "[]"); done != 1 || got != "21 50 23 22 51 26 23 52 29 0" {
			t.Errorf("-O %d: out=%s done=%d", level, got, done)
		}
	}
}

// TestInterfaceNearBankNES: near の interface で、実装のバンクを自分で切り替えて呼ぶ (@bank_of_id。ループの中で切り替え、戻すのは
// ループの後に 1 回)。
func TestInterfaceNearBankNES(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"fc.toml": "[target]\nmapper = \"UxROM\"\nprg = \"64K\"\n[bank.a]\nslot = 0x8000\n[bank.b]\nslot = 0x8000\n",
		"task.fc": `#fc 4
public soa interface Task {
	function process(self:*Task):u8;
}
public soa Tasks:[4]Task;
`,
		"ea.fc": `#fc 4
@(bank: "a");
use task;
const T:[2]u8 = [10, 20];
public struct A: task.Task { n:u8; }
public function A.process(self:*A):u8 { self.n += 1; return self.n + T[1]; }
`,
		"eb.fc": `#fc 4
@(bank: "b");
use task;
const T:[2]u8 = [50, 60];
public struct B: task.Task { m:u8; }
public function B.process(self:*B):u8 { self.m += 2; return self.m + T[0]; }
`,
		"main.fc": `#fc 4
use uxrom;
use task;
use ea;
use eb;
use Task from task;
public var out:[16]u8;
public var done:u8;
function main():void
{
	uxrom.init();
	uxrom.prg(@bank("a"));
	@set_id(&task.Tasks[0], .B);
	@set_id(&task.Tasks[1], .A);
	@set_id(&task.Tasks[2], .B);
	var old = uxrom.bank;
	for (var n = 0; n < 2; n++) {
		for (var i = 0; i < 4; i++) {
			var e = &task.Tasks[i];
			uxrom.prg(@bank_of_id(@id_of(e)));
			out[n * 4 + i] = e.process();
		}
	}
	uxrom.prg(old);
	out[8] = uxrom.bank;
	out[9] = @bank_of_id(Task.Id.A);
	out[10] = @bank_of_id(Task.Id.B);
	done = 1;
	while (true) {
	}
}
`,
	}
	for _, level := range []int{-1, 0, 2} {
		out, done, _ := runNes(t, files, level, 11)
		// B: m = 2, 4 (+50)、A: n = 1, 2 (+20)、.none は 0。バンクは a (0) に戻る。a は 0、b は 1
		if got := strings.Trim(fmtInts(out[:11]), "[]"); done != 1 || got != "52 21 52 0 54 22 54 0 0 0 1" {
			t.Errorf("-O %d: out=%s done=%d", level, got, done)
		}
	}
}

// TestInterfaceRecursive: 実装のメソッドから別の要素のメソッドを呼ぶ (振り分けの関数を通して呼び出しが輪になる: 再帰の関数として
// スタックに置く)。
func TestInterfaceRecursive(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"t.fc": `#fc 4
use console;
soa interface Node {
	next:u8;
	function sum(self:*Node, depth:u8):u16;
}
soa Nodes:[8]Node;
struct Leaf: Node { v:u8; }
struct Pair: Node { a:u8; b:u8; }
function Leaf.sum(self:*Leaf, depth:u8):u16 { return self.v; }
function Pair.sum(self:*Pair, depth:u8):u16
{
	var s:u16 = self.a + self.b * depth;
	if (self.next != 0) {
		s += Nodes[self.next].sum(depth + 1); // 別の要素 (Leaf でも Pair でも)
	}
	return s;
}
function main():void
{
	Nodes[0] = Pair{next: 1, a: 1, b: 2};
	Nodes[1] = Pair{next: 2, a: 3, b: 4};
	Nodes[2] = Pair{next: 3, a: 5, b: 6};
	Nodes[3] = Leaf{v: 100};
	@printf("{} {}\n", Nodes[0].sum(1), Nodes[2].sum(1));
	console.exit(0);
}
`})
	// 1+2*1 + 3+4*2 + 5+6*3 + 100 = 137、5+6*1 + 100 = 111
	if want := "137 111\n"; err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestInterfaceErrors: interface の宣言・実装・呼び出しの誤り。
func TestInterfaceErrors(t *testing.T) {
	t.Parallel()
	const head = "#fc 4\nsoa interface Task { x:u8; function f(self:*Task):void; function g(self:*Task):u8; }\nsoa Tasks:[4]Task;\n"
	for _, c := range []struct{ src, msg string }{
		{"struct A: Task = 1 { y:u8; }\nfunction A.f(self:*A):void {}\nfunction A.g(self:*A):i16 { return 0; }\nfunction main():void { Tasks[0].g(); }", "method A.g does not match Task.g"},
		{"struct A: Task = 0 { y:u8; }", "the id of A cannot be 0"},
		{"struct A: Task = 1 { y:u8; }\nstruct B: Task = 1 { z:u8; }", "A and B have the same id 1"},
		{"struct A: Task = 300 { y:u8; }", "must be 1..255"},
		{"struct A: Task { x:u8; }", "field x of A is a field of interface Task"},
		{"struct none: Task { y:u8; }", "cannot be named none"},
		{"struct A: Task { y:u8; }\nfunction A.f(self:*A, n:u8):void {}\nfunction main():void { Tasks[0].f(); }", "method A.f does not match Task.f"},
		{"struct P { y:u8; }\nstruct A: P { y:u8; }", "is not an interface"},
		{"struct A: Task { y:u8; }\nsoa As:[4]A;", "is an implementation of Task"},
		{"soa Ts:[4]Task;", "already has the soa Tasks"},
		{"function main():void { var t = Tasks[0]; }", "cannot be copied as a value"},
		{"struct A: Task { y:u8; }\nfunction A.f(self:*A):void { var c = *self; }", "cannot be copied as a value"},
		{"struct A: Task { y:u8; }\nfunction main():void { var a = A{1, 2, 3}; }", "struct t.A has 2 fields but 3 values given"},
		{"function main():void { @set_id(1, 2); }", "@set_id takes an element of an interface"},
		{"interface Q @(near) { function f(self:*Q):void; }", "unknown attribute near"},
		{"interface Q:u16 { function f(self:*Q):void; }", "the id type must be u8"},
		{"interface Q { function f(self:u8):void; }", "the first parameter is the receiver (self:*Q)"},
		{"interface Q { Id:u8; }", "Id is the name of the id type"},
		{"interface Q { function f(self:*Q):void; }\nsoa Qs:[2]Q;", "is not a soa interface"},
		{"soa interface R { function f(self:*R):void; }\nstruct S: R { a:u8; }\nfunction S.f(self:*S):void {}\nfunction main():void { var s:*S; }", "soa interface R has no soa"},
	} {
		_, err := buildFiles(t, map[string]string{"t.fc": head + c.src + "\n"})
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	for _, src := range []string{"#fc 3\ninterface Q { x:u8; }\n", "#fc 3\nstruct A: Q { x:u8; }\n"} {
		if _, err := syntax.Parse([]byte(src), "t.fc"); err == nil {
			t.Errorf("fc 3: %q should not parse", src)
		}
	}
	// fc 3 では interface は名前
	if _, err := syntax.Parse([]byte("#fc 3\nvar interface:u8;\n"), "t.fc"); err != nil {
		t.Errorf("fc 3 interface as a name: %v", err)
	}
}
