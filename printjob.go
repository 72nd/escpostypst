package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/72nd/escposimg"
)

type printJobParams struct {
	PrintMode      string
	PaperWidthMM   int
	DPI            int
	NoCut          bool
	Copies         int
	CutSinglePage  bool
	DebugOutput    bool
	DebugImagePath string
	DebugText      string
}

func printJobParamsFromConfig(imgCfg *escposimg.Config, copies int, cutSinglePage bool, printMode string) printJobParams {
	return printJobParams{
		PrintMode:      printMode,
		PaperWidthMM:   imgCfg.PaperWidthMM,
		DPI:            imgCfg.DPI,
		NoCut:          !cutSinglePage,
		Copies:         copies,
		CutSinglePage:  cutSinglePage,
		DebugOutput:    imgCfg.DebugOutput,
		DebugImagePath: imgCfg.DebugImagePath,
		DebugText:      imgCfg.DebugText,
	}
}

func (p printJobParams) toImageConfig() (*escposimg.Config, error) {
	printMode, err := parsePrintMode(p.PrintMode)
	if err != nil {
		return nil, err
	}
	if p.Copies < 1 {
		return nil, fmt.Errorf("copies must be at least 1")
	}
	return &escposimg.Config{
		PaperWidthMM:   p.PaperWidthMM,
		DPI:            p.DPI,
		DitheringAlgo:  escposimg.DitheringNone,
		PrintMode:      printMode,
		DebugOutput:    p.DebugOutput,
		DebugImagePath: p.DebugImagePath,
		DebugText:      p.DebugText,
		CutPaper:       false,
	}, nil
}

func printJobParamsFromForm(values map[string][]string) (printJobParams, error) {
	get := func(key string) string {
		if v, ok := values[key]; ok && len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	parseBool := func(key string) (bool, error) {
		v := get(key)
		if v == "" {
			return false, nil
		}
		return strconv.ParseBool(v)
	}
	parseInt := func(key string) (int, error) {
		v := get(key)
		if v == "" {
			return 0, fmt.Errorf("%s is required", key)
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("invalid %s: %w", key, err)
		}
		return n, nil
	}

	paperWidth, err := parseInt("paper-width")
	if err != nil {
		return printJobParams{}, err
	}
	dpi, err := parseInt("dpi")
	if err != nil {
		return printJobParams{}, err
	}
	copies, err := parseInt("copies")
	if err != nil {
		return printJobParams{}, err
	}
	noCut, err := parseBool("no-cut")
	if err != nil {
		return printJobParams{}, fmt.Errorf("invalid no-cut: %w", err)
	}
	cutSinglePage, err := parseBool("cut-single-page")
	if err != nil {
		return printJobParams{}, fmt.Errorf("invalid cut-single-page: %w", err)
	}

	printMode := get("print-mode")
	if printMode == "" {
		printMode = "raster"
	}

	return printJobParams{
		PrintMode:      printMode,
		PaperWidthMM:   paperWidth,
		DPI:            dpi,
		NoCut:          noCut,
		Copies:         copies,
		CutSinglePage:  cutSinglePage,
		DebugOutput:    mustParseBool(get("debug-output")),
		DebugImagePath: defaultString(get("debug-image"), "debug_output.png"),
		DebugText:      get("debug-text"),
	}, nil
}

func mustParseBool(v string) bool {
	b, _ := strconv.ParseBool(v)
	return b
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
