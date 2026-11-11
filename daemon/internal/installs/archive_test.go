// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"archive/zip"
	"bytes"
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
