//go:build linux

package diskprobe

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const block = 4096

// Probe runs reads 4 KiB reads at random offsets with O_DIRECT, then
// syncs 8 KiB writes each followed by fdatasync, one at a time, in dir.
// The files stay for a second probe; Clean removes them.
func Probe(dir string, reads, syncs int) (*Result, error) {
	rp := filepath.Join(dir, ReadFile)
	if err := ensureReadFile(rp); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(rp, os.O_RDONLY|unix.O_DIRECT, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s with O_DIRECT: %w", rp, err)
	}
	defer f.Close()
	buf := aligned(block)
	rnd := rand.New(rand.NewPCG(1, 2))
	rd := make([]time.Duration, 0, reads)
	for range reads {
		off := int64(rnd.IntN(ReadFileBytes/block)) * block
		t := time.Now()
		if _, err := f.ReadAt(buf, off); err != nil {
			return nil, fmt.Errorf("reading %s: %w", rp, err)
		}
		rd = append(rd, time.Since(t))
	}

	sp := filepath.Join(dir, SyncFile)
	w, err := os.OpenFile(sp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	defer w.Close()
	page := make([]byte, 8192)
	sd := make([]time.Duration, 0, syncs)
	for i := range syncs {
		t := time.Now()
		if _, err := w.WriteAt(page, int64(i)*int64(len(page))); err != nil {
			return nil, err
		}
		if err := unix.Fdatasync(int(w.Fd())); err != nil {
			return nil, fmt.Errorf("fdatasync: %w", err)
		}
		sd = append(sd, time.Since(t))
	}
	return &Result{Read: Summarize(rd), Sync: Summarize(sd)}, nil
}

// Clean removes the probe's files from dir.
func Clean(dir string) error {
	for _, n := range []string{ReadFile, SyncFile} {
		if err := os.Remove(filepath.Join(dir, n)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ensureReadFile writes the read file once, flushed, so its blocks are on
// the device rather than in the page cache.
func ensureReadFile(p string) error {
	if st, err := os.Stat(p); err == nil && st.Size() == ReadFileBytes {
		return nil
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	chunk := make([]byte, 1<<20)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	for range ReadFileBytes / len(chunk) {
		if _, err := f.Write(chunk); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Drop the file's pages from the cache; O_DIRECT reads skip it anyway.
	if g, err := os.Open(p); err == nil {
		_ = unix.Fadvise(int(g.Fd()), 0, 0, unix.FADV_DONTNEED)
		g.Close()
	}
	return nil
}

// aligned is a buffer of n bytes on a block boundary, as O_DIRECT needs.
func aligned(n int) []byte {
	b := make([]byte, n+block)
	off := int(uintptr(unsafe.Pointer(&b[0])) & (block - 1))
	if off != 0 {
		off = block - off
	}
	return b[off : off+n]
}
