// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"os"
	"path/filepath"
	"testing"
)

// a valid 32-byte key, base64 — caravel's keyHex requires exactly 32 bytes.
const testKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "awg0.conf")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveInlineTunnelDefaults(t *testing.T) {
	p := writeConfig(t, `{
		"address": "10.86.0.5",
		"tunnel": {
			"private_key": "`+testKey+`",
			"server_public_key": "`+testKey+`",
			"endpoint": "203.0.113.7:443",
			"obfuscation": {"jc": 5, "jmin": 25, "jmax": 800}
		}
	}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	r, err := cfg.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if r.Interface != DefaultInterface {
		t.Errorf("interface = %q, want %q", r.Interface, DefaultInterface)
	}
	if r.MTU != DefaultMTU {
		t.Errorf("mtu = %d, want %d", r.MTU, DefaultMTU)
	}
	if r.Routing != RoutingFull {
		t.Errorf("routing = %q, want %q", r.Routing, RoutingFull)
	}
	if r.VP.Endpoint != "203.0.113.7:443" {
		t.Errorf("endpoint = %q", r.VP.Endpoint)
	}
	if r.VP.Obfuscation.Jmax != 800 {
		t.Errorf("jmax = %d, want 800", r.VP.Obfuscation.Jmax)
	}
}

func TestResolveValidation(t *testing.T) {
	cases := map[string]string{
		"no tunnel or profile": `{"address": "10.0.0.1"}`,
		"missing address":      `{"tunnel": {"private_key": "` + testKey + `", "server_public_key": "` + testKey + `", "endpoint": "x:1"}}`,
		"missing endpoint":     `{"address": "10.0.0.1", "tunnel": {"private_key": "` + testKey + `", "server_public_key": "` + testKey + `"}}`,
		"bad routing":          `{"address": "10.0.0.1", "routing": "sideways", "tunnel": {"private_key": "` + testKey + `", "server_public_key": "` + testKey + `", "endpoint": "x:1"}}`,
		"long interface":       `{"interface": "thisnameistoolong", "address": "10.0.0.1", "tunnel": {"private_key": "` + testKey + `", "server_public_key": "` + testKey + `", "endpoint": "x:1"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := LoadConfig(writeConfig(t, body))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cfg.Resolve(); err == nil {
				t.Fatalf("expected validation error for %q", name)
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	for in, want := range map[string]int{"silent": 0, "verbose": 2, "error": 1, "": 1, "bogus": 1} {
		if got := logLevel(in); got != want {
			t.Errorf("logLevel(%q) = %d, want %d", in, got, want)
		}
	}
}
