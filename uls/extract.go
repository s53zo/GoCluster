package uls

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractArchive extracts only the FCC ULS table files we care about into a
// temporary directory and returns the directory path. Caller is responsible for
// cleaning it up.
func extractArchive(archivePath string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("fcc uls: open zip: %w", err)
	}
	defer r.Close()

	tmpDir, err := os.MkdirTemp(filepath.Dir(archivePath), "fcc-uls-extract-*")
	if err != nil {
		return "", fmt.Errorf("fcc uls: create temp dir: %w", err)
	}

	wanted := map[string]bool{
		"AM.DAT": true,
		"EN.DAT": true,
		"HD.DAT": true,
	}

	for _, f := range r.File {
		name := strings.ToUpper(filepath.Base(f.Name))
		if !wanted[name] {
			continue
		}
		if err := extractFile(f, filepath.Join(tmpDir, name)); err != nil {
			return "", err
		}
	}

	return tmpDir, nil
}

func extractFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("fcc uls: open %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("fcc uls: create %s: %w", dest, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("fcc uls: copy %s: %w", dest, err)
	}
	return nil
}
