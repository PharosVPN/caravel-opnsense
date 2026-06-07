// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PharosVPN/caravel/core/profile"
)

// a minimal valid none-mode .pharos envelope: a one-node AmneziaWG profile.
// Mirrors the controller's format (fmt/v/enc header + JSON payload).
const samplePharos = `{
  "fmt": "pharos-profile",
  "v": 1,
  "enc": "none",
  "payload": {
    "fleet_id": "fleet-demo",
    "user": "usr_demo",
    "revision": 7,
    "expires_at": "2030-01-01T00:00:00Z",
    "profiles": [
      {
        "id": "pspec_demo",
        "name": "Amsterdam Direct",
        "protocol": "amneziawg",
        "nodes": [
          {
            "id": "nod_ams1",
            "name": "Amsterdam-1",
            "region": "eu-nl",
            "endpoints": ["203.0.113.7"],
            "protocols": [
              {
                "type": "amneziawg",
                "v": 2,
                "params": {
                  "private_key": "QlpVeFhVc0xkSGRoY21VZ2FYTWdkR2hsSUd4dloyOD0=",
                  "address": "10.86.0.5/32",
                  "public_key": "U0VWU1JTQkpVeUJVU0VVZ1RFOUhUeUJoYm1RZ2FYUT0=",
                  "endpoints": [{"ip": "203.0.113.7", "port_min": 51820, "port_max": 51830}],
                  "allowed_ips": ["0.0.0.0/0", "::/0"],
                  "obfuscation": {"jc": 5, "jmin": 25, "jmax": 800}
                }
              }
            ]
          }
        ]
      }
    ]
  }
}`

func writeProfile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fleet.pharos")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInspectProfileMetadata(t *testing.T) {
	path := writeProfile(t, samplePharos)
	m, err := InspectProfile(path, profile.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Enc != "none" {
		t.Errorf("enc = %q, want none", m.Enc)
	}
	if m.FleetID != "fleet-demo" {
		t.Errorf("fleet = %q", m.FleetID)
	}
	if m.Expired {
		t.Error("profile should not be expired (expires 2030)")
	}
	if len(m.Profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(m.Profiles))
	}
	cp := m.Profiles[0]
	if cp.Name != "Amsterdam Direct" || cp.Protocol != "amneziawg" {
		t.Errorf("profile = %+v", cp)
	}
	if len(cp.Nodes) != 1 || cp.Nodes[0].Name != "Amsterdam-1" || cp.Nodes[0].Region != "eu-nl" {
		t.Errorf("nodes = %+v", cp.Nodes)
	}
}

func TestInspectConfigViaProfileRef(t *testing.T) {
	profPath := writeProfile(t, samplePharos)
	cfgBody := `{"interface":"awg0","routing":"none","profile":{"path":"` + profPath + `"}}`
	cfgPath := writeConfig(t, cfgBody)
	m, err := InspectConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.FleetID != "fleet-demo" || len(m.Profiles) != 1 {
		t.Errorf("metadata = %+v", m)
	}
}

func TestInspectRejectsNonPharos(t *testing.T) {
	path := writeProfile(t, `{"hello":"world"}`)
	if _, err := InspectProfile(path, profile.Options{}); err == nil {
		t.Fatal("expected error parsing non-pharos file")
	}
}

func TestResolveProfileRefBringsUpTunnel(t *testing.T) {
	profPath := writeProfile(t, samplePharos)
	cfgBody := `{"interface":"awg0","routing":"none","profile":{"path":"` + profPath + `"}}`
	cfg, err := LoadConfig(writeConfig(t, cfgBody))
	if err != nil {
		t.Fatal(err)
	}
	r, err := cfg.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if r.Address != "10.86.0.5" {
		t.Errorf("address = %q, want 10.86.0.5 (CIDR stripped)", r.Address)
	}
	if r.VP.ServerPublicKey == "" || r.VP.PrivateKey == "" {
		t.Error("resolved tunnel missing keys")
	}
	if r.VP.Obfuscation.Jmax != 800 {
		t.Errorf("jmax = %d, want 800", r.VP.Obfuscation.Jmax)
	}
}
