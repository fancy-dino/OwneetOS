// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A big GOG Linux installer (over 4 GB, or over 65 535 entries) has a ZIP64 archive; after the
// script, its ZIP64 records do not count the script either, and Go's reader cannot find them
// (FrankenPC, 2026-11-11: The Witcher 2's Linux installer, "the game could not be installed").
// More than 65 535 entries make the same records without a 4 GB file.
func TestZip64AfterTheScript(t *testing.T) {
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	for _, name := range []string{"scripts/config.lua", "data/noarch/start.sh"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("#!/bin/bash\n"))
	}
	for i := 0; i < 70000; i++ {
		if _, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("data/noarch/many/%05d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\n# This script was generated using Makeself 2.1.5\n" + strings.Repeat("#", 9000) + "\nexit 0\n"
	path := filepath.Join(t.TempDir(), "big.sh")
	if err := os.WriteFile(path, append([]byte(stub), zbuf.Bytes()...), 0o755); err != nil {
		t.Fatal(err)
	}
	a, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.files) != 70001 || a.total != int64(len("#!/bin/bash\n")) {
		t.Fatalf("%d files, %d bytes", len(a.files), a.total)
	}
	r, err := a.files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
}

// A file in parts (GOG's support/split_files) is joined after the install, as GOG's postinst.sh
// does: the whole file, in the parts' order, and no part left (The Witcher 2's pack0.dzip).
func TestSplitFilesJoined(t *testing.T) {
	m, home, _, ch := newManager(t)
	src := filepath.Join(home, "Giochi", "Big Game (installer)")
	os.MkdirAll(src, 0o755)
	installer := filepath.Join(src, "gog_big_game_1.0.sh")
	whole := make([]byte, 0, 3*70000)
	parts := map[string][]byte{}
	for i, b := range []byte{'a', 'b', 'c'} {
		part := bytes.Repeat([]byte{b}, 70000-i*1000)
		parts[fmt.Sprintf("data/noarch/game/data/pack0.dzip.split%02d", i)] = part
		whole = append(whole, part...)
	}
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	add := func(name string, body []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
	}
	add("data/noarch/start.sh", []byte("#!/bin/bash\n"))
	add("data/noarch/support/split_files", []byte("game/data/pack0.dzip\r\n\n"))
	// the parts in the archive out of order: the join follows their names
	for _, name := range []string{"data/noarch/game/data/pack0.dzip.split02", "data/noarch/game/data/pack0.dzip.split00", "data/noarch/game/data/pack0.dzip.split01"} {
		add(name, parts[name])
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(installer, append([]byte("#!/bin/sh\n# Makeself\nexit 0\n"), zbuf.Bytes()...), 0o755)
	run(t, m)
	dest := filepath.Join(home, "Giochi", "Big Game")
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: dest, Title: "Big Game"}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "install.done")
	got, err := os.ReadFile(filepath.Join(dest, "game", "data", "pack0.dzip"))
	if err != nil || !bytes.Equal(got, whole) {
		t.Fatalf("joined file: %d bytes, want %d (%v)", len(got), len(whole), err)
	}
	if left, _ := filepath.Glob(filepath.Join(dest, "game", "data", "*.split*")); len(left) != 0 {
		t.Fatalf("parts left: %v", left)
	}
}

// A list naming a file outside the game is refused; a file already whole is left alone.
func TestSplitFilesChecked(t *testing.T) {
	dest := t.TempDir()
	os.MkdirAll(filepath.Join(dest, "support"), 0o755)
	os.WriteFile(filepath.Join(dest, "support", "split_files"), []byte("game/whole.bin\n"), 0o644)
	os.MkdirAll(filepath.Join(dest, "game"), 0o755)
	os.WriteFile(filepath.Join(dest, "game", "whole.bin"), []byte("whole"), 0o644)
	if err := joinSplitFiles(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "game", "whole.bin")); string(b) != "whole" {
		t.Fatalf("changed: %q", b)
	}
	os.WriteFile(filepath.Join(dest, "support", "split_files"), []byte("../outside.bin\n"), 0o644)
	if err := joinSplitFiles(context.Background(), dest); err == nil {
		t.Fatal("a file outside the game was accepted")
	}
}
