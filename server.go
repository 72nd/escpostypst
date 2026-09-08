package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runServer(listenAddr string, dest outputConfig) error {
	slog.Info("print server listening", "addr", listenAddr)
	srv := &http.Server{
		Addr:    listenAddr,
		Handler: newPrintServer(dest),
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func newPrintServer(dest outputConfig) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /print", func(w http.ResponseWriter, r *http.Request) {
		handlePrintRequest(w, r, dest)
	})
	return mux
}

func handlePrintRequest(w http.ResponseWriter, r *http.Request, dest outputConfig) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("parse multipart form: %w", err))
		return
	}

	params, err := printJobParamsFromForm(r.MultipartForm.Value)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}

	imgCfg, err := params.toImageConfig()
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}

	pages, cleanup, err := saveUploadedPages(r)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	defer cleanup()

	slog.Info("received print job", "pages", len(pages), "copies", params.Copies, "remote", r.RemoteAddr)
	if err := printPages(r.Context(), pages, params.Copies, params.CutSinglePage, imgCfg, dest); err != nil {
		writeHTTPError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func saveUploadedPages(r *http.Request) (pages []string, cleanup func(), err error) {
	workDir, err := os.MkdirTemp("", "escpostypst-server-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create work dir: %w", err)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(workDir); rmErr != nil {
			slog.Warn("failed to remove server work dir", "path", workDir, "error", rmErr)
		}
	}

	files := r.MultipartForm.File["page"]
	if len(files) == 0 {
		cleanup()
		return nil, nil, fmt.Errorf("no page files in request")
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].Filename < files[j].Filename
	})

	pages = make([]string, 0, len(files))
	for i, header := range files {
		src, openErr := header.Open()
		if openErr != nil {
			cleanup()
			return nil, nil, fmt.Errorf("open uploaded page %s: %w", header.Filename, openErr)
		}

		filename := strings.TrimSpace(header.Filename)
		if filename == "" {
			filename = fmt.Sprintf("page-%d.pbm", i+1)
		}
		destPath := filepath.Join(workDir, filepath.Base(filename))
		dst, createErr := os.Create(destPath)
		if createErr != nil {
			src.Close()
			cleanup()
			return nil, nil, fmt.Errorf("create page file %s: %w", destPath, createErr)
		}

		if _, copyErr := io.Copy(dst, src); copyErr != nil {
			dst.Close()
			src.Close()
			cleanup()
			return nil, nil, fmt.Errorf("save page file %s: %w", destPath, copyErr)
		}
		dst.Close()
		src.Close()
		pages = append(pages, destPath)
	}

	sortPBMPathsByPageNumber(pages)
	return pages, cleanup, nil
}

func writeHTTPError(w http.ResponseWriter, status int, err error) {
	slog.Error("print request failed", "status", status, "error", err)
	http.Error(w, err.Error(), status)
}
