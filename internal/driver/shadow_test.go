package driver

import (
	"strings"
	"testing"
)

// TestNameShadowing: 名前の隠れの診断 (GitHub の Issue #21)。
//   - `use * from boss` の名前をモジュールの宣言が隠すと警告 (再輸出で戻ってきた自分の宣言は隠していない)
//   - 引数がモジュールを隠して `x.f` がエラーになるとき、引数がモジュールを隠していることをエラーに書く
//   - `use * from boss` の後の `boss.kill()` は、モジュールの名前を付けずに書くことを案内する
func TestNameShadowing(t *testing.T) {
	t.Parallel()
	boss := "#fc 4\npublic var t:u8;\npublic function kill():void\n{\n\tt = 0;\n}\n"
	build := func(main string, extra map[string]string) *buildResult {
		files := map[string]string{"t.fc": main, "boss.fc": boss}
		for k, v := range extra {
			files[k] = v
		}
		return testBuild(t, buildSpec{Files: files, CompileOnly: true})
	}
	r := build("#fc 4\nuse * from boss;\nvar t:u8;\nfunction main():void\n{\n\tt = 1;\n\tkill();\n}\n", nil)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if len(r.Res.Warnings) != 1 || !strings.Contains(r.Res.Warnings[0].Msg, "t hides t of module boss imported by `use * from boss`") {
		t.Errorf("隠れの警告: %v", r.Res.Warnings)
	}

	// 自分の宣言を再輸出するモジュールを取り込んでも警告しない
	r = build("#fc 4\nuse * from hub;\npublic const K = 1;\nfunction main():void\n{\n}\n",
		map[string]string{"hub.fc": "#fc 4\npublic use * from t;\n"})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if len(r.Res.Warnings) != 0 {
		t.Errorf("再輸出の自分の宣言で警告した: %v", r.Res.Warnings)
	}

	r = build("#fc 4\nuse boss;\nfunction copy(boss:[]u8):void\n{\n\tboss.t = boss[0];\n}\nfunction main():void\n{\n}\n", nil)
	if r.Err == nil || !strings.Contains(r.Err.Error(), "boss here is the variable or parameter boss, which hides module boss") {
		t.Errorf("引数がモジュールを隠す: %v", r.Err)
	}

	r = build("#fc 4\nuse * from boss;\nfunction main():void\n{\n\tboss.kill();\n}\n", nil)
	if r.Err == nil || !strings.Contains(r.Err.Error(), "`use * from boss` imports the names of module boss without the module name") || strings.Contains(r.Err.Error(), "did you mean") {
		t.Errorf("use * from の後の boss.: %v", r.Err)
	}
}
