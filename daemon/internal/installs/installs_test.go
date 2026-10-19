// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fancy-dino/OwneetOS/daemon/internal/events"
)

// gogScript writes an installer shaped like GOG's Linux ones: a shell script followed by a ZIP
// archive whose offsets do not count the script (as makeself + MojoSetup make them).
func gogScript(t *testing.T, path string, big int) {
	t.Helper()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	add := func(name string, mode os.FileMode, body string) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, body)
	}
	add("scripts/config.lua", 0o644, "-- MojoSetup")
	add("data/noarch/", os.ModeDir|0o755, "")
	add("data/noarch/start.sh", 0o755, "#!/bin/bash\necho game\n")
	add("data/noarch/game/data.bin", 0o644, strings.Repeat("x", big))
	add("data/noarch/docs/End User License Agreement.txt", 0o644, "LICENCE\r\nYou may play.\r\n")
	add("data/noarch/game/current", os.ModeSymlink|0o777, "data.bin")
	add("data/noarch/game/escape", os.ModeSymlink|0o777, "../../../../etc/passwd")
	add("data/noarch/../../evil.txt", 0o644, "never")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\n# This script was generated using Makeself 2.4.0\n" + strings.Repeat("#", 4000) + "\nexit 0\n"
	if err := os.WriteFile(path, append([]byte(stub), zbuf.Bytes()...), 0o755); err != nil {
		t.Fatal(err)
	}
}

type fakeRunner struct {
	mu    sync.Mutex
	argv  [][]string
	units []string
	run   func(ctx context.Context, argv []string) error
}

func (f *fakeRunner) Run(ctx context.Context, unit string, argv, env []string) error {
	f.mu.Lock()
	f.argv = append(f.argv, argv)
	f.units = append(f.units, unit)
	run := f.run
	f.mu.Unlock()
	if run != nil {
		return run(ctx, argv)
	}
	return nil
}

func newManager(t *testing.T) (*Manager, string, string, <-chan events.Event) {
	t.Helper()
	home := t.TempDir()
	disk := filepath.Join(t.TempDir(), "SSD") // stands for /run/media/owneet/SSD
	broker := events.NewBroker()
	ch, unsub := broker.Subscribe(64)
	t.Cleanup(unsub)
	m := &Manager{Broker: broker, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Home: home,
		StateDir: filepath.Join(home, "state"), WindowsRun: "/usr/lib/owneet/owneet-windows-run",
		Runner: &fakeRunner{}, Free: func(string) (int64, error) { return 1 << 40, nil }}
	os.MkdirAll(filepath.Join(home, "Giochi"), 0o755)
	return m, home, disk, ch
}

func next(t *testing.T, ch <-chan events.Event, typ string) events.Event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Type == typ {
				return e
			}
		case <-timeout:
			t.Fatalf("no %s event", typ)
		}
	}
}

func run(t *testing.T, m *Manager) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go m.Run(ctx)
}

func TestGOGScriptExtracted(t *testing.T) {
	m, home, _, ch := newManager(t)
	src := filepath.Join(home, "Giochi", "Stargunner (installer)")
	os.MkdirAll(src, 0o755)
	installer := filepath.Join(src, "gog_stargunner_2.0.0.10.sh")
	gogScript(t, installer, 3<<20)
	info, err := m.Inspect(installer, KindSh)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(len("#!/bin/bash\necho game\n") + 3<<20 + len("LICENCE\r\nYou may play.\r\n") + len("data.bin") + len("../../../../etc/passwd") + len("never"))
	if !info.Exact || info.Need != want || info.Licence != "LICENCE\nYou may play.\n" || info.Free != 1<<40 {
		t.Fatalf("info %+v (need %d)", info, want)
	}
	run(t, m)
	dest := filepath.Join(home, "Giochi", "Stargunner")
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: dest, Title: "Stargunner"}); err != nil {
		t.Fatal(err)
	}
	if d := next(t, ch, "install.done").Data.(map[string]string); d["dest"] != dest || d["title"] != "Stargunner" {
		t.Fatalf("install.done %v", d)
	}
	st, err := os.Stat(filepath.Join(dest, "start.sh"))
	if err != nil || st.Mode().Perm() != 0o755 {
		t.Fatalf("start.sh %v %v", st, err)
	}
	if link, err := os.Readlink(filepath.Join(dest, "game", "current")); err != nil || link != "data.bin" {
		t.Fatalf("link %q %v", link, err)
	}
	for _, never := range []string{filepath.Join(dest, "game", "escape"), filepath.Join(home, "evil.txt"),
		filepath.Join(home, "Giochi", "evil.txt"), filepath.Join(dest, "scripts"), filepath.Join(dest, MarkInstalling)} {
		if exists(never) {
			t.Fatalf("%s written", never)
		}
	}
	if !exists(filepath.Join(dest, MarkInstalled)) {
		t.Fatal("not marked installed")
	}
	if got := m.Status(); len(got.Installs) != 0 {
		t.Fatalf("status %+v", got)
	}
	// installed already: not again
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: dest}); err != ErrExists {
		t.Fatalf("again: %v", err)
	}
	if err := m.Uninstall(dest); err != nil || exists(dest) {
		t.Fatalf("uninstall: %v, still there %v", err, exists(dest))
	}
	if !exists(installer) {
		t.Fatal("the installer was removed")
	}
	if err := m.Uninstall(src); err != ErrNotFound {
		t.Fatalf("uninstall of a folder not installed: %v", err)
	}
}

