<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".assets/logo-inverse.svg">
    <img src=".assets/logo.svg" alt="PharosVPN" width="120" height="120">
  </picture>
</p>
# caravel-opnsense

PharosVPN for **OPNsense** (FreeBSD) firewalls — a PharosVPN endpoint as an
OPNsense plugin.

> Part of [PharosVPN](https://github.com/PharosVPN). Design: [`docs/integrations/opnsense.md`](https://github.com/PharosVPN/docs/blob/main/integrations/opnsense.md).

Two modes (the same box can do either):

- **Client mode** — the firewall is a [caravel](https://github.com/PharosVPN/caravel)
  client: import a profile, bring up the obfuscated tunnel, and route LAN traffic
  through it (whole-network VPN), with a kill-switch and policy routing. For a
  multi-hop profile it dials only the entry node.
- **Server mode** — the firewall runs a PharosVPN server component: a **relay**
  (easiest), a **node** (a VPN gateway), or **coxswain** (the controller).

## Data plane

Userspace **[amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go)** (it has a
native `tun_freebsd.go` backend; `if_tuntap` is resident) — the *same* obfuscated-
WireGuard engine the other clients use. FreeBSD has no AmneziaWG kernel module and
stock `if_wg` can't carry the obfuscation parameters, so the userspace engine is
the single data-plane strategy. Cross-compiled `GOOS=freebsd`.

## Architecture (planned)

- `cmd/` — the Go worker / `pharos-awg` daemon, reusing `caravel/go`'s
  `profile` / `vp` / `sync` (the `caravel-mac` BSD `ifconfig`/`route` shell ports
  closely).
- `plugin/` — the `os-pharosvpn` OPNsense MVC plugin (GUI pages, configd actions,
  service template, `.inc` hooks, `pharos-service-control.php`), modelled on the
  in-core WireGuard plugin.
- `pkg/` + `scripts/build-pkg.sh` — the FreeBSD `pkg` build (base `pharosvpn`
  binary pkg + the `os-pharosvpn` plugin pkg).

## Status

🚧 Pre-alpha. **Design complete** (see the design doc); **client mode shipped**.

- ✅ **`pharos-awg`** — the userspace AmneziaWG data-plane daemon
  ([`cmd/pharos-awg`](cmd/pharos-awg), [`internal/awgd`](internal/awgd)). Wraps
  caravel's `vp` engine over a FreeBSD tun, applies addressing/routing, serves the
  UAPI socket. Adds `inspect` (resolve a `.pharos`'s non-secret metadata as JSON —
  no crypto in PHP) and `status` (read a running tunnel's UAPI as JSON).
  Cross-compiles `GOOS=freebsd` and is **verified on the live VM** (interface
  bring-up/teardown, obfuscated data plane, UAPI). See
  [`docs/pharos-awg.md`](docs/pharos-awg.md).
- ✅ **`os-pharosvpn` plugin (client mode)** — the installable OPNsense MVC plugin
  ([`plugin/`](plugin)): a **VPN → PharosVPN** menu with Profiles / Connection /
  Status pages, profile import + inspection, the `pharosvpn` interface group,
  configd-managed lifecycle, a service template that renders the daemon config at
  0600, the `.inc` reconfigure/persistence hooks, and a
  [`pharos-service-control.php`](plugin/src/opnsense/scripts/PharosVPN/pharos-service-control.php)
  that drives `pharos-awg`. Packaged as the `os-pharosvpn` pkg (depends on a base
  `pharosvpn` binary pkg) — build with [`scripts/build-pkg.sh`](scripts/build-pkg.sh).
- ⬜ Not yet: a full live tunnel through a real fleet node + LAN policy-route /
  kill-switch leak tests + reboot persistence; relay → experimental node (its
  cascade netpolicy needs a `pf`/`setfib` rewrite) → coxswain (discouraged on edge).

### Verified vs not (on the dev VM, OPNsense 25.7 / FreeBSD 14.3)

- ✅ Both pkgs build (`pkg create`) and install; `os-pharosvpn` depends on `pharosvpn`.
- ✅ Model mounts/validates; the **VPN → PharosVPN** menu registers (3 entries); all
  controllers instantiate; `/api/pharosvpn/*` routes resolve.
- ✅ configd discovers all actions (start/stop/restart/configure/inspect/status);
  `configctl pharosvpn inspect` resolves profile metadata; the service template
  renders valid daemon JSON from the model.
- ✅ `configctl pharosvpn configure` starts the daemon, which creates `awg0`,
  addresses it, and serves the UAPI socket (`status` reports up + the peer/endpoint).
- ⬜ **Untested**: a real handshake / data through a live fleet node (needs a valid
  profile with a reachable endpoint); the `pf` outbound-NAT + policy-route +
  kill-switch *applied end-to-end* (the plumbing is wired via the interface group +
  service-control script, but not leak-tested); reboot-persistence path; CARP/HA.

## License

Apache-2.0. Contributions under the DCO (`git commit -s`).
