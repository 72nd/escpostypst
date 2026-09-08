package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/72nd/escposimg"
)

func runRemote(ctx context.Context, typPath string, typstRoot string, copies int, cutSinglePage bool, reversePages bool, pagesSpec string, imgCfg *escposimg.Config, printMode string, serverAddr string) error {
	pages, cleanup, err := preprocessPipeline(ctx, typPath, typstRoot, reversePages, pagesSpec, imgCfg.DPI)
	if err != nil {
		return err
	}
	defer cleanup()

	params := printJobParamsFromConfig(imgCfg, copies, cutSinglePage, printMode)
	return postPrintJob(ctx, serverAddr, pages, params)
}

func postPrintJob(ctx context.Context, serverAddr string, pages []string, params printJobParams) error {
	if len(pages) == 0 {
		return fmt.Errorf("no pages to send")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	writeField := func(name, value string) error {
		return writer.WriteField(name, value)
	}

	fields := map[string]string{
		"print-mode":       params.PrintMode,
		"paper-width":      strconv.Itoa(params.PaperWidthMM),
		"dpi":              strconv.Itoa(params.DPI),
		"no-cut":           strconv.FormatBool(params.NoCut),
		"copies":           strconv.Itoa(params.Copies),
		"cut-single-page":  strconv.FormatBool(params.CutSinglePage),
		"debug-output":     strconv.FormatBool(params.DebugOutput),
		"debug-image":      params.DebugImagePath,
		"debug-text":       params.DebugText,
	}
	for name, value := range fields {
		if err := writeField(name, value); err != nil {
			return fmt.Errorf("write form field %s: %w", name, err)
		}
	}

	for i, pagePath := range pages {
		filename := filepath.Base(pagePath)
		if filename == "" || filename == "." {
			filename = fmt.Sprintf("page-%d.pbm", i+1)
		}
		part, err := writer.CreateFormFile("page", filename)
		if err != nil {
			return fmt.Errorf("create form file for %s: %w", pagePath, err)
		}
		data, err := os.ReadFile(pagePath)
		if err != nil {
			return fmt.Errorf("read %s: %w", pagePath, err)
		}
		if _, err := part.Write(data); err != nil {
			return fmt.Errorf("write page data for %s: %w", pagePath, err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize multipart body: %w", err)
	}

	url := normalizeServerURL(serverAddr) + "/print"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	slog.Debug("sending print job to server", "url", url, "pages", len(pages), "copies", params.Copies)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("post print job: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		text := strings.TrimSpace(string(msg))
		if text == "" {
			return fmt.Errorf("server returned %s", resp.Status)
		}
		return fmt.Errorf("server returned %s: %s", resp.Status, text)
	}

	slog.Info("remote print job completed", "pages", len(pages), "copies", params.Copies, "server", serverAddr)
	return nil
}

func normalizeServerURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + strings.TrimRight(addr, "/")
}
