package driver

// v2 の true / false / null と *void。

import (
	"strings"
	"testing"
)

func TestBoolNullVoidPtr(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `struct Node { v:int; next:*Node; }
var nodes:[2]Node;
var handler:fn(int):int;
function twice(x:int):int { return x * 2; }
function count(p:*Node):int
{
	var n = 0;
	while (p != null) {
		n++;
		p = p.next;
	}
	return n;
}
function take(p:*void):int
{
	var q = bitcast<*int>(p);
	return q[0];
}
function main():void
{
	var b:bool = true;
	var c = false;
	var d:int = true;
	if (b && !c) {
		printf("bool ", b, " ", c, " ", d, " ", b as int + 1, "\n");
	}
	nodes[0] = {1, &nodes[1]};
	nodes[1] = {2, null};
	var p:*Node = null;
	printf("null ", count(&nodes[0]), " ", count(p), " ", p == null, " ", null == p, " ", &nodes[0] != null, "\n");
	handler = null;
	var h0 = handler == null;
	handler = twice;
	printf("fn ", h0, " ", handler != null, " ", handler(4), "\n");
	var arr:[2]int = [7, 8];
	var v:*void = arr;
	var w:*void = &nodes[1];
	var x:*void = twice;
	var y:*void = null;
	printf("void ", take(v), " ", take(arr), " ", bitcast<*Node>(w).v, " ", bitcast<fn(int):int>(x)(5), " ", y == null, " ", v == w, "\n");
	exit(0);
}
`)
	want := "bool 1 0 1 2\nnull 2 0 1 1 1\nfn 1 1 8\nvoid 7 7 2 10 1 0\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}

func TestBoolNullVoidPtrErrors(t *testing.T) {
	t.Parallel()
	cases := []struct{ src, want string }{
		{"function main():void { var p = null; }\n", "null needs a context"},
		{"function main():void { var n:int = null; }\n", "null cannot be used as u8"},
		{"struct P { x:int; }\nsoa Ps:[4]P;\nfunction main():void { var p:*Ps = null; }\n", "has no null"},
		{"function main():void { var p:*void; var q = *p; }\n", "cannot dereference *void"},
		{"function main():void { var p:*void; p[1] = 0; }\n", "cannot index *void"},
		{"function main():void { var p:*void; p += 1; }\n", "no arithmetic on *void"},
		{"function main():void { var p:*void; var q:*int = p; }\n", "not compatible type"},
	}
	for _, c := range cases {
		got := compileErr(t, c.src)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q:\n  got  %q\n  want /%s/", c.src, got, c.want)
		}
	}
}

// TestConstPointerArrayLiteral: `const P:*int = "..."` / `= [...]` は配列定数の宣言 (データそのものに名前を付ける)。
// 以前はポインタ変数扱いになり、データの先頭 2 バイトをアドレスとして読んでいた。
func TestConstPointerArrayLiteral(t *testing.T) {
	t.Parallel()
	out := runEmu(t, `const A:*int = "HELLO";
const B:*int16 = [1000, 2000];
function first(p:*int):int { return p[0]; }
function main():void
{
	printf(A[1], " ", first(A), " ", B[1], " ", sizeof(A), "\n");
	exit(0);
}
`)
	want := "69 72 2000 6\n"
	if out != want {
		t.Errorf("got %q\nwant %q", out, want)
	}
}
