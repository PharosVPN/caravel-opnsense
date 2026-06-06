// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// runner runs a host command (ifconfig/route). Injectable so tests can assert
// the command sequence without touching the real routing table.
type runner func(name string, args ...string) error

// gatewayFunc returns the current IPv4 default gateway. Injectable for tests.
type gatewayFunc func() (string, error)

// network owns the host networking state for one tunnel interface and tracks an
// undo list so teardown reverses exactly what bring-up installed. The BSD
// ifconfig/route command surface is shared between macOS (caravel-mac, the
// reference) and FreeBSD, so the port is mechanical.
type network struct {
	iface   string
	run     runner
	gateway gatewayFunc
	undo    []string // route specs ("net <cidr>" / "host <ip>") to delete on teardown
}

func newNetwork(iface string) *network {
	return &network{iface: iface, run: execRunner, gateway: defaultGateway}
}

// setAddress brings the interface up with a point-to-point address (the same
// "ifconfig <if> inet A A up" form caravel-mac uses on the BSD tun).
func (n *network) setAddress(addr string) error {
	if addr == "" {
		return errors.New("no tunnel address")
	}
	return n.run("ifconfig", n.iface, "inet", addr, addr, "up")
}

// configure installs host routes for the chosen routing mode. endpoint is the
// server host:port (pinned to the physical gateway in full-tunnel mode);
// allowedIPs are the tunnel's routed CIDRs (used in split mode).
func (n *network) configure(routing RoutingMode, endpoint string, allowedIPs []string) error {
	switch routing {
	case RoutingNone:
		return nil
	case RoutingSplit:
		for _, cidr := range allowedIPs {
			if isDefaultRoute(cidr) {
				continue // split tunnel never claims the default route
			}
			if err := n.addRoute("net", cidr, "-interface", n.iface); err != nil {
				return err
			}
		}
		return nil
	case RoutingFull:
		return n.fullTunnel(endpoint)
	default:
		return fmt.Errorf("unknown routing mode %q", routing)
	}
}

// fullTunnel pins the server endpoint to the physical gateway (so encrypted WG
// packets don't loop back into the tunnel) then overrides the default route with
// the 0.0.0.0/1 + 128.0.0.0/1 split.
func (n *network) fullTunnel(endpoint string) error {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	ip, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return fmt.Errorf("resolve endpoint %q: %w", host, err)
	}
	gw, err := n.gateway()
	if err != nil {
		return err
	}
	if err := n.addRoute("host", ip.String(), gw); err != nil {
		return err
	}
	for _, half := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		if err := n.addRoute("net", half, "-interface", n.iface); err != nil {
			return err
		}
	}
	return nil
}

// addRoute runs `route -n add -<kind> <dst> <rest...>` and records the undo.
func (n *network) addRoute(kind, dst string, rest ...string) error {
	args := append([]string{"-n", "add", "-" + kind, dst}, rest...)
	if err := n.run("route", args...); err != nil {
		return err
	}
	n.undo = append(n.undo, kind+" "+dst)
	return nil
}

// teardown reverses every route added, most-recent first. Errors are ignored —
// the interface is destroyed by the engine close anyway, which drops its routes.
func (n *network) teardown() {
	for i := len(n.undo) - 1; i >= 0; i-- {
		parts := strings.Fields(n.undo[i]) // e.g. ["net", "0.0.0.0/1"]
		_ = n.run("route", append([]string{"-n", "delete", "-" + parts[0]}, parts[1:]...)...)
	}
	n.undo = nil
}

func isDefaultRoute(cidr string) bool {
	return cidr == "0.0.0.0/0" || cidr == "::/0"
}

func execRunner(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// defaultGateway returns the current IPv4 default gateway by parsing
// `route -n get default` (identical output on macOS and FreeBSD).
func defaultGateway() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("read default route: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if gw, ok := strings.CutPrefix(strings.TrimSpace(line), "gateway:"); ok {
			return strings.TrimSpace(gw), nil
		}
	}
	return "", errors.New("no default gateway found")
}
