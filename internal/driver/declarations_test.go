package driver

import "testing"

func TestForwardDeclarationsExecution(t *testing.T) {
	out := runEmu(t, `
const TABLE=[draw,update];
const TOTAL=COUNT*sizeof(Item);
var items:[COUNT]Item;
const origin=Item{x:7,y:300};
function main():void {
 items[0]=origin;
 TABLE[0](); TABLE[1](); TABLE[0]();
 printf("size=",TOTAL,",",sizeof(items),"\n");
 exit(0);
}
function draw():void { printf(items[0].x,",",items[0].y,"\n"); }
function update():void { items[0].y+=COUNT; }
const COUNT=BASE+1;
struct Item {x:u8; y:u16;}
const BASE=3;
`)
	if want := "7,300\n7,304\nsize=12,12\n"; out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}
func TestRecursiveSoaDeclarationsExecution(t *testing.T) {
	out := runEmu(t, `
soa Nodes:[COUNT]Node;
var head:*Nodes;
struct Node {next:*Nodes; value:u8;}
const COUNT=4;
function main():void {
 head=&Nodes[0]; head.value=7;
 head.next=&Nodes[1]; head.next.value=9;
 printf(head.value,",",head.next.value,",",sizeof(Node),"\n");
 exit(0);
}
`)
	if want := "7,9,2\n"; out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}
