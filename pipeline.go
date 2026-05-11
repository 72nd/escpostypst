package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"

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

func runPipeline(ctx context.Context, typPath string, copies int, cutSinglePage bool, imgCfg *escposimg.Config, outputMethod, networkAddr, filePath string) error {
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
	sort.Strings(pages)

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
