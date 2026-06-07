// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/PharosVPN/caravel/core/profile"
)

// Metadata is the resolved, non-secret summary of a .pharos profile, emitted as
// JSON by `pharos-awg inspect`. The GUI/PHP renders node choices and summaries
// from this without ever touching profile crypto (DESIGN §3.3, §6 — parsing
// stays in the audited Go core). It deliberately carries NO key material: only
// the metadata needed to populate the Profiles/Connection pages.
type Metadata struct {
	Enc       string        `json:"enc"`        // none | password | account
	FleetID   string        `json:"fleet_id"`   // fleet identifier
	User      string        `json:"user"`       // account label, if present
	Revision  int64         `json:"revision"`   // profile revision
	IssuedAt  string        `json:"issued_at"`  // RFC3339, empty if zero
	ExpiresAt string        `json:"expires_at"` // RFC3339, empty if zero
	Expired   bool          `json:"expired"`    // true if ExpiresAt is in the past
	Profiles  []ProfileMeta `json:"profiles"`   // the named connection configs
	Control   *ControlMeta  `json:"control,omitempty"`
}

// ProfileMeta is one named connection config (a ClientProfile) — its entry
// nodes and, for a cascade profile, the egress path.
type ProfileMeta struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Protocol string     `json:"protocol"` // amneziawg | xray-reality | both
	Nodes    []NodeMeta `json:"nodes"`
	Path     *PathMeta  `json:"path,omitempty"` // egress chain for a cascade profile
}

// NodeMeta is one selectable VPN endpoint (no keys, no endpoint pool — those are
// resolved at connect time by the daemon).
type NodeMeta struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Entry  bool   `json:"entry"` // true if this node is the cascade entry hop
}

// PathMeta is the ordered egress chain a cascade profile's traffic takes.
type PathMeta struct {
	Name string    `json:"name"`
	Hops []HopMeta `json:"hops"`
}

// HopMeta is one hop in an egress chain (entry → [mid] → exit).
type HopMeta struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Role   string `json:"role"`
}

// ControlMeta is the geo-located control-plane endpoint, if the profile carries
// one (for display only).
type ControlMeta struct {
	Label string  `json:"label"`
	City  string  `json:"city,omitempty"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

// InspectConfig resolves the profile referenced by a pharos-awg config file (the
// config's Profile block) and returns its non-secret metadata. It does not bring
// up any tunnel. An inline-tunnel config has no profile metadata to inspect.
func InspectConfig(cfgPath string) (*Metadata, error) {
	c, err := LoadConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	if c.Profile == nil {
		return nil, fmt.Errorf("config %s has no %q reference to inspect", cfgPath, "profile")
	}
	return InspectProfile(c.Profile.Path, profile.Options{Password: c.Profile.Password})
}

// InspectProfile parses a .pharos file with the given options and returns its
// non-secret metadata. Account-mode profiles need the device key + signer key in
// opts; password-mode needs opts.Password.
func InspectProfile(path string, opts profile.Options) (*Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	return inspectBytes(data, opts)
}

func inspectBytes(data []byte, opts profile.Options) (*Metadata, error) {
	p, err := profile.Parse(data, opts)
	if err != nil {
		return nil, err
	}
	m := &Metadata{
		Enc:       encMode(data),
		FleetID:   p.FleetID,
		User:      p.User,
		Revision:  p.Revision,
		IssuedAt:  rfc3339(p.IssuedAt),
		ExpiresAt: rfc3339(p.ExpiresAt),
		Expired:   !p.ExpiresAt.IsZero() && p.ExpiresAt.Before(time.Now()),
	}
	for i := range p.Profiles {
		m.Profiles = append(m.Profiles, profileMeta(&p.Profiles[i]))
	}
	if p.Control != nil {
		m.Control = &ControlMeta{
			Label: p.Control.Label,
			City:  p.Control.City,
			Lat:   p.Control.Lat,
			Lon:   p.Control.Lon,
		}
	}
	return m, nil
}

func profileMeta(cp *profile.ClientProfile) ProfileMeta {
	entry := cp.EntryNodeID()
	pm := ProfileMeta{ID: cp.ID, Name: cp.Name, Protocol: cp.Protocol}
	for _, n := range cp.Nodes {
		pm.Nodes = append(pm.Nodes, NodeMeta{
			ID:     n.ID,
			Name:   n.Name,
			Region: n.Region,
			Entry:  entry != "" && n.ID == entry,
		})
	}
	if cp.Path != nil {
		pm.Path = &PathMeta{Name: cp.Path.Name}
		for _, h := range cp.Path.Hops {
			pm.Path.Hops = append(pm.Path.Hops, HopMeta{
				ID: h.ID, Name: h.Name, Region: h.Region, Role: h.Role,
			})
		}
	}
	return pm
}

// encMode reads the always-readable `enc` header from the raw envelope so the
// GUI knows whether to prompt for a password / account key before importing.
func encMode(data []byte) string {
	var hdr struct {
		Fmt string `json:"fmt"`
		Enc string `json:"enc"`
	}
	if err := json.Unmarshal(data, &hdr); err != nil {
		return ""
	}
	return hdr.Enc
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
