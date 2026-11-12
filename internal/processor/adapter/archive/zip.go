// Package archive implementa a porta Archiver gerando arquivos ZIP.
package archive

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

// Zip compacta arquivos em ZIP (cada arquivo entra pelo nome, sem diretórios).
type Zip struct{}

// Archive cria o ZIP em destPath.
func (Zip) Archive(files []string, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	if err := Write(files, f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Write escreve o ZIP no writer.
func Write(files []string, w io.Writer) error {
	zw := zip.NewWriter(w)
	for _, path := range files {
		if err := addFile(zw, path); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

func addFile(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	h.Name = filepath.Base(path)
	h.Method = zip.Deflate
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
