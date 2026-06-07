// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 The PharosVPN Authors

package awgd

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// uapiDir is where amneziawg-go's ipc package serves the per-interface UAPI
// socket on FreeBSD (ipc.UAPIOpen/UAPIListen). pharos-awg logs this path on
// start ("uapi socket at /var/run/amneziawg/<if>.sock").
const uapiDir = "/var/run/amneziawg"

// Status is the live status of one tunnel interface, read from its UAPI socket.
// It is the structured form the diagnostics/status GUI pages render (the analog
// of WireGuard's wg_show.py output): the interface plus its single server peer
// with handshake age and RX/TX.
type Status struct {
	Interface  string     `json:"interface"`
	Up         bool       `json:"up"` // the socket answered (daemon running)
	ListenPort int        `json:"listen_port,omitempty"`
	Peers      []PeerStat `json:"peers"`
}

// PeerStat is one peer's live counters from the UAPI dump.
type PeerStat struct {
	PublicKey         string `json:"public_key"`              // base64
	Endpoint          string `json:"endpoint,omitempty"`      // the host:port actually dialed
	LatestHandshake   int64  `json:"latest_handshake"`        // unix seconds, 0 = never
	HandshakeAge      *int64 `json:"handshake_age,omitempty"` // seconds since handshake, nil = never
	TransferRx        int64  `json:"transfer_rx"`             // bytes received
	TransferTx        int64  `json:"transfer_tx"`             // bytes sent
	PersistentKeepalv int    `json:"persistent_keepalive,omitempty"`
	AllowedIPs        string `json:"allowed_ips,omitempty"`
}

// ReadStatus connects to the interface's UAPI socket, issues a `get=1` request,
// and parses the wireguard-go UAPI dump into a Status. A missing/closed socket
// (daemon not running) yields Up=false and no error so the GUI can render
// "disconnected" cleanly.
func ReadStatus(iface string) (*Status, error) {
	st := &Status{Interface: iface, Up: false}
	sock := fmt.Sprintf("%s/%s.sock", uapiDir, iface)
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return st, nil // not running — report "down", not an error
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("get=1\n\n")); err != nil {
		return st, fmt.Errorf("uapi write: %w", err)
	}

	st.Up = true
	now := time.Now().Unix()
	var cur *PeerStat
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break // end of response
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "listen_port":
			st.ListenPort = atoi(val)
		case "public_key":
			// hex on the wire; new peer record begins here.
			st.Peers = append(st.Peers, PeerStat{PublicKey: hexToB64(val)})
			cur = &st.Peers[len(st.Peers)-1]
		case "endpoint":
			if cur != nil {
				cur.Endpoint = val
			}
		case "last_handshake_time_sec":
			if cur != nil {
				cur.LatestHandshake = atoi64(val)
				if cur.LatestHandshake > 0 {
					age := now - cur.LatestHandshake
					cur.HandshakeAge = &age
				}
			}
		case "rx_bytes":
			if cur != nil {
				cur.TransferRx = atoi64(val)
			}
		case "tx_bytes":
			if cur != nil {
				cur.TransferTx = atoi64(val)
			}
		case "persistent_keepalive_interval":
			if cur != nil {
				cur.PersistentKeepalv = atoi(val)
			}
		case "allowed_ip":
			if cur != nil {
				if cur.AllowedIPs == "" {
					cur.AllowedIPs = val
				} else {
					cur.AllowedIPs += "," + val
				}
			}
		case "errno":
			if n := atoi(val); n != 0 {
				return st, fmt.Errorf("uapi errno %d", n)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return st, fmt.Errorf("uapi read: %w", err)
	}
	return st, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// hexToB64 converts a UAPI hex key to the base64 form `wg`/profiles use, so the
// GUI can match it against the configured public key. Returns the input on a
// parse failure (never panics on malformed UAPI data).
func hexToB64(h string) string {
	b, err := hex.DecodeString(strings.TrimSpace(h))
	if err != nil || len(b) != 32 {
		return h
	}
	return base64.StdEncoding.EncodeToString(b)
}
