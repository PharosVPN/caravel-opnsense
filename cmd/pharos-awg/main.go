// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

// Command pharos-awg is PharosVPN's userspace AmneziaWG data-plane daemon for
// FreeBSD / OPNsense. It runs a caravel AmneziaWG tunnel over a FreeBSD tun,
// applies the host addressing and routing, and serves the wireguard UAPI socket
// for status tooling. Both the OPNsense client-mode plugin and (later) the
// server components drive it via configd.
//
//	pharos-awg --config /usr/local/etc/pharosvpn/awg0.conf
//	pharos-awg --version
//
// Run as root: creating the tun and changing routes require it.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/PharosVPN/caravel-opnsense/internal/awgd"
)

// version is overridable at build time: -ldflags "-X main.version=$(cat VERSION)".
var version = "dev"

func main() {
	cfgPath := flag.String("config", "", "path to the pharos-awg JSON config file")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Println("pharos-awg", version)
		return
	}

	logger := log.New(os.Stderr, "pharos-awg: ", log.LstdFlags)
	if *cfgPath == "" {
		logger.Println("--config is required")
		flag.Usage()
		os.Exit(2)
	}

	cfg, err := awgd.LoadConfig(*cfgPath)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	resolved, err := cfg.Resolve()
	if err != nil {
		logger.Fatalf("%v", err)
	}

	logf := func(format string, args ...any) { logger.Printf(format, args...) }
	if err := awgd.Run(context.Background(), resolved, logf); err != nil {
		logger.Fatalf("%v", err)
	}
}
