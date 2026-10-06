package driver

import (
	"strings"
	"testing"

	"github.com/haramako/fc/internal/syntax"
)

// TestMethods: fc 4 の struct・soa のメソッド (sema/method.go)。受け取り手の形 (値・ポインタ・読むだけのポインタ・soa のハンドル) に
// 呼ぶ側を合わせる (変数なら &、ポインタなら中身を写す、soa の要素はハンドルに)、ほかのモジュールの public なメソッド、`T.m(p, …)`
// の明示の呼び出しと関数の値、メソッドの中からのメソッドの呼び出し。
func TestMethods(t *testing.T) {
	t.Parallel()
	out, err := buildBothLevels(t, map[string]string{"geom.fc": `#fc 4
public struct Point {
	x:u8;
	y:u8;
}
public function Point.add(self:Point, o:Point):Point { return {self.x + o.x, self.y + o.y}; }
public function Point.move(self:*Point, dx:u8):void { self.x += dx; }
public function Point.len2(self:*const Point):u8 { return self.x * self.x + self.y * self.y; }
public function Point.twice(self:*Point):void { self.move(self.x); }
`, "t.fc": `#fc 4
use console;
use geom;
struct Enemy {
	x:u8;
	hp:u8;
	think:fn(u8):u8;
}
soa Enemies:[4]Enemy;
function Enemies.hit(self:*Enemies, d:u8):void { self.hp -= d; }
function Enemies.alive(self:*Enemies):bool { return self.hp != 0; }
function Enemy.sum(self:Enemy):u8 { return self.x + self.hp; }
function double(n:u8):u8 { return n * 2; }
const ORIGIN = geom.Point{x: 1, y: 2};
function main():void
{
	var p:geom.Point = {3, 4};
	p.move(2);
	var q = p.add({10, 20});
	var pp = &p;
	pp.move(1);
	pp.twice();
	var f = geom.Point.len2;
	@printf("{} {} {} {} {} {} {}\n", p.x, p.y, q.x, q.y, pp.len2(), geom.Point.len2(&q), f(&p));
	@printf("{} {}\n", ORIGIN.len2(), ORIGIN.add(p).x);
	Enemies[1].x = 5;
	Enemies[1].hp = 10;
	Enemies[1].think = double;
	Enemies[1].hit(3);
	var e = &Enemies[1];
	e.hit(2);
	@printf("{} {} {} {} {}\n", Enemies[1].hp, Enemies[1].sum(), e.sum(), e.alive(), Enemies[1].think(4));
	e.hit(5);
	@printf("{}\n", Enemies[1].alive());
	console.exit(0);
}
`})
	want := "12 4 15 24 160 33 160\n5 13\n5 10 10 true 8\nfalse\n"
	if err != nil || out != want {
		t.Errorf("got %q, %v\nwant %q", out, err, want)
	}
}

// TestMethodErrors: メソッドの宣言と呼び出しの誤り。
func TestMethodErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, msg string }{
		{"struct P { x:u8; }\nfunction P.x(self:P):u8 { return 0; }", "P has a field x"},
		{"struct P { x:u8; }\nfunction P.f(a:u8):u8 { return a; }", "the first parameter is the receiver (self:P, self:*P or self:*const P), not u8"},
		{"struct P { x:u8; }\nfunction P.f():void {}", "needs the receiver as the first parameter"},
		{"enum E { A }\nfunction E.f(self:E):void {}", "E is not a struct or soa declared in this module"},
		{"function Q.f(self:u8):void {}", "Q is not a struct or soa declared in this module"},
		{"struct P { x:u8; }\nfunction P.f(self:P):void {}\nfunction P.f(self:*P):void {}", "method P.f already defined"},
		{"struct E { x:u8; }\nsoa S:[2]E;\nfunction S.f(self:E):void {}", "the first parameter is the receiver (self:*S)"},
		{"struct P { x:u8; }\nfunction main():void { var p:P; p.g(); }", "struct t.P has no field or method g"},
		{"struct P { x:u8; }\nfunction P.m(self:*P):void {}\nfunction f():P { return {1}; }\nfunction main():void { f().m(); }", "method P.m changes self"},
		{"use lib;\nfunction main():void { var p:lib.P; p.hidden(); }", "method P.hidden is private"},
		{"use lib;\nfunction main():void { lib.P.hidden({1}); }", "method P.hidden is private"},
	} {
		files := map[string]string{"t.fc": "#fc 4\n" + c.src + "\n", "lib.fc": "#fc 4\npublic struct P { x:u8; }\nfunction P.hidden(self:P):void {}\n"}
		_, err := buildFiles(t, files)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s: got %v, want /%s/", c.src, err, c.msg)
		}
	}
	// fc 3 では書けない
	if _, err := syntax.Parse([]byte("#fc 3\nstruct P { x:u8; }\nfunction P.f(self:P):void {}\n"), "t.fc"); err == nil || !strings.Contains(err.Error(), "fc 4 syntax") {
		t.Errorf("fc 3: got %v", err)
	}
}
