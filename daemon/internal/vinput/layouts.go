// SPDX-License-Identifier: GPL-3.0-or-later

package vinput

// Key codes (linux/input-event-codes.h) used by the layouts.
const (
	keyEsc        = 1
	key1          = 2
	key0          = 11
	keyMinus      = 12
	keyEqual      = 13
	keyBackspace  = 14
	keyTab        = 15
	keyQ          = 16
	keyW          = 17
	keyE          = 18
	keyY          = 21
	keyLeftBrace  = 26
	keyRightBrace = 27
	keyEnter      = 28
	keyLeftCtrl   = 29
	keyA          = 30
	keySemicolon  = 39
	keyApostrophe = 40
	keyGrave      = 41
	keyLeftShift  = 42
	keyBackslash  = 43
	keyZ          = 44
	keyM          = 50
	keyComma      = 51
	keyDot        = 52
	keySlash      = 53
	keyLeftAlt    = 56
	keySpace      = 57
	key102nd      = 86
	keyRightAlt   = 100 // AltGr
	keyHome       = 102
	keyUp         = 103
	keyPageUp     = 104
	keyLeft       = 105
	keyRight      = 106
	keyEnd        = 107
	keyDown       = 108
	keyPageDown   = 109
	keyDelete     = 111
	keyLeftMeta   = 125
)

// stroke is how a character is typed: a key, possibly with Shift and/or AltGr.
type stroke struct {
	code  int
	shift bool
	altgr bool
}

// Layout maps characters to key strokes for one XKB keyboard layout. It must match the layout
// of the compositor (the console session), otherwise other characters come out.
type Layout map[rune]stroke

var letterRows = []string{"qwertyuiop", "asdfghjkl", "zxcvbnm"}
var letterRowStart = []int{keyQ, keyA, keyZ}

// common adds what most Latin layouts share: letters, digits, space, Enter, Tab.
func common() Layout {
	l := Layout{' ': {code: keySpace}, '\n': {code: keyEnter}, '\t': {code: keyTab}}
	for i, row := range letterRows {
		for j, c := range row {
			l[c] = stroke{code: letterRowStart[i] + j}
			l[c-'a'+'A'] = stroke{code: letterRowStart[i] + j, shift: true}
		}
	}
	for d := '1'; d <= '9'; d++ {
		l[d] = stroke{code: key1 + int(d-'1')}
	}
	l['0'] = stroke{code: key0}
	return l
}

func (l Layout) add(code int, base, shifted, altgr, shiftAltgr rune) {
	if base != 0 {
		l[base] = stroke{code: code}
	}
	if shifted != 0 {
		l[shifted] = stroke{code: code, shift: true}
	}
	if altgr != 0 {
		l[altgr] = stroke{code: code, altgr: true}
	}
	if shiftAltgr != 0 {
		l[shiftAltgr] = stroke{code: code, shift: true, altgr: true}
	}
}

// layoutUS is XKB "us".
func layoutUS() Layout {
	l := common()
	for i, s := range ")!@#$%^&*(" { // shifted digits 0..9
		code := key0
		if i > 0 {
			code = key1 + i - 1
		}
		l[s] = stroke{code: code, shift: true}
	}
	l.add(keyGrave, '`', '~', 0, 0)
	l.add(keyMinus, '-', '_', 0, 0)
	l.add(keyEqual, '=', '+', 0, 0)
	l.add(keyLeftBrace, '[', '{', 0, 0)
	l.add(keyRightBrace, ']', '}', 0, 0)
	l.add(keyBackslash, '\\', '|', 0, 0)
	l.add(keySemicolon, ';', ':', 0, 0)
	l.add(keyApostrophe, '\'', '"', 0, 0)
	l.add(keyComma, ',', '<', 0, 0)
	l.add(keyDot, '.', '>', 0, 0)
	l.add(keySlash, '/', '?', 0, 0)
	return l
}

// layoutIT is XKB "it" (Italian keyboard).
func layoutIT() Layout {
	l := common()
	for i, s := range "=!\"£$%&/()" { // shifted digits 0..9
		code := key0
		if i > 0 {
			code = key1 + i - 1
		}
		l[s] = stroke{code: code, shift: true}
	}
	l.add(keyGrave, '\\', '|', 0, 0)
	l.add(keyMinus, '\'', '?', '`', 0)
	l.add(keyEqual, 'ì', '^', '~', 0)
	l.add(keyLeftBrace, 'è', 'é', '[', '{')
	l.add(keyRightBrace, '+', '*', ']', '}')
	l.add(keySemicolon, 'ò', 'ç', '@', 0)
	l.add(keyApostrophe, 'à', '°', '#', 0)
	l.add(keyBackslash, 'ù', '§', 0, 0)
	l.add(key102nd, '<', '>', 0, 0)
	l.add(keyComma, ',', ';', 0, 0)
	l.add(keyDot, '.', ':', 0, 0)
	l.add(keySlash, '-', '_', 0, 0)
	l['€'] = stroke{code: keyE, altgr: true}
	return l
}

// Layouts are the supported keyboard layouts, by XKB name.
var Layouts = map[string]func() Layout{"us": layoutUS, "it": layoutIT, "de": layoutDE, "fr": layoutFR, "es": layoutES}

// namedKeys are the special keys accepted by PressKey.
var namedKeys = map[string]int{
	"enter": keyEnter, "backspace": keyBackspace, "tab": keyTab, "escape": keyEsc,
	"space": keySpace, "delete": keyDelete, "home": keyHome, "end": keyEnd,
	"up": keyUp, "down": keyDown, "left": keyLeft, "right": keyRight,
	"pageup": keyPageUp, "pagedown": keyPageDown,
}

