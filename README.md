# escpostypst

A CLI tool that compiles [Typst](https://typst.app) documents and prints them to ESC/POS thermal printers.

## How it works

The tool runs a three-stage pipeline:

1. **Typst → PDF** — calls `typst compile` to produce a PDF from the source file.
2. **PDF → PBM** — calls Ghostscript (`gs`) to rasterize each PDF page into a raw PBM bitmap at the configured DPI. Each page becomes a separate file (`page-00001.pbm`, `page-00002.pbm`, …).
3. **PBM → printer** — passes each bitmap to [escposimg](https://github.com/72nd/escposimg), which encodes it as ESC/POS commands and sends it to the printer.

All temporary files (PDF + PBMs) are written to a hidden work directory inside the Typst file's folder and are deleted automatically when the job finishes.

Multi-page documents emit a paper cut between pages. Single-page documents cut by default (disable with `--no-cut`).

## Dependencies

| Tool | Purpose |
|------|---------|
| `typst` | Compile `.typ` source to PDF |
| `gs` (Ghostscript) | Rasterize PDF pages to PBM bitmaps |
| [escposimg](https://github.com/72nd/escposimg) | Encode bitmaps as ESC/POS and drive the printer |

Both `typst` and `gs` must be on `PATH`.

## Installation

```bash
go install github.com/72nd/escpostypst@latest
```

Or build from source:

```bash
git clone https://github.com/72nd/escpostypst
cd escpostypst
go build -o escpostypst .
```

## Usage

```
escpostypst [options] <path/to/document.typ>
```

### Output methods

| Method | Description |
|--------|-------------|
| `network` (default) | Send ESC/POS over TCP (raw/JetDirect). Requires `--host`. |
| `stdout` | Write raw ESC/POS bytes to stdout. |
| `file` | Write raw ESC/POS bytes to a file. Requires `--file-path`. |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dpi` | `203` | Printer resolution in DPI. |
| `--paper-width` | `72` | Paper width in millimetres. |
| `--print-mode` | `raster` | ESC/POS encoding mode: `raster`, `graphics`, or `column`. |
| `--output` | `network` | Output method: `network`, `stdout`, or `file`. |
| `--host` | — | Printer hostname or IP (required for `network` output). |
| `--port` | `9100` | TCP port for network output. |
| `--file-path` | — | Destination file (required for `file` output). |
| `--copies` / `-n` | `1` | Number of copies to print. |
| `--pages` | all | Page selection: `3`, `2-4`, `1,3,5`, `2-` (open-ended range). |
| `--reverse-pages` | off | Print pages in reverse order. |
| `--no-cut` | off | Suppress paper cut after single-page jobs. |
| `--root` | file dir | Typst project root passed to `typst compile --root`. |
| `--debug-output` | off | Save the processed bitmap for inspection. |
| `--debug-image` | `debug_output.png` | Path for the debug bitmap. |
| `--debug-text` | — | Text printed above the image (debug only). |
| `--verbose` | off | Enable debug-level logging. |

### Examples

Print to a network printer:

```bash
escpostypst --output network --host 192.168.1.100 invoice.typ
```

Print two copies of pages 1–3:

```bash
escpostypst --output network --host 192.168.1.100 --copies 2 --pages 1-3 invoice.typ
```

Write ESC/POS bytes to a file for inspection:

```bash
escpostypst --output file --file-path out.bin invoice.typ
```

## Source layout

| File | Responsibility |
|------|---------------|
| `main.go` | CLI definition, flag parsing, output-method wiring. |
| `typst.go` | Runs `typst compile` to produce the intermediate PDF. |
| `pipeline.go` | Orchestrates the full pipeline: temp directory, Ghostscript rasterisation, page selection/ordering, and the print loop. |
