package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/72nd/escposimg"
)

func rasterizePDF(ctx context.Context, pdfPath, pbmOutPattern string, dpi int) error {
	gsBin, err := exec.LookPath("gs")
	if err != nil {
		return fmt.Errorf("ghostscript (gs) not found in PATH: %w", err)
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, gsBin,
		"-dNOPAUSE",
		"-dBATCH",
		"-sDEVICE=pbmraw",
		"-r"+strconv.Itoa(dpi),
		"-dDITHERPPI="+strconv.Itoa(dpi),
		"-sOutputFile="+pbmOutPattern,
		pdfPath,
	)
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := stderr.String()
		if msg != "" {
			return fmt.Errorf("ghostscript: %w: %s", runErr, msg)
		}
		return fmt.Errorf("ghostscript: %w", runErr)
	}
	return nil
}

// pageNumberFromPBMPath returns the Ghostscript page index encoded in names like "page-00001.pbm".
// Sorting by this integer preserves PDF page order regardless of zero-padding width.
func pageNumberFromPBMPath(path string) (n int, ok bool) {
	base := filepath.Base(path)
	const prefix = "page-"
	const suffix = ".pbm"
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return 0, false
	}
	mid := base[len(prefix) : len(base)-len(suffix)]
	if mid == "" {
		return 0, false
	}
	v, err := strconv.Atoi(mid)
	if err != nil {
		return 0, false
	}
	return v, true
}

func sortPBMPathsByPageNumber(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		ni, okI := pageNumberFromPBMPath(paths[i])
		nj, okJ := pageNumberFromPBMPath(paths[j])
		switch {
		case okI && okJ && ni != nj:
			return ni < nj
		case okI != okJ:
			return okI // known filenames before unexpected matches
		default:
			return paths[i] < paths[j]
		}
	})
}

func runPipeline(ctx context.Context, typPath string, copies int, cutSinglePage bool, reversePages bool, imgCfg *escposimg.Config, outputMethod, networkAddr, filePath string) error {
	if copies < 1 {
		return fmt.Errorf("copies must be at least 1")
	}

	absTyp, err := filepath.Abs(typPath)
	if err != nil {
		return fmt.Errorf("resolve typ path: %w", err)
	}
	baseDir := filepath.Dir(absTyp)

	workDir, err := os.MkdirTemp(baseDir, ".escpostypst-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(workDir); rmErr != nil {
			slog.Warn("failed to remove work dir", "path", workDir, "error", rmErr)
		}
	}()

	pdfPath := filepath.Join(workDir, "document.pdf")
	slog.Debug("compiling Typst", "root", baseDir, "input", absTyp, "pdf", pdfPath)
	if err := compileTypst(ctx, baseDir, absTyp, pdfPath); err != nil {
		return err
	}

	pbmDir := filepath.Join(workDir, "pages")
	if err := os.MkdirAll(pbmDir, 0o755); err != nil {
		return fmt.Errorf("create pages dir: %w", err)
	}

	outPattern := filepath.Join(pbmDir, "page-%05d.pbm")
	slog.Debug("rasterizing PDF", "dpi", imgCfg.DPI, "pattern", outPattern)
	if err := rasterizePDF(ctx, pdfPath, outPattern, imgCfg.DPI); err != nil {
		return err
	}

	pages, err := filepath.Glob(filepath.Join(pbmDir, "page-*.pbm"))
	if err != nil {
		return fmt.Errorf("list PBM pages: %w", err)
	}
	if len(pages) == 0 {
		return fmt.Errorf("ghostscript produced no PBM pages (check PDF and gs install)")
	}
	sortPBMPathsByPageNumber(pages)
	if reversePages {
		slices.Reverse(pages)
	}

	cutAfterEachPage := len(pages) > 1 || cutSinglePage

	for copyIdx := 0; copyIdx < copies; copyIdx++ {
		slog.Debug("printing copy", "copy", copyIdx+1, "of", copies, "pages", len(pages))
		for _, pagePath := range pages {
			cfg := *imgCfg
			cfg.CutPaper = cutAfterEachPage

			output, outErr := createOutputMethod(outputMethod, networkAddr, filePath)
			if outErr != nil {
				return outErr
			}
			slog.Debug("sending page", "path", pagePath)
			if procErr := escposimg.ProcessImage(pagePath, &cfg, output); procErr != nil {
				return fmt.Errorf("process %s: %w", pagePath, procErr)
			}
		}
	}

	slog.Info("Typst document printed successfully", "pages", len(pages), "copies", copies)
	return nil
}
