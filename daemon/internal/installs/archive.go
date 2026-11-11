// SPDX-License-Identifier: GPL-3.0-or-later

package installs

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// GOG's Linux installer is a shell script (makeself) followed by a ZIP archive (MojoSetup): Go's
// reader finds the archive after the script. The game is data/noarch, exactly what MojoSetup
// copies when it runs (owner, 2026-10-19: extracted, never run).
const gameDir = "data/noarch/"

type archive struct {
	*zip.Reader
	fh    *os.File    // the installer, open while the archive is read
	files []*zip.File // the game's files and folders (under data/noarch)
	total int64       // their size
}

func (a *archive) Close() error { return a.fh.Close() }

func openArchive(path string) (*archive, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	size := st.Size()
	zr, err := zip.NewReader(f, size)
	if err != nil {
		// a big installer: its ZIP64 records do not count the script, which Go's reader cannot
		// follow; read the archive from where it really starts
		if base, ok := zipStart(f, size); ok {
			zr, err = zip.NewReader(io.NewSectionReader(f, base, size-base), size-base)
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	a := &archive{Reader: zr, fh: f}
	for _, zf := range zr.File {
		if strings.HasPrefix(zf.Name, gameDir) && len(zf.Name) > len(gameDir) {
			a.files = append(a.files, zf)
			if !zf.FileInfo().IsDir() {
				a.total += int64(zf.UncompressedSize64)
			}
		}
	}
	if len(a.files) == 0 {
		f.Close()
		return nil, errors.New("no game in the archive (data/noarch)")
	}
	return a, nil
}

// zipStart finds where a ZIP64 archive starts in a file that has something before it (GOG's
// script): the ZIP64 end record lies right before its locator at the end of the file, and the
// locator tells where it is from the archive's start.
func zipStart(r io.ReaderAt, size int64) (int64, bool) {
	const (
		endSig     = 0x06054b50 // end of central directory
		locatorSig = 0x07064b50 // ZIP64 end locator (20 bytes, right before the end record)
		end64Sig   = 0x06064b50 // ZIP64 end record (56 bytes and more, right before the locator)
	)
	n := int64(22 + 65535 + 20 + 56 + 1024)
	if n > size {
		n = size
	}
	buf := make([]byte, n)
	if _, err := r.ReadAt(buf, size-n); err != nil && err != io.EOF {
		return 0, false
	}
	le32 := func(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }
	// the end record: the last signature whose comment reaches the end of the file
	end := -1
	for i := len(buf) - 22; i >= 0; i-- {
		if le32(buf[i:]) == endSig && i+22+int(binary.LittleEndian.Uint16(buf[i+20:])) == len(buf) {
			end = i
			break
		}
	}
	loc := end - 20
	if end < 0 || loc < 0 || le32(buf[loc:]) != locatorSig {
		return 0, false
	}
	rel := int64(binary.LittleEndian.Uint64(buf[loc+8:]))
	for i := loc - 56; i >= 0; i-- {
		if le32(buf[i:]) == end64Sig && int64(i)+12+int64(binary.LittleEndian.Uint64(buf[i+4:])) == int64(loc) {
			base := size - n + int64(i) - rel
			return base, base > 0 && base < size
		}
	}
	return 0, false
}

// licenceName: a licence's text file (EULA, licence, terms).
var licenceName = regexp.MustCompile(`(?i)(eula|licen[cs]e|terms)[^/]*\.txt$`)

// maxLicence: a licence's text is shown up to this size.
const maxLicence = 512 << 10

// licence is the game's licence text in the archive, "" when there is none: first in the game's
// docs folder, then anywhere in the game, then elsewhere in the archive (the installer's own).
func (a *archive) licence() string {
	var docs, game, other *zip.File
	for _, f := range a.File {
		if f.FileInfo().IsDir() || !licenceName.MatchString(f.Name) {
			continue
		}
		switch {
		case strings.HasPrefix(f.Name, gameDir+"docs/") && docs == nil:
			docs = f
		case strings.HasPrefix(f.Name, gameDir) && game == nil:
			game = f
		case other == nil:
			other = f
		}
	}
	for _, f := range []*zip.File{docs, game, other} {
		if f == nil {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(r, maxLicence))
		r.Close()
		if err == nil && len(data) > 0 {
			return text(data)
		}
	}
	return ""
}

// text reads UTF-8, else Latin-1 (older licences); Windows line ends become \n.
func text(data []byte) string {
	s := string(data)
	if !utf8.ValidString(s) {
		r := make([]rune, len(data))
		for i, b := range data {
			r[i] = rune(b)
		}
		s = string(r)
	}
	s = strings.TrimPrefix(s, "\ufeff")
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// safeJoin is dest/rel, or false if rel leaves dest.
func safeJoin(dest, rel string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.Join(dest, clean), true
}

// extract writes the game's files into dest, with their permissions; progress gets the bytes
// written so far.
func (a *archive) extract(ctx context.Context, dest string, progress func(done int64)) error {
	var done int64
	for _, f := range a.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		target, ok := safeJoin(dest, strings.TrimPrefix(f.Name, gameDir))
		if !ok {
			continue // never outside the game's folder
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			if err := a.symlink(f, dest, target); err != nil {
				return err
			}
		default:
			n, err := a.file(ctx, f, target, mode.Perm()|0o600, func(n int64) { progress(done + n) })
			done += n
			if err != nil {
				return err
			}
			progress(done)
		}
	}
	return nil
}

// symlink makes a link of the archive, only if it points inside the game's folder.
func (a *archive) symlink(f *zip.File, dest, target string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	link, err := io.ReadAll(io.LimitReader(r, 4096))
	r.Close()
	if err != nil {
		return err
	}
	to := string(link)
	if filepath.IsAbs(to) {
		return nil
	}
	if resolved := filepath.Clean(filepath.Join(filepath.Dir(target), to)); !within(resolved, dest) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	os.Remove(target)
	return os.Symlink(to, target)
}

// file writes one file, telling the bytes written as it goes.
func (a *archive) file(ctx context.Context, f *zip.File, target string, perm os.FileMode, moved func(n int64)) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return 0, err
	}
	r, err := f.Open()
	if err != nil {
		return 0, err
	}
	defer r.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return 0, err
	}
	var n int64
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return n, err
		}
		k, rerr := r.Read(buf)
		if k > 0 {
			if _, werr := out.Write(buf[:k]); werr != nil {
				out.Close()
				return n, werr
			}
			n += int64(k)
			moved(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return n, rerr
		}
	}
	if err := out.Close(); err != nil {
		return n, err
	}
	return n, os.Chmod(target, perm) // the umask may have taken bits away
}
