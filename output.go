package main

import (
	"fmt"
	"strings"

	"github.com/72nd/escposimg"
	"github.com/urfave/cli/v3"
)

type destinationKind int

const (
	destStdout destinationKind = iota
	destNetwork
	destUSB
	destFile
	destRemote
)

type outputConfig struct {
	kind    destinationKind
	network string
	usb     string
	file    string
	remote  string
}

func destinationFlags(includeRemote bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "network", Usage: "Print via TCP to host:port (raw/JetDirect)", Local: true},
		&cli.StringFlag{Name: "usb", Usage: "Print via USB device path (e.g. /dev/usb/lp0)", Local: true},
		&cli.StringFlag{Name: "file", Usage: "Write ESC/POS bytes to file path", Local: true},
		&cli.BoolFlag{Name: "stdout", Usage: "Write ESC/POS bytes to stdout", Local: true},
	}
	if includeRemote {
		flags = append(flags, &cli.StringFlag{
			Name:  "remote",
			Usage: "Preprocess locally and print via escpostypst server at host:port",
			Local: true,
		})
	}
	return flags
}

func parseOutputConfig(cmd *cli.Command, includeRemote bool) (outputConfig, error) {
	var cfg outputConfig
	var count int

	if cmd.Bool("stdout") {
		count++
		cfg.kind = destStdout
	}
	if v := strings.TrimSpace(cmd.String("network")); v != "" {
		count++
		cfg.kind = destNetwork
		cfg.network = v
	}
	if v := strings.TrimSpace(cmd.String("usb")); v != "" {
		count++
		cfg.kind = destUSB
		cfg.usb = v
	}
	if v := strings.TrimSpace(cmd.String("file")); v != "" {
		count++
		cfg.kind = destFile
		cfg.file = v
	}
	if includeRemote {
		if v := strings.TrimSpace(cmd.String("remote")); v != "" {
			count++
			cfg.kind = destRemote
			cfg.remote = v
		}
	}

	if count == 0 {
		return outputConfig{}, fmt.Errorf("a destination flag is required (--network, --usb, --file, --stdout%s)", remoteFlagHint(includeRemote))
	}
	if count > 1 {
		return outputConfig{}, fmt.Errorf("only one destination flag may be set (--network, --usb, --file, --stdout%s)", remoteFlagHint(includeRemote))
	}
	return cfg, nil
}

func remoteFlagHint(includeRemote bool) string {
	if includeRemote {
		return ", or --remote"
	}
	return ""
}

func (c outputConfig) createOutputMethod() (escposimg.OutputMethod, error) {
	switch c.kind {
	case destStdout:
		return escposimg.NewStdoutOutput(), nil
	case destNetwork:
		if c.network == "" {
			return nil, fmt.Errorf("network address is required")
		}
		return escposimg.NewNetworkOutput(c.network)
	case destFile:
		if c.file == "" {
			return nil, fmt.Errorf("file path is required")
		}
		return escposimg.NewFileOutput(c.file)
	case destUSB:
		return escposimg.NewUSBOutput(c.usb)
	case destRemote:
		return nil, fmt.Errorf("remote destination cannot be used for local printing")
	default:
		return nil, fmt.Errorf("unknown destination")
	}
}
