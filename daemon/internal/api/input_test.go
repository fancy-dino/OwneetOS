// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/fancy-dino/OwneetOS/daemon/internal/vinput"
)

// fakeInput records calls and checks text with the real layout tables.
type fakeInput struct {
	typed  []string
	keys   []string
	layout string
}

func (f *fakeInput) TypeText(text string) error {
	if _, err := vinput.Strokes(f.layout, text); err != nil {
		return err
	}
	f.typed = append(f.typed, text)
	return nil
}
func (f *fakeInput) PressKey(name string, mods []string) error {
	if name != "enter" {
		return vinput.ErrUnknownKey
	}
	f.keys = append(f.keys, strings.Join(append(mods, name), "+"))
	return nil
}
func (f *fakeInput) MovePointer(dx, dy, wheel int) error { return nil }
func (f *fakeInput) Click(button string) error           { return nil }
func (f *fakeInput) Layout() string                      { return f.layout }
func (f *fakeInput) SetLayout(name string) error {
	if name != "us" && name != "it" {
		return vinput.ErrUnknownKey
	}
	f.layout = name
	return nil
}

func post(t *testing.T, c *http.Client, path, body string) (int, string) {
	t.Helper()
	resp, err := c.Post("http://owneetd"+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var e ErrorBody
	json.NewDecoder(resp.Body).Decode(&e)
	return resp.StatusCode, e.Error.Code
}

func TestInputUnavailableWithoutUinput(t *testing.T) {
	_, c, _ := start(t)
	if code, errCode := post(t, c, "/v1/input/text", `{"text":"hi"}`); code != 503 || errCode != "input.unavailable" {
		t.Fatalf("got %d %s", code, errCode)
	}
}

func TestInputText(t *testing.T) {
	s, c, _ := start(t)
	f := &fakeInput{layout: "it"}
	s.Input = f
	if code, _ := post(t, c, "/v1/input/text", `{"text":"perché"}`); code != 204 {
		t.Fatalf("status %d", code)
	}
	if code, errCode := post(t, c, "/v1/input/text", `{"text":"PERCHÈ"}`); code != 422 || errCode != "input.unsupported_character" {
		t.Fatalf("got %d %s", code, errCode)
	}
	if code, errCode := post(t, c, "/v1/input/text", `{"txt":"x"}`); code != 400 || errCode != "bad_request" {
		t.Fatalf("got %d %s", code, errCode)
	}
	if len(f.typed) != 1 || f.typed[0] != "perché" {
		t.Fatalf("typed %v", f.typed)
	}
}

func TestInputKey(t *testing.T) {
	s, c, _ := start(t)
	f := &fakeInput{layout: "us"}
	s.Input = f
	if code, _ := post(t, c, "/v1/input/key", `{"key":"enter","modifiers":["ctrl"]}`); code != 204 {
		t.Fatalf("status %d", code)
	}
	if code, errCode := post(t, c, "/v1/input/key", `{"key":"warp"}`); code != 400 || errCode != "input.unknown_key" {
		t.Fatalf("got %d %s", code, errCode)
	}
	if len(f.keys) != 1 || f.keys[0] != "ctrl+enter" {
		t.Fatalf("keys %v", f.keys)
	}
}
