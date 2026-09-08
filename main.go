package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/72nd/escposimg"
	"github.com/urfave/cli/v3"
)

const appVersion = "0.1.4"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	var typArg string

	app := &cli.Command{
		Name:        "escpostypst",
		Usage:       "Print a Typst document to an ESC/POS thermal printer",
		Description: "Compile Typst to PDF, rasterize with Ghostscript to PBM (one file per page), then print via escposimg.",
		Version:     appVersion,
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:        "typfile",
				UsageText:   "PATH",
				Destination: &typArg,
			},
		},
		Flags: append(
			preprocessingFlags(),
			append(destinationFlags(true), &cli.BoolFlag{Name: "verbose", Usage: "Enable verbose logging", Local: true})...,
		),
		Action: func(ctx context.Context, cmd *cli.Command) error {
			setupVerbose(cmd)

			if strings.TrimSpace(typArg) == "" {
				return cli.Exit("Error: path to Typst file is required\n", 1)
			}

			imgCfg, printMode, copies, cutSinglePage, err := parsePrintOptions(cmd)
			if err != nil {
				return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
			}

			if err := validateTypFile(typArg); err != nil {
				return err
			}

			typstRoot, err := resolveTypstRoot(cmd)
			if err != nil {
				return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
			}

			dest, err := parseOutputConfig(cmd, true)
			if err != nil {
				return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
			}

			reversePages := !cmd.Bool("reverse-pages")
			pagesSpec := cmd.String("pages")

			if dest.kind == destRemote {
				return runRemote(ctx, typArg, typstRoot, copies, cutSinglePage, reversePages, pagesSpec, imgCfg, printMode, dest.remote)
			}

			return runPipeline(ctx, typArg, typstRoot, copies, cutSinglePage, reversePages, pagesSpec, imgCfg, dest)
		},
		Commands: []*cli.Command{
			{
				Name:        "serve",
				Usage:       "Start a print server that accepts preprocessed PBM pages",
				Description: "Listen for HTTP POST /print requests and send jobs to a local printer.",
				Flags: append(
					destinationFlags(false),
					&cli.StringFlag{Name: "listen", Value: ":8080", Usage: "Listen address (host:port)", Local: true},
					&cli.BoolFlag{Name: "verbose", Usage: "Enable verbose logging", Local: true},
				),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					setupVerbose(cmd)

					dest, err := parseOutputConfig(cmd, false)
					if err != nil {
						return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
					}

					return runServer(cmd.String("listen"), dest)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func preprocessingFlags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{Name: "paper-width", Value: 72, Usage: "Paper width in millimeters", Local: true},
		&cli.IntFlag{Name: "dpi", Value: 203, Usage: "Printer DPI", Local: true},
		&cli.StringFlag{Name: "root", Usage: "Typst project root (passed to typst compile --root); default is the directory of the input file", Local: true},
		&cli.StringFlag{Name: "print-mode", Value: "raster", Usage: "ESC/POS print mode (raster, graphics, column)", Local: true},
		&cli.BoolFlag{Name: "debug-output", Usage: "Save processed image for debugging", Local: true},
		&cli.StringFlag{Name: "debug-image", Value: "debug_output.png", Usage: "Path to save debug image", Local: true},
		&cli.StringFlag{Name: "debug-text", Usage: "Optional debug text to print before image", Local: true},
		&cli.BoolFlag{Name: "no-cut", Usage: "Do not send a paper cut after single-page jobs (multi-page documents still cut between pages)", Local: true},
		&cli.BoolFlag{Name: "reverse-pages", Usage: "Print pages in reverse order (last PDF page first)", Local: true},
		&cli.IntFlag{Name: "copies", Aliases: []string{"n"}, Value: 1, Usage: "Number of copies to print", Local: true},
		&cli.StringFlag{Name: "pages", Usage: "Pages to print, e.g. '3', '2-4', '1,3,5', '2-' (default: all)", Local: true},
	}
}

func setupVerbose(cmd *cli.Command) {
	if cmd.Bool("verbose") {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		slog.SetDefault(logger)
	}
}

func parsePrintOptions(cmd *cli.Command) (*escposimg.Config, string, int, bool, error) {
	copies := cmd.Int("copies")
	if copies < 1 {
		return nil, "", 0, false, fmt.Errorf("copies must be at least 1")
	}

	printMode := cmd.String("print-mode")
	printModeType, err := parsePrintMode(printMode)
	if err != nil {
		return nil, "", 0, false, err
	}

	imgCfg := &escposimg.Config{
		PaperWidthMM:   cmd.Int("paper-width"),
		DPI:            cmd.Int("dpi"),
		DitheringAlgo:  escposimg.DitheringNone,
		PrintMode:      printModeType,
		DebugOutput:    cmd.Bool("debug-output"),
		DebugImagePath: cmd.String("debug-image"),
		DebugText:      cmd.String("debug-text"),
		CutPaper:       false,
	}

	return imgCfg, printMode, copies, !cmd.Bool("no-cut"), nil
}

func validateTypFile(typArg string) error {
	st, err := os.Stat(typArg)
	if err != nil {
		return fmt.Errorf("typ file: %w", err)
	}
	if st.IsDir() {
		return fmt.Errorf("typ path is a directory: %s", typArg)
	}
	return nil
}

func resolveTypstRoot(cmd *cli.Command) (string, error) {
	r := strings.TrimSpace(cmd.String("root"))
	if r == "" {
		return "", nil
	}

	absRoot, err := filepath.Abs(r)
	if err != nil {
		return "", fmt.Errorf("--root: %w", err)
	}
	stRoot, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("--root: %w", err)
	}
	if !stRoot.IsDir() {
		return "", fmt.Errorf("--root is not a directory: %s", absRoot)
	}
	return absRoot, nil
}

func parsePrintMode(mode string) (escposimg.PrintMode, error) {
	switch strings.ToLower(mode) {
	case "raster":
		return escposimg.PrintModeRaster, nil
	case "graphics":
		return escposimg.PrintModeGraphics, nil
	case "column":
		return escposimg.PrintModeColumn, nil
	default:
		return 0, fmt.Errorf("unknown print mode: %s (supported: raster, graphics, column)", mode)
	}
}
