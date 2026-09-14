package main

// fcc version / --version: バージョン表示。
//
// version は goreleaser (または go build -ldflags "-X main.version=...") で埋め込む。
// 埋め込みが無ければ go のビルド情報 (モジュールのバージョン、VCS リビジョン) から組み立てる。

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var version = "" // -X main.version=v1.2.3

func versionString() string {
	v := version
	info, ok := debug.ReadBuildInfo()
	if v == "" && ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	if v == "" {
		v = "devel"
	}
	rev, dirty := "", ""
	if ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if len(s.Value) > 12 {
					rev = s.Value[:12]
				} else {
					rev = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" {
					dirty = " (modified)"
				}
			}
		}
	}
	s := "fcc " + v
	if rev != "" {
		s += " " + rev + dirty
	}
	return s + " " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}

func runVersion() int {
	fmt.Println(versionString())
	return 0
}
