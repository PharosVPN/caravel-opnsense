// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

// Package awgd is the pharos-awg daemon: it runs a caravel AmneziaWG tunnel over
// a FreeBSD tun, applies the host addressing/routing, and serves the wireguard
// UAPI socket for status tooling. It is the userspace data-plane unit both the
// OPNsense client-mode plugin and (later) the server components build on.
package awgd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/PharosVPN/caravel/core/profile"
	"github.com/PharosVPN/caravel/core/vp"
	"github.com/amnezia-vpn/amneziawg-go/device"
)

const (
	// DefaultInterface is the tun interface name pharos-awg creates. FreeBSD's
	// tun cloning hands back a generic tunN which amneziawg-go renames to this
	// via SIOCSIFNAME, so the name is stable and predictable for OPNsense
	// interface assignment.
	DefaultInterface = "awg0"
	// DefaultMTU is the AmneziaWG default (device.DefaultMTU).
	DefaultMTU = 1420
	// maxIfaceName is FreeBSD's IFNAMSIZ-1; CreateTUN rejects longer names.
	maxIfaceName = 15
)

// RoutingMode selects how the host routing table is set up for the tunnel.
type RoutingMode string

const (
	// RoutingFull sends the default route through the tunnel: the server
	// endpoint is pinned to the physical gateway and 0.0.0.0/1 + 128.0.0.0/1
	// override the default without deleting it.
	RoutingFull RoutingMode = "full"
	// RoutingSplit routes only the tunnel's AllowedIPs CIDRs (excluding the
	// default route) through the interface.
	RoutingSplit RoutingMode = "split"
	// RoutingNone addresses the interface only and installs no routes — the
	// host (e.g. OPNsense pf policy-routing) owns the routing decision.
	RoutingNone RoutingMode = "none"
)

// Config is the on-disk pharos-awg configuration (JSON, written at 0600 by the
// plugin). It carries either an already-resolved Tunnel or a reference to a
// .pharos Profile the daemon resolves itself (keeping profile decryption in the
// audited Go core, never in PHP — DESIGN §6).
type Config struct {
	Interface string      `json:"interface"` // tun name; default awg0
	Address   string      `json:"address"`   // bare tunnel IP; from the profile if empty
	MTU       int         `json:"mtu"`       // default 1420
	Routing   RoutingMode `json:"routing"`   // full | split | none; default full
	LogLevel  string      `json:"log_level"` // silent | error | verbose; default error

	Tunnel  *TunnelConfig `json:"tunnel,omitempty"`  // a pre-resolved tunnel
	Profile *ProfileRef   `json:"profile,omitempty"` // or a .pharos to resolve
}

// TunnelConfig is a resolved AmneziaWG tunnel — the vp.Config inputs carried
// directly in the daemon config (keys base64, as profiles carry them).
type TunnelConfig struct {
	PrivateKey      string      `json:"private_key"`
	ServerPublicKey string      `json:"server_public_key"`
	PresharedKey    string      `json:"preshared_key,omitempty"`
	Endpoint        string      `json:"endpoint"`
	AllowedIPs      []string    `json:"allowed_ips,omitempty"`
	Keepalive       int         `json:"keepalive,omitempty"`
	Obfuscation     Obfuscation `json:"obfuscation"`
}

// Obfuscation mirrors the AmneziaWG obfuscation parameter set the server node
// advertises (must match exactly or the handshake fails).
type Obfuscation struct {
	Jc   uint32 `json:"jc"`
	Jmin uint32 `json:"jmin"`
	Jmax uint32 `json:"jmax"`
	S1   uint32 `json:"s1"`
	S2   uint32 `json:"s2"`
	S3   uint32 `json:"s3"`
	S4   uint32 `json:"s4"`
	H1   uint32 `json:"h1"`
	H2   uint32 `json:"h2"`
	H3   uint32 `json:"h3"`
	H4   uint32 `json:"h4"`
	I1   string `json:"i1,omitempty"`
	I2   string `json:"i2,omitempty"`
	I3   string `json:"i3,omitempty"`
	I4   string `json:"i4,omitempty"`
	I5   string `json:"i5,omitempty"`
}

// ProfileRef points the daemon at a .pharos profile to resolve at startup.
type ProfileRef struct {
	Path     string `json:"path"`               // path to the .pharos file
	Password string `json:"password,omitempty"` // for password-mode profiles
	Profile  string `json:"profile,omitempty"`  // named connection config id/name; empty = first
	Node     string `json:"node,omitempty"`     // node id/name; empty = entry/first hop
}

// Resolved is a validated, ready-to-apply tunnel spec.
type Resolved struct {
	Interface string
	Address   string
	MTU       int
	Routing   RoutingMode
	LogLevel  int
	VP        vp.Config
}

