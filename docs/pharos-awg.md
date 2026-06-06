# `pharos-awg` — userspace AmneziaWG data-plane daemon (FreeBSD/OPNsense)

Status: **implemented & verified on the live VM** (`opnsense-dev`, OPNsense 25.7 /
FreeBSD 14.3, `192.168.0.224`). This is step 1 of the integration order in
[`docs/integrations/opnsense.md`](https://github.com/PharosVPN/docs/blob/main/integrations/opnsense.md)
(§8): *`pharos-awg` + pkg → client mode → relay → node → coxswain*.

## What it does

`pharos-awg` runs a caravel AmneziaWG tunnel over a FreeBSD tun, applies the host
addressing/routing, and serves the wireguard UAPI socket for status tooling. It is
the single userspace data-plane unit both the OPNsense client-mode plugin and
(later) the server components build on.

```
pharos-awg --config /usr/local/etc/pharosvpn/awg0.conf   # run as root
pharos-awg --version
```

Lifecycle (`internal/awgd`):

1. `tun.CreateTUN(iface, mtu)` — creates the FreeBSD tun and **renames** the
   kernel-assigned `tunN` to the configured name (e.g. `awg0`).
2. `vp.Up(cfg, dev, level)` — caravel's engine builds the `amneziawg-go` device,
   applies the obfuscation params + server peer via UAPI, and brings it up. The
   obfuscation rendering lives **only** in caravel's `vp` package — no second copy.
3. host network (`network.go`): `ifconfig <if> inet A A up`, then routes per mode.
4. UAPI socket served at `/var/run/amneziawg/<if>.sock` (each connection → the
   tunnel's `IpcHandle`).
5. On `SIGINT`/`SIGTERM` (or engine exit): delete routes (LIFO), then close the
   tunnel — which destroys the interface and removes the socket.

### Routing modes (`routing`)

- `full` — default route via the tunnel: pin the endpoint to the physical gateway
  (`route add -host <server> <gw>`) so encrypted packets don't loop, then override
  the default with `0.0.0.0/1` + `128.0.0.0/1`.
- `split` — only the tunnel's `allowed_ips` CIDRs (the default route is skipped).
- `none` — address the interface only; the host owns routing. **This is what the
  OPNsense plugin will use** (pf policy-routing of selected LAN sources, §3.2 of
  the design), and it's what the VM smoke test used to avoid hijacking SSH.

## Config

JSON, written 0600 by the plugin. Either an inline resolved `tunnel`, or a
`profile` reference the daemon resolves itself (keeping `.pharos`
parsing/decryption in the audited Go core, not PHP — design §6). See
[`awg0.conf.example`](awg0.conf.example).

## Two corrections to the design doc, found while building

1. **UAPI socket path.** The design (§2) assumed `/var/run/wireguard/<if>.sock`.
   The amneziawg-go library's `ipc.UAPIOpen` actually uses
   **`/var/run/amneziawg/<if>.sock`** (umask 0077 → `srwx------`). The plugin's
   diagnostics page must read that path.
2. **Open question §9.2 answered.** `tun.CreateTUN("awg0", mtu)` on FreeBSD opens
   `/dev/tun`, takes the cloned `tunN`, and renames it to the exact name via
   `SIOCSIFNAME` — **no auto-numbering**, so pass the full name `awg0` (≤15 chars).
   It also errors if the name already exists, so teardown must destroy it (it does:
   `device.Close()` → tun `Close()` → `SIOCIFDESTROY`). Verified live: `awg0`
   appears in `group tun`, addresses cleanly, and is gone after `SIGTERM`.

## Dependency on caravel

Imports `github.com/PharosVPN/caravel/core/{vp,profile}` via a `replace =>
../caravel/go`. Adds two **purely additive** thin pass-throughs to caravel's `vp`
package so a long-lived daemon can drive the device it already owns:
`(*vp.Tunnel).IpcHandle(net.Conn)` (serve the UAPI socket) and
`(*vp.Tunnel).Wait()` (follow the engine's lifetime). No behaviour change for the
existing macOS/iOS/Android consumers. As part of this, `vp.Tunnel` is now
guarded by an `RWMutex` so a daemon can serve UAPI from many goroutines while
another goroutine calls `Close` — an adversarial review caught a data race on the
unsynchronised `dev` field (it pre-dated the daemon: `Close`/`Stats` were already
unguarded), now fixed and covered by a `-race` test.

## VM verification (2026-06-06, read it back from the box)

- `awg0` created, named, `inet 10.99.0.2`, `mtu 1420`, `group tun`.
- UAPI `get=1` returned every obfuscation param (jc/jmin/jmax/s1-4/h1-4), the
  peer/endpoint/keepalive/allowed-ips, and `tx_bytes=124734` — the obfuscated
  engine is emitting handshake/junk traffic out its UDP bind (data plane live;
  `rx=0` only because the test used an unroutable dummy endpoint).
- `SIGTERM` → graceful worker shutdown, `awg0` destroyed, socket removed.

## Next

- FreeBSD `pkg` (rc.d service + manifest declaring `if_tuntap`); see design §5.
- Client-mode `os-pharosvpn` plugin: interface assignment + group, gateway,
  outbound NAT, pf policy-routing, diagnostics reading the UAPI socket.
- A real end-to-end handshake against a live fleet node (needs a `.pharos` profile).
