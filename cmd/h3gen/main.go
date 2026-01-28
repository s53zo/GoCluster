//go:build !windows && cgo

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/uber/h3-go/v4"
)

const (
	res1Count = 842
	res2Count = 5882
)

func main() {
	outDir := flag.String("out", "data/config/h3", "output directory for H3 tables")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal("create output dir", err)
	}

	if err := writeTable(*outDir, 1, res1Count); err != nil {
		fatal("write res-1", err)
	}
	if err := writeTable(*outDir, 2, res2Count); err != nil {
		fatal("write res-2", err)
	}
}

func writeTable(outDir string, res int, wantCount int) error {
	cells, err := allCellsAtResolution(res)
	if err != nil {
		return err
	}
	if len(cells) != wantCount {
		return fmt.Errorf("unexpected res-%d cell count %d (want %d)", res, len(cells), wantCount)
	}
	name := filepath.Join(outDir, fmt.Sprintf("res%d.bin", res))
	tmp, err := os.CreateTemp(outDir, fmt.Sprintf("res%d-*.bin", res))
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	buf := make([]byte, 8)
	for _, cell := range cells {
		binary.LittleEndian.PutUint64(buf, uint64(cell))
		if _, err := tmp.Write(buf); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("write temp file: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
}

func allCellsAtResolution(res int) ([]h3.Cell, error) {
	if res < 0 || res > 15 {
		return nil, fmt.Errorf("invalid resolution %d", res)
	}
	res0, err := h3.Res0Cells()
	if err != nil {
		return nil, fmt.Errorf("res0 cells: %w", err)
	}
	var all []h3.Cell
	for _, base := range res0 {
		children, err := base.Children(res)
		if err != nil {
			return nil, fmt.Errorf("children for base cell %d: %w", base, err)
		}
		all = append(all, children...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	return all, nil
}

func fatal(action string, err error) {
	fmt.Fprintf(os.Stderr, "h3gen: %s: %v\n", action, err)
	os.Exit(1)
}
