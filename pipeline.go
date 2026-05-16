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

// parsePageSelection parses a page spec (1-based): single "3", range "2-4", open end "2-",
// comma lists "1,3,5" or "1-3,5". Empty spec means all pages 1..total.
// Returns sorted, deduplicated page numbers. Fails if any selected page is outside 1..total.
func parsePageSelection(spec string, total int) ([]int, error) {
	if total < 1 {
		return nil, fmt.Errorf("document has no pages")
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages, nil
	}

	selected := make(map[int]struct{})

	parts := strings.Split(spec, ",")
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			return nil, fmt.Errorf("invalid --pages: empty segment in %q", spec)
		}

		if strings.Contains(part, "-") {
			loStr, hiStr, ok := strings.Cut(part, "-")
			if !ok {
				return nil, fmt.Errorf("invalid --pages segment %q", part)
			}
			loStr = strings.TrimSpace(loStr)
			hiStr = strings.TrimSpace(hiStr)

			var start, end int
			if loStr == "" {
				return nil, fmt.Errorf("invalid --pages range %q (start page is required)", part)
			}
			lo, err := strconv.Atoi(loStr)
			if err != nil {
				return nil, fmt.Errorf("invalid --pages range %q: %w", part, err)
			}
			start = lo

			if hiStr == "" {
				end = total
			} else {
				hi, err := strconv.Atoi(hiStr)
				if err != nil {
					return nil, fmt.Errorf("invalid --pages range %q: %w", part, err)
				}
				end = hi
			}

			if start < 1 {
				return nil, fmt.Errorf("invalid --pages: page numbers must be at least 1 (got %d)", start)
			}
			if start > end {
				return nil, fmt.Errorf("invalid --pages range %d-%d: start is after end", start, end)
			}
			if start > total {
				return nil, fmt.Errorf("invalid --pages: page %d is out of range (document has %d page(s))", start, total)
			}
			if end > total {
				return nil, fmt.Errorf("invalid --pages: page %d is out of range (document has %d page(s))", end, total)
			}
			for p := start; p <= end; p++ {
				selected[p] = struct{}{}
			}
			continue
		}

		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid --pages segment %q: %w", part, err)
		}
		if n < 1 {
			return nil, fmt.Errorf("invalid --pages: page numbers must be at least 1 (got %d)", n)
		}
		if n > total {
			return nil, fmt.Errorf("invalid --pages: page %d is out of range (document has %d page(s))", n, total)
		}
		selected[n] = struct{}{}
	}

	out := make([]int, 0, len(selected))
	for p := range selected {
		out = append(out, p)
	}
	sort.Ints(out)
	return out, nil
}

func filterPagesBySelection(paths []string, selected []int) ([]string, error) {
	set := make(map[int]struct{}, len(selected))
	for _, p := range selected {
		set[p] = struct{}{}
	}
	var out []string
	for _, path := range paths {
		n, ok := pageNumberFromPBMPath(path)
		if !ok {
			return nil, fmt.Errorf("unexpected PBM filename (expected page-%%d.pbm): %s", path)
		}
		if _, keep := set[n]; keep {
			out = append(out, path)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no pages match --pages selection")
	}
	return out, nil
}

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

func runPipeline(ctx context.Context, typPath string, typstRoot string, copies int, cutSinglePage bool, reversePages bool, pagesSpec string, imgCfg *escposimg.Config, outputMethod, networkAddr, filePath string) error {
	if copies < 1 {
		return fmt.Errorf("copies must be at least 1")
	}

	absTyp, err := filepath.Abs(typPath)
	if err != nil {
		return fmt.Errorf("resolve typ path: %w", err)
	}
	baseDir := filepath.Dir(absTyp)
	rootDir := baseDir
	if typstRoot != "" {
		rootDir = typstRoot
	}

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
	slog.Debug("compiling Typst", "root", rootDir, "input", absTyp, "pdf", pdfPath)
	if err := compileTypst(ctx, rootDir, absTyp, pdfPath); err != nil {
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

	total := len(pages)
	selected, err := parsePageSelection(pagesSpec, total)
	if err != nil {
		return err
	}
	pages, err = filterPagesBySelection(pages, selected)
	if err != nil {
		return err
	}
	slog.Debug("page selection applied", "total_pdf_pages", total, "printing", len(pages), "spec", pagesSpec)

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
