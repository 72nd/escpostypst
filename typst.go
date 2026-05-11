package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// compileTypst runs `typst compile` with --root set to rootDir (the directory of the source file).
func compileTypst(ctx context.Context, rootDir, typSrcPath, pdfDestPath string) error {
	typstBin, err := exec.LookPath("typst")
	if err != nil {
		return fmt.Errorf("typst not found in PATH: %w", err)
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, typstBin, "compile", "--root", rootDir, typSrcPath, pdfDestPath)
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("typst compile: %w: %s", runErr, msg)
		}
		return fmt.Errorf("typst compile: %w", runErr)
	}
	return nil
}