// LoadConfig reads and JSON-decodes a pharos-awg config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}
	return &c, nil
}

// Resolve applies defaults, resolves the tunnel (from the inline Tunnel or by
// parsing the referenced .pharos profile), and validates the result.
func (c *Config) Resolve() (*Resolved, error) {
	r := &Resolved{
		Interface: orDefault(c.Interface, DefaultInterface),
		Address:   c.Address,
		MTU:       c.MTU,
		Routing:   c.Routing,
		LogLevel:  logLevel(c.LogLevel),
	}
	if r.MTU == 0 {
		r.MTU = DefaultMTU
	}
	if r.Routing == "" {
		r.Routing = RoutingFull
	}

	switch {
	case c.Profile != nil:
		if err := c.resolveFromProfile(r); err != nil {
			return nil, err
		}
	case c.Tunnel != nil:
		r.VP = vp.Config{
			PrivateKey:      c.Tunnel.PrivateKey,
			ServerPublicKey: c.Tunnel.ServerPublicKey,
			PresharedKey:    c.Tunnel.PresharedKey,
			Endpoint:        c.Tunnel.Endpoint,
			AllowedIPs:      c.Tunnel.AllowedIPs,
			Keepalive:       c.Tunnel.Keepalive,
			Obfuscation:     c.Tunnel.Obfuscation.toVP(),
		}
	default:
		return nil, fmt.Errorf("config must set either %q or %q", "tunnel", "profile")
	}

	if err := r.validate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (c *Config) resolveFromProfile(r *Resolved) error {
	data, err := os.ReadFile(c.Profile.Path)
	if err != nil {
		return fmt.Errorf("read profile: %w", err)
	}
	p, err := profile.Parse(data, profile.Options{Password: c.Profile.Password})
	if err != nil {
		return fmt.Errorf("parse profile: %w", err)
	}
	cp, err := p.Select(c.Profile.Profile)
	if err != nil {
		return fmt.Errorf("select profile: %w", err)
	}
	node, err := cp.Node(c.Profile.Node)
	if err != nil {
		return fmt.Errorf("select node: %w", err)
	}
	t, err := node.Tunnel()
	if err != nil {
		return fmt.Errorf("resolve tunnel: %w", err)
	}
	r.VP = vp.Config{
		PrivateKey:      t.PrivateKey,
		ServerPublicKey: t.ServerPublicKey,
		PresharedKey:    t.PresharedKey,
		Endpoint:        t.Endpoint,
		AllowedIPs:      t.AllowedIPs,
		Keepalive:       t.Keepalive,
		Obfuscation:     obfFromProfile(t.Obfuscation),
	}
	if r.Address == "" {
		r.Address = t.Address
	}
	if c.MTU == 0 && t.MTU != 0 {
		r.MTU = t.MTU
	}
	return nil
}

func (r *Resolved) validate() error {
	if r.Interface == "" || len(r.Interface) > maxIfaceName {
		return fmt.Errorf("interface name %q must be 1..%d chars", r.Interface, maxIfaceName)
	}
	if r.Address == "" {
		return fmt.Errorf("no tunnel address (set %q or use a profile that carries one)", "address")
	}
	switch r.Routing {
	case RoutingFull, RoutingSplit, RoutingNone:
	default:
		return fmt.Errorf("unknown routing mode %q (want full|split|none)", r.Routing)
	}
	if r.VP.PrivateKey == "" || r.VP.ServerPublicKey == "" {
		return fmt.Errorf("tunnel is missing a private or server public key")
	}
	if r.VP.Endpoint == "" {
		return fmt.Errorf("tunnel has no endpoint to dial")
	}
	return nil
}

func (o Obfuscation) toVP() vp.Obfuscation {
	return vp.Obfuscation{
		Jc: o.Jc, Jmin: o.Jmin, Jmax: o.Jmax,
		S1: o.S1, S2: o.S2, S3: o.S3, S4: o.S4,
		H1: o.H1, H2: o.H2, H3: o.H3, H4: o.H4,
		I1: o.I1, I2: o.I2, I3: o.I3, I4: o.I4, I5: o.I5,
	}
}

func obfFromProfile(o profile.Obfuscation) vp.Obfuscation {
	return vp.Obfuscation{
		Jc: o.Jc, Jmin: o.Jmin, Jmax: o.Jmax,
		S1: o.S1, S2: o.S2, S3: o.S3, S4: o.S4,
		H1: o.H1, H2: o.H2, H3: o.H3, H4: o.H4,
		I1: o.I1, I2: o.I2, I3: o.I3, I4: o.I4, I5: o.I5,
	}
}

func logLevel(s string) int {
	switch s {
	case "silent":
		return device.LogLevelSilent
	case "verbose":
		return device.LogLevelVerbose
	default:
		return device.LogLevelError
	}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
