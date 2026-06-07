// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

// Command pharos-awg is PharosVPN's userspace AmneziaWG data-plane daemon for
// FreeBSD / OPNsense. It runs a caravel AmneziaWG tunnel over a FreeBSD tun,
// applies the host addressing/routing, and serves the wireguard UAPI socket for
// status tooling. Both the OPNsense client-mode plugin and (later) the server
// components drive it via configd.
//
//	pharos-awg --config /usr/local/etc/pharosvpn/awg0.conf      # run the tunnel
//	pharos-awg inspect --config <json>                          # profile metadata (JSON)
//	pharos-awg inspect --profile <file> [--password <pw>]       # profile metadata (JSON)
//	pharos-awg status --interface awg0                          # live UAPI status (JSON)
//	pharos-awg --version
//
// inspect/status emit JSON for the GUI/PHP so the plugin never does profile
// crypto in PHP (DESIGN §3.3, §6 — parsing stays in the audited Go core).
//
// Run as root: creating the tun and changing routes require it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/PharosVPN/caravel-opnsense/internal/awgd"
	"github.com/PharosVPN/caravel/core/profile"
)

// version is overridable at build time: -ldflags "-X main.version=$(cat VERSION)".
var version = "dev"

func main() {
	// Subcommand dispatch: the first non-flag arg, if present, selects a mode.
	// The default (no subcommand) runs the tunnel from --config, preserving the
	// original CLI so configd's existing rc.d invocation is unaffected.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "inspect":
			os.Exit(runInspect(os.Args[2:]))
		case "status":
			os.Exit(runStatus(os.Args[2:]))
		case "version", "--version", "-version":
			fmt.Println("pharos-awg", version)
			return
		}
	}
	os.Exit(runDaemon(os.Args[1:]))
}

// runDaemon is the default mode: bring up the tunnel described by --config and
// block until signalled. This is what configd's rc.d action runs.
func runDaemon(args []string) int {
	fs := flag.NewFlagSet("pharos-awg", flag.ExitOnError)
	cfgPath := fs.String("config", "", "path to the pharos-awg JSON config file")
	showVer := fs.Bool("version", false, "print version and exit")
	_ = fs.Parse(args)

	if *showVer {
		fmt.Println("pharos-awg", version)
		return 0
	}

	logger := log.New(os.Stderr, "pharos-awg: ", log.LstdFlags)
	if *cfgPath == "" {
		logger.Println("--config is required")
		fs.Usage()
		return 2
	}

	cfg, err := awgd.LoadConfig(*cfgPath)
	if err != nil {
		logger.Printf("%v", err)
		return 1
	}
	resolved, err := cfg.Resolve()
	if err != nil {
		logger.Printf("%v", err)
		return 1
	}

	logf := func(format string, a ...any) { logger.Printf(format, a...) }
	if err := awgd.Run(context.Background(), resolved, logf); err != nil {
		logger.Printf("%v", err)
		return 1
	}
	return 0
}

// runInspect resolves a profile's non-secret metadata and prints it as JSON. It
// takes either --config (resolves the config's profile reference) or --profile
// with an optional --password. No tunnel is brought up.
func runInspect(args []string) int {
	fs := flag.NewFlagSet("pharos-awg inspect", flag.ExitOnError)
	cfgPath := fs.String("config", "", "pharos-awg config whose profile reference to inspect")
	profPath := fs.String("profile", "", "path to a .pharos profile to inspect")
	password := fs.String("password", "", "password for a password-mode profile")
	_ = fs.Parse(args)

	var (
		meta *awgd.Metadata
		err  error
	)
	switch {
	case *profPath != "":
		meta, err = awgd.InspectProfile(*profPath, profile.Options{Password: *password})
	case *cfgPath != "":
		meta, err = awgd.InspectConfig(*cfgPath)
	default:
		fmt.Fprintln(os.Stderr, "inspect: one of --config or --profile is required")
		return 2
	}
	if err != nil {
		// Emit a structured error so the GUI can show it (e.g. password needed).
		emitJSON(map[string]string{"error": err.Error()})
		return 1
	}
	emitJSON(meta)
	return 0
}

// runStatus reads a running interface's UAPI socket and prints live status as
// JSON (the analog of WireGuard's wg_show). A down interface reports up:false.
func runStatus(args []string) int {
	fs := flag.NewFlagSet("pharos-awg status", flag.ExitOnError)
	iface := fs.String("interface", awgd.DefaultInterface, "tun interface to query")
	_ = fs.Parse(args)

	st, err := awgd.ReadStatus(*iface)
	if err != nil {
		emitJSON(map[string]any{"interface": *iface, "up": false, "error": err.Error()})
		return 1
	}
	emitJSON(st)
	return 0
}

// emitJSON prints v as indented JSON to stdout for the GUI/PHP to consume.
func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
