package processor

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

// ZipFiles escreve os arquivos (pelo nome base) em um ZIP no writer.
func ZipFiles(files []string, w io.Writer) error {
	zw := zip.NewWriter(w)
	for _, path := range files {
		if err := addFile(zw, path); err != nil {
			_ = zw.Close()
			return err
		}
	}
	return zw.Close()
}

// ZipFilesToPath cria o arquivo ZIP no caminho indicado.
func ZipFilesToPath(files []string, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	if err := ZipFiles(files, f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
