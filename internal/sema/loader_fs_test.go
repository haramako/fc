package sema

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestLoaderFS: Program.FS を渡すと、ソース・use の先のモジュール・@incbin のファイルをディスクでなくそこから読む
// (sema のテストをメモリの上で済ませられる)。改行の CRLF は LF に正規化する。
func TestLoaderFS(t *testing.T) {
	prog := NewProgram()
	prog.FS = fstest.MapFS{
		"t.fc":  {Data: []byte("#fc 4\r\nuse m;\r\nconst D = @incbin(\"d.bin\");\r\nfunction main():void { m.f(@len(D)); }\r\n")},
		"m.fc":  {Data: []byte("#fc 4\npublic function f(n:u16):void { }\n")},
		"d.bin": {Data: []byte{1, 2, 3}},
	}
	if err := CompileProgram(prog, "", []string{"."}, "t.fc"); err != nil {
		t.Fatal(err)
	}
	if _, ok := prog.Modules.Get("m"); !ok {
		t.Errorf("use m のモジュールが読まれていない")
	}
	if src := prog.Sources["t"]; src == nil || strings.Contains(string(src.Src), "\r") {
		t.Errorf("t.fc のソースが無いか、CRLF が残っている")
	}
	err := CompileProgram(NewProgram(), "", []string{"."}, "none.fc")
	if err == nil || !strings.Contains(err.Error(), "file none.fc not found") {
		t.Errorf("無いファイル: %v", err)
	}
}
