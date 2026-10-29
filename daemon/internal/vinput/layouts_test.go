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

// The letters, digits and a few characters of the German, French and Spanish layouts
func TestStrokesMoreLayouts(t *testing.T) {
	cases := []struct {
		layout, text string
		want         []stroke
	}{
		{"de", "zyß@", []stroke{{code: keyY}, {code: keyZ}, {code: keyMinus}, {code: keyQ, altgr: true}}},
		{"de", "Ü1!", []stroke{{code: keyLeftBrace, shift: true}, {code: key1}, {code: key1, shift: true}}},
		{"fr", "aqmé1", []stroke{{code: keyQ}, {code: keyA}, {code: keySemicolon}, {code: key1 + 1}, {code: key1, shift: true}}},
		{"fr", "@,!", []stroke{{code: key0, altgr: true}, {code: keyM}, {code: keySlash}}},
		{"es", "ñÑ¿@", []stroke{{code: keySemicolon}, {code: keySemicolon, shift: true}, {code: keyEqual, shift: true}, {code: key1 + 1, altgr: true}}},
	}
	for _, c := range cases {
		got, err := Strokes(c.layout, c.text)
		if err != nil {
			t.Errorf("%s %q: %v", c.layout, c.text, err)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("%s %q stroke %d: got %+v want %+v", c.layout, c.text, i, got[i], c.want[i])
			}
		}
	}
}

// Every letter and digit can be typed on every layout, each on its own stroke
func TestLettersAndDigitsOnEveryLayout(t *testing.T) {
	for name := range Layouts {
		seen := map[stroke]rune{}
		for _, c := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 " {
			s, err := Strokes(name, string(c))
			if err != nil {
				t.Errorf("%s: %q: %v", name, c, err)
				continue
			}
			if other, ok := seen[s[0]]; ok {
				t.Errorf("%s: %q and %q on the same stroke", name, other, c)
			}
			seen[s[0]] = c
		}
	}
}