func TestPathsChecked(t *testing.T) {
	m, home, _, _ := newManager(t)
	src := filepath.Join(home, "Giochi", "A")
	os.MkdirAll(src, 0o755)
	installer := filepath.Join(src, "a.sh")
	gogScript(t, installer, 10)
	outside := filepath.Join(home, "a.sh")
	gogScript(t, outside, 10)
	for _, req := range []Request{
		// the installer not in a games folder
		{Installer: outside, Kind: KindSh, Dest: filepath.Join(home, "Giochi", "B")},
		// not a games folder, hidden, too deep, not absolute
		{Installer: installer, Kind: KindSh, Dest: filepath.Join(home, "Documents", "B")},
		{Installer: installer, Kind: KindSh, Dest: filepath.Join(home, "Giochi", ".B")},
		{Installer: installer, Kind: KindSh, Dest: filepath.Join(home, "Giochi", "x", "B")},
		{Installer: installer, Kind: KindSh, Dest: "Giochi/B"},
		// an unknown kind; a prefix outside the console's prefixes
		{Installer: installer, Kind: "exe", Dest: filepath.Join(home, "Giochi", "B")},
		{Installer: installer, Kind: KindInno, Dest: filepath.Join(home, "Giochi", "B"), Prefix: "/tmp/p"},
	} {
		if err := m.Install(req); err != ErrInvalid {
			t.Errorf("%+v: %v", req, err)
		}
	}
	m.Free = func(string) (int64, error) { return 5, nil }
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: filepath.Join(home, "Games", "B")}); err != ErrNoSpace {
		t.Fatalf("no space: %v", err)
	}
	// an existing folder of the same name, not the installer's: never overwritten
	os.MkdirAll(filepath.Join(home, "Giochi", "C"), 0o755)
	m.Free = func(string) (int64, error) { return 1 << 40, nil }
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: filepath.Join(home, "Giochi", "C")}); err != ErrExists {
		t.Fatalf("existing folder: %v", err)
	}
}

// An installer in the game's own folder on the console's disk: installed next to it; uninstalling
// keeps it.
func TestInstallerInTheGamesFolder(t *testing.T) {
	m, home, _, ch := newManager(t)
	dest := filepath.Join(home, "Giochi", "Tyrian")
	os.MkdirAll(dest, 0o755)
	installer := filepath.Join(dest, "tyrian.sh")
	gogScript(t, installer, 100)
	os.WriteFile(filepath.Join(dest, "cover.jpg"), []byte("jpg"), 0o644)
	run(t, m)
	if err := m.Install(Request{Installer: installer, Kind: KindSh, Dest: dest}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "install.done")
	if !exists(filepath.Join(dest, "start.sh")) {
		t.Fatal("not installed")
	}
	if err := m.Uninstall(dest); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"tyrian.sh", "cover.jpg"} {
		if !exists(filepath.Join(dest, f)) {
			t.Fatalf("%s removed", f)
		}
	}
	if exists(filepath.Join(dest, "start.sh")) || exists(filepath.Join(dest, MarkInstalled)) {
		t.Fatal("game left")
	}
}

