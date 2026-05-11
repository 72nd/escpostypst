package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/72nd/escposimg"
	"github.com/urfave/cli/v3"
)

const appVersion = "0.1.1"

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
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "paper-width", Value: 72, Usage: "Paper width in millimeters"},
			&cli.IntFlag{Name: "dpi", Value: 203, Usage: "Printer DPI"},
			&cli.StringFlag{Name: "print-mode", Value: "raster", Usage: "ESC/POS print mode (raster, graphics, column)"},
			&cli.BoolFlag{Name: "debug-output", Usage: "Save processed image for debugging"},
			&cli.StringFlag{Name: "debug-image", Value: "debug_output.png", Usage: "Path to save debug image"},
			&cli.StringFlag{Name: "debug-text", Usage: "Optional debug text to print before image"},
			&cli.BoolFlag{Name: "no-cut", Usage: "Do not send a paper cut after single-page jobs (multi-page documents still cut between pages)"},
			&cli.BoolFlag{Name: "reverse-pages", Usage: "Print pages in reverse order (last PDF page first)"},
			&cli.StringFlag{Name: "output", Value: "network", Usage: "Output method (stdout, network, file)"},
			&cli.StringFlag{Name: "host", Usage: "Printer hostname or IP for network output"},
			&cli.IntFlag{Name: "port", Value: 9100, Usage: "TCP port for network output (common raw/JetDirect default: 9100)"},
			&cli.StringFlag{Name: "file-path", Usage: "File path for file output"},
			&cli.BoolFlag{Name: "verbose", Usage: "Enable verbose logging"},
			&cli.IntFlag{Name: "copies", Aliases: []string{"n"}, Value: 1, Usage: "Number of copies to print"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("verbose") {
				logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
				slog.SetDefault(logger)
			}

			if strings.TrimSpace(typArg) == "" {
				return cli.Exit("Error: path to Typst file is required\n", 1)
			}

			copies := cmd.Int("copies")
			if copies < 1 {
				return cli.Exit("Error: copies must be at least 1\n", 1)
			}

			printModeType, err := parsePrintMode(cmd.String("print-mode"))
			if err != nil {
				return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
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

			st, err := os.Stat(typArg)
			if err != nil {
				return fmt.Errorf("typ file: %w", err)
			}
			if st.IsDir() {
				return fmt.Errorf("typ path is a directory: %s", typArg)
			}

			output := cmd.String("output")
			networkAddr, err := resolveNetworkAddress(output, cmd.String("host"), cmd.Int("port"))
			if err != nil {
				return cli.Exit(fmt.Sprintf("Error: %v\n", err), 1)
			}

			cutSinglePage := !cmd.Bool("no-cut")
			return runPipeline(ctx, typArg, copies, cutSinglePage, !cmd.Bool("reverse-pages"), imgCfg,
				output, networkAddr, cmd.String("file-path"))
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func resolveNetworkAddress(output, host string, port int) (string, error) {
	if strings.ToLower(strings.TrimSpace(output)) != "network" {
		return "", nil
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is required for network output (use --host)")
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
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

func createOutputMethod(method, networkAddr, filePath string) (escposimg.OutputMethod, error) {
	switch strings.ToLower(method) {
	case "stdout":
		return escposimg.NewStdoutOutput(), nil
	case "network":
		if networkAddr == "" {
			return nil, fmt.Errorf("network address is required for network output")
		}
		return escposimg.NewNetworkOutput(networkAddr)
	case "file":
		if filePath == "" {
			return nil, fmt.Errorf("file path is required for file output")
		}
		return escposimg.NewFileOutput(filePath)
	default:
		return nil, fmt.Errorf("unknown output method: %s", method)
	}
}
