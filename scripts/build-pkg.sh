#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
#
# Hand-assemble the two installable FreeBSD packages on a FreeBSD/OPNsense box:
#   - pharosvpn      : the pharos-awg daemon + rc.d (base binary pkg)
#   - os-pharosvpn   : the GUI plugin (MVC + configd + templates + .inc hooks)
#
# Standard OPNsense plugins build via their Makefile + opnsense/tools infra;
# that infra isn't on the dev VM, so this script does the equivalent with
# `pkg create` (the FreeBSD primitive the plugin Makefile ultimately calls).
#
# Run ON a FreeBSD host (sh, not tcsh). The pharos-awg binary must already be
# cross-compiled into bin/freebsd-amd64/ (scripts/build.sh).
#
#   sh scripts/build-pkg.sh [version]
#
set -eu

cd "$(dirname "$0")/.."
ROOT=$(pwd)
VER="${1:-$(tr -d '[:space:]' < VERSION 2>/dev/null || echo 0.0.0)}"
ABI="FreeBSD:14:amd64"
ARCH="freebsd:14:x86:64"
OUT="$ROOT/build/pkg"
WORK="$ROOT/build/work"
BIN="$ROOT/bin/freebsd-amd64/pharos-awg"

if [ ! -x "$BIN" ]; then
    echo "error: $BIN missing — run scripts/build.sh first (on the Mac)" >&2
    exit 1
fi

rm -rf "$WORK" "$OUT"
mkdir -p "$OUT" "$WORK"

##############################################################################
# 1) base pkg: pharosvpn (the daemon binary + rc.d)
##############################################################################
STAGE="$WORK/pharosvpn"
mkdir -p "$STAGE/usr/local/sbin" "$STAGE/usr/local/etc/rc.d"
install -m 0755 "$BIN" "$STAGE/usr/local/sbin/pharos-awg"
install -m 0755 "$ROOT/pkg/pharosvpn/usr/local/etc/rc.d/pharos-awg" "$STAGE/usr/local/etc/rc.d/pharos-awg"

sed "s/%%VERSION%%/$VER/" "$ROOT/pkg/pharosvpn/+MANIFEST.in" > "$WORK/pharosvpn.manifest"

# plist: every file the pkg owns, prefix-relative
( cd "$STAGE" && find usr -type f | sed 's,^,/,' ) > "$WORK/pharosvpn.plist"

pkg create -M "$WORK/pharosvpn.manifest" -p "$WORK/pharosvpn.plist" -r "$STAGE" -o "$OUT"
echo "built: $OUT/pharosvpn-$VER.pkg"

##############################################################################
# 2) plugin pkg: os-pharosvpn (the MVC GUI + configd + templates + .inc)
##############################################################################
PVER="${PLUGIN_VERSION:-1.0.0}"
PSTAGE="$WORK/os-pharosvpn"
mkdir -p "$PSTAGE/usr/local"
# copy the whole plugin overlay tree into the staging prefix
cp -R "$ROOT/plugin/src/." "$PSTAGE/usr/local/.local_tmp" 2>/dev/null || true
# the overlay is rooted at usr/local already inside plugin/src? No — it's src/{etc,opnsense}
rm -rf "$PSTAGE/usr/local/.local_tmp"
# plugin/src/etc -> /usr/local/etc ;  plugin/src/opnsense -> /usr/local/opnsense
mkdir -p "$PSTAGE/usr/local/etc" "$PSTAGE/usr/local/opnsense"
cp -R "$ROOT/plugin/src/etc/." "$PSTAGE/usr/local/etc/"
cp -R "$ROOT/plugin/src/opnsense/." "$PSTAGE/usr/local/opnsense/"

# make the PHP CLI scripts executable
chmod 0755 "$PSTAGE/usr/local/opnsense/scripts/PharosVPN/"*.php 2>/dev/null || true

cat > "$WORK/os-pharosvpn.manifest" <<EOF
name: os-pharosvpn
origin: opnsense/os-pharosvpn
version: "$PVER"
comment: PharosVPN client (caravel) for OPNsense
maintainer: dev@pharosvpn.invalid
www: https://github.com/PharosVPN/caravel-opnsense
abi: "$ABI"
arch: "$ARCH"
prefix: /usr/local
licenselogic: single
licenses: [APACHE20]
categories: [security]
deps: {
  pharosvpn: {origin: "security/pharosvpn", version: "$VER"}
}
desc: <<EOD
$(cat "$ROOT/plugin/pkg-descr")
EOD
scripts: {
  post-install: "/usr/local/etc/rc.d/configd restart >/dev/null 2>&1 || true"
}
EOF

( cd "$PSTAGE" && find usr -type f | sed 's,^,/,' ) > "$WORK/os-pharosvpn.plist"

pkg create -M "$WORK/os-pharosvpn.manifest" -p "$WORK/os-pharosvpn.plist" -r "$PSTAGE" -o "$OUT"
echo "built: $OUT/os-pharosvpn-$PVER.pkg"

echo
echo "artifacts in $OUT:"
ls -la "$OUT"