func TestWindowsInstallers(t *testing.T) {
	m, home, _, ch := newManager(t)
	src := filepath.Join(home, "Giochi", "Shadow Harbor setup")
	os.MkdirAll(src, 0o755)
	setup := filepath.Join(src, "setup_shadow_harbor_1.4.2.exe")
	os.WriteFile(setup, make([]byte, 1000), 0o644)
	os.WriteFile(filepath.Join(src, "setup_shadow_harbor_1.4.2-1.bin"), make([]byte, 4000), 0o644)
	os.WriteFile(filepath.Join(src, "EULA.txt"), []byte("Windows licence"), 0o644)
	info, err := m.Inspect(setup, KindInno)
	if err != nil || info.Exact || info.Need != 10000 || info.Licence != "Windows licence" {
		t.Fatalf("info %+v %v", info, err)
	}
	runner := m.Runner.(*fakeRunner)
	// the installer writes into the folder it is given (Z: is the whole file system)
	runner.run = func(ctx context.Context, argv []string) error {
		last := argv[len(argv)-1]
		_, win, _ := strings.Cut(last, "=")
		dir := strings.ReplaceAll(strings.TrimPrefix(win, "Z:"), `\`, "/")
		return os.WriteFile(filepath.Join(dir, "game.exe"), []byte("MZ"), 0o644)
	}
	m.Env = func() []string { return []string{"DISPLAY=:1"} }
	run(t, m)
	prefixes := filepath.Join(home, ".local", "share", "owneet", "windows", "prefixes")
	dest := filepath.Join(home, "Giochi", "Shadow Harbor")
	if err := m.Install(Request{Installer: setup, Kind: KindInno, Dest: dest, Prefix: filepath.Join(prefixes, "shadow-harbor-1234")}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "install.done")
	argv := runner.argv[0]
	want := []string{"/usr/lib/owneet/owneet-windows-run", filepath.Join(prefixes, "shadow-harbor-1234"), setup,
		"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/NOICONS", `/DIR=Z:` + strings.ReplaceAll(dest, "/", `\`)}
	if strings.Join(argv, "|") != strings.Join(want, "|") {
		t.Fatalf("argv %q", argv)
	}
	if !strings.HasPrefix(runner.units[0], "owneet-install-") || !exists(filepath.Join(dest, "game.exe")) {
		t.Fatalf("unit %q, installed %v", runner.units[0], exists(filepath.Join(dest, "game.exe")))
	}
	// NSIS: through a link without spaces, kept for the game; removed with it
	dest2 := filepath.Join(home, "Giochi", "Lantern Keep")
	if err := m.Install(Request{Installer: setup, Kind: KindNSIS, Dest: dest2, Prefix: filepath.Join(prefixes, "lantern-keep-1")}); err != nil {
		t.Fatal(err)
	}
	next(t, ch, "install.done")
	argv = runner.argv[1]
	if argv[3] != "/S" || !strings.HasPrefix(argv[4], `/D=Z:`) || strings.Contains(argv[4], " ") {
		t.Fatalf("NSIS argv %q", argv)
	}
	link := strings.ReplaceAll(strings.TrimPrefix(argv[4], "/D=Z:"), `\`, "/")
	if to, err := os.Readlink(link); err != nil || to != dest2 || !exists(filepath.Join(dest2, "game.exe")) {
		t.Fatalf("link %q → %q %v", link, to, err)
	}
	if err := m.Uninstall(dest2); err != nil || exists(link) || exists(dest2) {
		t.Fatalf("uninstall: %v, link %v, folder %v", err, exists(link), exists(dest2))
	}
	// Windows games not ready (owneet-windows-run's exit code 3): nothing left
	runner.run = func(ctx context.Context, argv []string) error { return exec.Command("sh", "-c", "exit 3").Run() }
	dest3 := filepath.Join(home, "Giochi", "Third")
	m.Install(Request{Installer: setup, Kind: KindMSI, Dest: dest3, Prefix: filepath.Join(prefixes, "third-1")})
	if d := next(t, ch, "install.failed").Data.(map[string]string); d["reason"] != ReasonWindows || exists(dest3) {
		t.Fatalf("not ready: %v, folder %v", d, exists(dest3))
	}
	// an installer that ends without writing anything failed
	runner.run = nil
	m.Install(Request{Installer: setup, Kind: KindInno, Dest: dest3, Prefix: filepath.Join(prefixes, "third-1")})
	if d := next(t, ch, "install.failed").Data.(map[string]string); d["reason"] != ReasonFailed || exists(dest3) {
		t.Fatalf("nothing written: %v", d)
	}
}

// Stopping: the one in progress (what it wrote is removed), one waiting (taken out of the queue);
// the disk unplugged in the middle
func TestStopAndSourceGone(t *testing.T) {
	m, home, _, ch := newManager(t)
	src := filepath.Join(home, "Giochi", "src")
	os.MkdirAll(src, 0o755)
	setup := filepath.Join(src, "setup.exe")
	os.WriteFile(setup, []byte("MZ"), 0o644)
	started := make(chan struct{}, 4)
	m.Runner.(*fakeRunner).run = func(ctx context.Context, argv []string) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	run(t, m)
	prefix := filepath.Join(home, ".local", "share", "owneet", "windows", "prefixes", "p")
	a, b := filepath.Join(home, "Giochi", "A"), filepath.Join(home, "Giochi", "B")
	m.Install(Request{Installer: setup, Kind: KindInno, Dest: a, Prefix: prefix})
	m.Install(Request{Installer: setup, Kind: KindInno, Dest: b, Prefix: prefix})
	<-started
	if st := m.Status(); len(st.Installs) != 2 || st.Installs[0].State != StateInstalling || st.Installs[1].State != StateQueued {
		t.Fatalf("status %+v", st)
	}
	if err := m.Stop(b); err != nil {
		t.Fatal(err)
	}
	if d := next(t, ch, "install.failed").Data.(map[string]string); d["dest"] != b || d["reason"] != ReasonCancelled {
		t.Fatalf("queued stopped %v", d)
	}
	if err := m.Stop(a); err != nil {
		t.Fatal(err)
	}
	if d := next(t, ch, "install.failed").Data.(map[string]string); d["dest"] != a || d["reason"] != ReasonCancelled || exists(a) {
		t.Fatalf("stopped %v, folder %v", d, exists(a))
	}
	if err := m.Stop(a); err != ErrNotFound {
		t.Fatalf("stop again: %v", err)
	}
	// the installer's disk goes away during the install
	m.Runner.(*fakeRunner).run = func(ctx context.Context, argv []string) error {
		os.Remove(setup)
		return exec.Command("false").Run()
	}
	m.Install(Request{Installer: setup, Kind: KindInno, Dest: a, Prefix: prefix})
	if d := next(t, ch, "install.failed").Data.(map[string]string); d["reason"] != ReasonSourceGone {
		t.Fatalf("source gone %v", d)
	}
}

// A restart in the middle of an install: its folder is removed when owneetd starts again.
func TestCutShortCleaned(t *testing.T) {
	m, home, _, _ := newManager(t)
	dest := filepath.Join(home, "Giochi", "Half")
	os.MkdirAll(dest, 0o755)
	os.WriteFile(filepath.Join(dest, MarkInstalling), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(dest, "part"), []byte("x"), 0o644)
	done := filepath.Join(home, "Giochi", "Done")
	os.MkdirAll(done, 0o755)
	os.WriteFile(filepath.Join(done, MarkInstalled), []byte(`{}`), 0o644)
	os.MkdirAll(m.StateDir, 0o755)
	os.WriteFile(filepath.Join(m.StateDir, "installs.json"), []byte(`["`+dest+`","`+done+`","/etc"]`), 0o644)
	m.init()
	m.cleanCutShort()
	if exists(dest) || !exists(done) || exists(filepath.Join(m.StateDir, "installs.json")) {
		t.Fatalf("half %v, done %v", exists(dest), exists(done))
	}
}

// A real GOG installer, when given (OWNEET_GOG_SH=path): what the console reads and extracts
// from it (checked with Beneath a Steel Sky's on 2026-10-19).
func TestRealGOGScript(t *testing.T) {
	path := os.Getenv("OWNEET_GOG_SH")
	if path == "" {
		t.Skip("no real GOG installer given (OWNEET_GOG_SH)")
	}
	a, err := openArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	lic := a.licence()
	t.Logf("%d files, %d bytes; licence: %d characters, starts %q", len(a.files), a.total, len(lic), firstLine(lic))
	if a.total == 0 || lic == "" {
		t.Fatal("nothing to install, or no licence")
	}
	dest := t.TempDir()
	if err := a.extract(context.Background(), dest, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dest, "start.sh"))
	if err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatalf("start.sh %v %v", st, err)
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
