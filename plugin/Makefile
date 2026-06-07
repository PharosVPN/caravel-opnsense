# SPDX-License-Identifier: Apache-2.0
# os-pharosvpn — OPNsense plugin Makefile.
#
# Standard OPNsense plugin Makefile shape (the helloworld/wireguard template):
# inside an opnsense/plugins checkout this includes ../../Mk/plugins.mk and
# `make package` builds the os-pharosvpn pkg. Outside that tree, use
# ../scripts/build-pkg.sh to hand-assemble the pkg on a FreeBSD box.

PLUGIN_NAME=		pharosvpn
PLUGIN_VERSION=		1.0.0
PLUGIN_COMMENT=		PharosVPN client (caravel) for OPNsense
PLUGIN_DEPENDS=		pharosvpn
PLUGIN_MAINTAINER=	dev@pharosvpn.invalid
PLUGIN_WWW=		https://github.com/PharosVPN/caravel-opnsense

.include "../../Mk/plugins.mk"