var modifierKeys = map[string]int{
	"ctrl": keyLeftCtrl, "shift": keyLeftShift, "alt": keyLeftAlt, "meta": keyLeftMeta,
}

// shiftedDigits sets the characters of Shift and a digit key, from 0 to 9.
func (l Layout) shiftedDigits(chars string) {
	for i, s := range chars {
		code := key0
		if i > 0 {
			code = key1 + i - 1
		}
		l[s] = stroke{code: code, shift: true}
	}
}

// letter puts a letter (and its capital) on another key.
func (l Layout) letter(c rune, code int) {
	l[c] = stroke{code: code}
	l[c-'a'+'A'] = stroke{code: code, shift: true}
}

// layoutDE is XKB "de" (German, QWERTZ); its dead keys (^, ´, `) are left out.
func layoutDE() Layout {
	l := common()
	l.letter('z', keyY)
	l.letter('y', keyZ)
	l.shiftedDigits("=!\"§$%&/()")
	l.add(keyGrave, 0, '°', 0, 0)
	l.add(keyMinus, 'ß', '?', '\\', 0)
	l.add(keyLeftBrace, 'ü', 'Ü', 0, 0)
	l.add(keyRightBrace, '+', '*', '~', 0)
	l.add(keySemicolon, 'ö', 'Ö', 0, 0)
	l.add(keyApostrophe, 'ä', 'Ä', 0, 0)
	l.add(keyBackslash, '#', '\'', 0, 0)
	l.add(key102nd, '<', '>', '|', 0)
	l.add(keyComma, ',', ';', 0, 0)
	l.add(keyDot, '.', ':', 0, 0)
	l.add(keySlash, '-', '_', 0, 0)
	for i, c := range "²³" {
		l[c] = stroke{code: key1 + 1 + i, altgr: true}
	}
	for i, c := range "{[]}" { // AltGr 7, 8, 9, 0
		code := key1 + 6 + i
		if i == 3 {
			code = key0
		}
		l[c] = stroke{code: code, altgr: true}
	}
	l['@'] = stroke{code: keyQ, altgr: true}
	l['€'] = stroke{code: keyE, altgr: true}
	return l
}

// layoutFR is XKB "fr" (French, AZERTY): the digits with Shift; its dead keys (^, ¨) are left out.
func layoutFR() Layout {
	l := common()
	l.letter('a', keyQ)
	l.letter('z', keyW)
	l.letter('q', keyA)
	l.letter('w', keyZ)
	l.letter('m', keySemicolon)
	for d := '1'; d <= '9'; d++ {
		l[d] = stroke{code: key1 + int(d-'1'), shift: true}
	}
	l['0'] = stroke{code: key0, shift: true}
	for i, c := range "&é\"'(-è_çà" { // the digit keys without Shift, from 1 to 0
		code := key1 + i
		if i == 9 {
			code = key0
		}
		l[c] = stroke{code: code}
	}
	for i, c := range "#{[|" { // AltGr 3, 4, 5, 6
		l[c] = stroke{code: key1 + 2 + i, altgr: true}
	}
	l['\\'] = stroke{code: key1 + 7, altgr: true}
	l['^'] = stroke{code: key1 + 8, altgr: true}
	l['@'] = stroke{code: key0, altgr: true}
	l.add(keyGrave, '²', 0, 0, 0)
	l.add(keyMinus, ')', '°', ']', 0)
	l.add(keyEqual, '=', '+', '}', 0)
	l.add(keyRightBrace, '$', '£', '¤', 0)
	l.add(keyApostrophe, 'ù', '%', 0, 0)
	l.add(keyBackslash, '*', 'µ', 0, 0)
	l.add(key102nd, '<', '>', 0, 0)
	l.add(keyM, ',', '?', 0, 0)
	l.add(keyComma, ';', '.', 0, 0)
	l.add(keyDot, ':', '/', 0, 0)
	l.add(keySlash, '!', '§', 0, 0)
	l['€'] = stroke{code: keyE, altgr: true}
	return l
}

// layoutES is XKB "es" (Spanish); its dead keys (`, ^, ´, ¨) are left out.
func layoutES() Layout {
	l := common()
	l.shiftedDigits("=!\"·$%&/()")
	l.add(keyGrave, 'º', 'ª', '\\', 0)
	l.add(keyMinus, '\'', '?', 0, 0)
	l.add(keyEqual, '¡', '¿', 0, 0)
	l.add(keyLeftBrace, 0, 0, '[', 0)
	l.add(keyRightBrace, '+', '*', ']', 0)
	l.add(keySemicolon, 'ñ', 'Ñ', 0, 0)
	l.add(keyApostrophe, 0, 0, '{', 0)
	l.add(keyBackslash, 'ç', 'Ç', '}', 0)
	l.add(key102nd, '<', '>', 0, 0)
	l.add(keyComma, ',', ';', 0, 0)
	l.add(keyDot, '.', ':', 0, 0)
	l.add(keySlash, '-', '_', 0, 0)
	l['|'] = stroke{code: key1, altgr: true}
	l['@'] = stroke{code: key1 + 1, altgr: true}
	l['#'] = stroke{code: key1 + 2, altgr: true}
	l['~'] = stroke{code: key1 + 3, altgr: true}
	l['¬'] = stroke{code: key1 + 5, altgr: true}
	l['€'] = stroke{code: keyE, altgr: true}
	return l
}
