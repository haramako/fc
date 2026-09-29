package rle

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	cases := [][]byte{nil, {5}, {1, 1, 1}, {1, 1, 1, 1}, bytes.Repeat([]byte{7}, 1000), []byte("aaaaabcccccccd")}
	for n := 0; n < 50; n++ {
		b := make([]byte, r.Intn(2000))
		for i := range b {
			if i > 0 && r.Intn(3) != 0 {
				b[i] = b[i-1]
			} else {
				b[i] = byte(r.Intn(40))
			}
		}
		cases = append(cases, b)
	}
	for _, src := range cases {
		c, err := Compress(src)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Decompress(c)
		if err != nil || !bytes.Equal(d, src) {
			t.Fatalf("% x: 元に戻らない (%v)", src[:min(len(src), 16)], err)
		}
	}
	if c, _ := Compress(bytes.Repeat([]byte{0}, 1024)); len(c) != 1+1+2*5+2 {
		t.Errorf("1024 個の 0: % x", c)
	}
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	if _, err := Compress(all); err == nil {
		t.Errorf("全部の値が現れるのにエラーにならない")
	}
}
