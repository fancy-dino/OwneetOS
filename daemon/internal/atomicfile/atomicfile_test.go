// SPDX-License-Identifier: GPL-3.0-or-later

package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "webapps.json")
	for _, content := range []string{`{"webapps":["netflix"]}`, `{"webapps":[]}`} {
		if err := Write(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("read %q (%v), want %q", got, err, content)
		}
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("the temporary file was left behind (%v)", err)
	}
}
