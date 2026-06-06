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
- `plugin/` — an `os-pharosvpn` OPNsense MVC plugin (GUI page, configd actions,
  rc.d service), modelled on the in-core WireGuard plugin.
- `packaging/` — FreeBSD `pkg` build.

## Status

🚧 Pre-alpha — **design complete** (see the design doc), implementation pending.
Recommended order: **client mode** (the headline) → relay → experimental node
(its cascade netpolicy needs a `pf`/`setfib` rewrite). coxswain runs but is
discouraged on an edge box.

## License

Apache-2.0. Contributions under the DCO (`git commit -s`).
