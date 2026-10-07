// SPDX-License-Identifier: GPL-3.0-or-later

package vinput

import (
	"errors"
	"testing"
)

func TestStrokesUS(t *testing.T) {
	got, err := Strokes("us", "Ab1!@ ?")
	if err != nil {
		t.Fatal(err)
	}
	want := []stroke{
		{code: keyA, shift: true}, {code: keyA + 18}, {code: key1}, {code: key1, shift: true},
		{code: key1 + 1, shift: true}, {code: keySpace}, {code: keySlash, shift: true},
	}
	// 'b' is KEY_B = 48 = keyA + 18
	if len(got) != len(want) {
		t.Fatalf("got %d strokes", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stroke %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestStrokesItalian(t *testing.T) {
	got, err := Strokes("it", "è@€{")
	if err != nil {
		t.Fatal(err)
	}
	want := []stroke{
		{code: keyLeftBrace}, {code: keySemicolon, altgr: true}, {code: keyE, altgr: true},
		{code: keyLeftBrace, shift: true, altgr: true},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stroke %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestUnsupportedCharacterTypesNothing(t *testing.T) {
	_, err := Strokes("it", "ciaoÈ")
	var uc *UnsupportedCharError
	if !errors.As(err, &uc) || uc.Char != 'È' {
		t.Fatalf("got %v", err)
	}
	if _, err := Strokes("us", "è"); err == nil {
		t.Fatal("è accepted on the US layout")
	}
}

func TestEveryPrintableASCIIOnUS(t *testing.T) {
	for c := rune(0x20); c < 0x7f; c++ {
		if _, err := Strokes("us", string(c)); err != nil {
			t.Errorf("%q: %v", c, err)
		}
	}
}
