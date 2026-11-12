package archive

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, name := range []string{"frame_00001.png", "frame_00002.png"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("png-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	dest := filepath.Join(dir, "out.zip")
	if err := (Zip{}).Archive(files, dest); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) != 2 || zr.File[0].Name != "frame_00001.png" {
		t.Fatalf("entries = %d first=%s", len(zr.File), zr.File[0].Name)
	}
	rc, _ := zr.File[1].Open()
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "png-frame_00002.png" {
		t.Fatalf("content = %q", b)
	}
}

func TestArchiveErrors(t *testing.T) {
	dir := t.TempDir()
	if err := Write([]string{filepath.Join(dir, "missing.png")}, io.Discard); err == nil {
		t.Fatal("missing file must fail")
	}
	if err := (Zip{}).Archive(nil, filepath.Join(dir, "no", "dir", "x.zip")); err == nil {
		t.Fatal("invalid destination must fail")
	}
	if err := (Zip{}).Archive([]string{filepath.Join(dir, "missing.png")}, filepath.Join(dir, "x.zip")); err == nil {
		t.Fatal("missing input must fail")
	}
}
