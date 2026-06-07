<?php

/*
 * Copyright (C) 2026 The PharosVPN Authors
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 *
 * 1. Redistributions of source code must retain the above copyright notice,
 *    this list of conditions and the following disclaimer.
 *
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES.
 */

namespace OPNsense\PharosVPN;

/**
 * Client-mode LAN routing: the firewall/routing layer that turns "the tunnel is
 * up" into "the selected LAN sources actually egress through it". Two halves:
 *
 *  - {@see registerFilterRules()} runs from the pharosvpn_firewall($fw) plugin
 *    hook on every filter reload. It (re)generates, per enabled client, an
 *    outbound-NAT (masquerade) rule, a policy-route pass rule to the per-client
 *    gateway, and (optionally) a kill-switch block — all derived from the model,
 *    so they are idempotent (regenerated each reload, never duplicated) and
 *    reversible (gone the moment the client is disabled). Nothing is hand-rolled
 *    into a raw pf anchor; these go through OPNsense's own Firewall\Plugin and
 *    show up in pfctl + the firewall log tagged PharosVPN.
 *
 *  - {@see syncGateways()} / {@see removeGateway()} keep the per-client
 *    Routing\Gateways entry (PharosVPN_GW) in sync with the running tunnel, so
 *    policy-based routing + the dashboard gateway health work. The gateway is
 *    bound to the awg device with the live tunnel address as its far gateway.
 *
 * SECURITY (DESIGN §7): only the explicitly selected lan_sources are ever
 * policy-routed. The box's own management / coxswain / control traffic stays on
 * the normal default route — we never touch it, so the management path can't be
 * black-holed.
 */
class FirewallRules
{
    /** Firewall category + rule-description prefix so plugin rules are identifiable in the GUI/log. */
    public const TAG = 'PharosVPN';

    /** Priority band for the generated filter rules (after core, like IPsec's 500000). */
    private const FILTER_PRIO = 400000;

    /** Priority band for the generated outbound-NAT rules (matches manual outbound NAT, 100). */
    private const SNAT_PRIO = 100;

    /** The virtual interface group the awg devices join (pharosvpn_interfaces()). */
    public const GROUP = 'pharosvpn';

    /**
     * Stable gateway name for a client interface, e.g. awg0 -> PharosVPN_AWG0.
     * One gateway per tunnel; 32-char limit per the Gateways model name mask.
     */
    public static function gatewayName(string $interface): string
    {
        return 'PharosVPN_' . strtoupper(preg_replace('/[^a-zA-Z0-9]/', '', $interface));
    }

    /** Path of the per-interface routes state file (tunnel AllowedIPs, for split mode). */
    public static function routesStateFile(string $interface): string
    {
        return '/var/run/pharosvpn/' . $interface . '.routes';
    }

    /**
     * Split lan_sources (NetworkField AsList: comma/space separated) into a clean
     * list of CIDRs/hosts. Empty -> []: nothing is routed (the safe default —
     * DESIGN §7).
     */
    public static function lanSources($client): array
    {
        $raw = (string)$client->lan_sources;
        $out = [];
        foreach (preg_split('/[\s,]+/', $raw) as $src) {
            $src = trim($src);
            if ($src !== '') {
                $out[] = $src;
            }
        }
        return $out;
    }

    /**
     * Whether OPNsense's outbound-NAT is in a mode where we must add our own SNAT
     * rule. In automatic/hybrid mode OPNsense auto-NATs known internal networks
     * but NOT a foreign tunnel interface, so we add the masquerade ourselves. In
     * 'advanced' mode the admin owns all outbound NAT; we stay out of it and the
     * GUI note tells them to add the rule.
     */
    public static function shouldManageOutboundNat(): bool
    {
        $cfg = \OPNsense\Core\Config::getInstance()->object();
        $mode = (string)($cfg->nat->outbound->mode ?? 'automatic');
        return $mode !== 'advanced';
    }

    /**
     * Register the per-client NAT + policy-route + kill-switch rules into the
     * firewall plugin. Called from pharosvpn_firewall($fw). Generates nothing
     * (and so removes nothing-but-itself) when the plugin/client is disabled or
     * no lan_sources are selected.
     *
     * @param \OPNsense\Firewall\Plugin $fw the live firewall rule builder
     */
    public static function registerFilterRules($fw): void
    {
        $mdl = new Client();
        if (!$mdl->isEnabled()) {
            return;
        }
        $manageNat = self::shouldManageOutboundNat();
        foreach ($mdl->enabledClients() as $client) {
            $if = (string)$client->interface;
            $sources = self::lanSources($client);
            if ($if === '' || empty($sources)) {
                // No interface or no selected sources -> nothing to route. The box's
                // own traffic is never touched.
                continue;
            }
            $gwName = self::gatewayName($if);
            $routing = (string)$client->routing;
            $label = self::TAG . ': ' . (string)$client->name;

            // Live tunnel state (address + AllowedIPs), written to the routes state
            // file by the service-control script once the tunnel is up. The NAT
            // target needs the tunnel address explicitly: we NAT on the pharosvpn
            // interface *group* (an ifgroup has no :0 address), so we must hand pf
            // a concrete translation address. Skip NAT until the tunnel is up; the
            // post-connect filter reload re-runs this hook with the address present.
            $tunAddr = self::tunnelAddress($if);

            // For split tunnel, scope the policy-route to the tunnel's AllowedIPs
            // (from the same state file). Full tunnel (or an unknown/down tunnel)
            // routes everything via the gateway.
            $destinations = ['any'];
            if ($routing === 'split') {
                $allowed = self::splitDestinations($if);
                if (!empty($allowed)) {
                    $destinations = $allowed;
                }
            }

            foreach ($sources as $src) {
                $ipproto = strpos($src, ':') !== false ? 'inet6' : 'inet';

                // 1) Outbound NAT: masquerade this source out the tunnel group so
                //    return traffic from the node has a way back. Registered only
                //    when the tunnel address is known and outbound NAT is
                //    automatic/hybrid. NOTE: pf can only load `nat on pharosvpn`
                //    once the group has a live member (the daemon-created awg
                //    device), i.e. when the tunnel is actually up — which is
                //    exactly when NAT is needed; with no tunnel there is nothing
                //    to NAT. The post-connect filter reload materialises it.
                if ($manageNat && $ipproto === 'inet' && $tunAddr !== '') {
                    $fw->registerSNatRule(self::SNAT_PRIO, [
                        'interface' => self::GROUP,
                        'ipprotocol' => $ipproto,
                        'from' => $src,
                        'target' => $tunAddr,
                        'descr' => $label . ' outbound NAT',
                        '#ref' => 'ui/pharosvpn',
                    ]);
                }

                // 2) Policy route: pass the selected source to the tunnel gateway
                //    (route-to). quick so it wins over the default allow, but only
                //    for this source — the box's own traffic is unaffected. Full
                //    tunnel = all destinations; split = the tunnel's AllowedIPs.
                foreach ($destinations as $dst) {
                    self::passToGateway($fw, $src, $dst, $ipproto, $gwName, $label);
                }

                // 3) Kill-switch: when enabled, a floating block on the source so
                //    that if the tunnel/gateway is down (the route-to pass is then
                //    skipped and creates no state), the source fails closed instead
                //    of leaking to the WAN. While the tunnel is up the routed flow
                //    is passed by the policy-route's keep-state and never hits this.
                if ((string)$client->killswitch === '1') {
                    self::killSwitch($fw, $src, $ipproto, $label);
                }
            }
        }
    }

    /**
     * Register the policy-route pass rule. Inbound on the LAN-side interfaces the
     * source can arrive on; we do not name a single LAN here (multi-LAN firewalls
     * exist) — pf matches the source address and route-to sends it out the tunnel
     * gateway. direction in + quick so it takes priority over the default allow.
     */
    private static function passToGateway($fw, string $src, string $dst, string $ipproto, string $gwName, string $label): void
    {
        $fw->registerFilterRule(self::FILTER_PRIO, [
            'direction' => 'in',
            'quick' => true,
            'ipprotocol' => $ipproto,
            'from' => $src,
            'to' => $dst,
            'gateway' => $gwName,
            'type' => 'pass',
            'statetype' => 'keep',
            'descr' => $label . ' policy route',
            '#ref' => 'ui/pharosvpn',
        ], [
            'log' => false,
        ]);
    }

    /**
     * Register the kill-switch block (fail closed). A floating `block out quick`
     * on the selected source. While the tunnel is up, the policy-route pass above
     * matches the connection `in` on the LAN and creates state (keep state), so
     * the egress packets are passed by that state and never re-evaluated against
     * this block. When the tunnel/gateway is DOWN, pf skips the route-to pass
     * (skip_rules_gw_down), no state is created, and this block then catches the
     * source — so it fails closed instead of leaking out the WAN.
     *
     * Deliberately not bound to a single interface: a floating rule loads in all
     * states (even before the tunnel device exists), unlike an `on <tunnel>` rule
     * that can't resolve until the daemon has created + grouped the awg device.
     */
    private static function killSwitch($fw, string $src, string $ipproto, string $label): void
    {
        $fw->registerFilterRule(self::FILTER_PRIO + 1, [
            'direction' => 'out',
            'quick' => true,
            'ipprotocol' => $ipproto,
            'from' => $src,
            'to' => 'any',
            'type' => 'block',
            'descr' => $label . ' kill-switch',
            '#ref' => 'ui/pharosvpn',
        ], [
            'log' => true,
        ]);
    }

    /**
     * Read the tunnel's AllowedIPs (written by the service-control script at
     * connect time) for split-mode destinations. Returns [] if unknown.
     */
    public static function splitDestinations(string $interface): array
    {
        $file = self::routesStateFile($interface);
        if (!is_file($file)) {
            return [];
        }
        $data = json_decode((string)@file_get_contents($file), true);
        if (!is_array($data) || empty($data['allowed_ips'])) {
            return [];
        }
        $out = [];
        foreach ((array)$data['allowed_ips'] as $cidr) {
            $cidr = trim((string)$cidr);
            // A split tunnel never claims the default route via policy routing.
            if ($cidr !== '' && $cidr !== '0.0.0.0/0' && $cidr !== '::/0') {
                $out[] = $cidr;
            }
        }
        return $out;
    }

    /**
     * The live tunnel (far) address, used as the gateway far-IP and the NAT
     * translation target. Read from the routes state file (written at connect
     * time); '' if the tunnel is down / not yet recorded.
     */
    public static function tunnelAddress(string $interface): string
    {
        $file = self::routesStateFile($interface);
        if (!is_file($file)) {
            return '';
        }
        $data = json_decode((string)@file_get_contents($file), true);
        $addr = is_array($data) ? trim((string)($data['address'] ?? '')) : '';
        // Strip any /mask: the gateway/NAT target wants a bare address.
        if ($addr !== '' && strpos($addr, '/') !== false) {
            $addr = explode('/', $addr)[0];
        }
        return $addr;
    }

    /**
     * Record the running tunnel's address + AllowedIPs to the routes state file
     * so the firewall hook can build the NAT target, gateway far-IP, and split
     * destinations on the next filter reload. Called by the service-control
     * script after the tunnel is up. Returns the recorded address ('' if none).
     */
    public static function writeRoutesState(string $interface, string $address, array $allowedIps): string
    {
        @mkdir(dirname(self::routesStateFile($interface)), 0755, true);
        $file = self::routesStateFile($interface);
        $payload = json_encode([
            'interface' => $interface,
            'address' => $address,
            'allowed_ips' => array_values($allowedIps),
        ]);
        $tmp = $file . '.tmp';
        @file_put_contents($tmp, $payload);
        @rename($tmp, $file);
        return $address;
    }

    /** Remove the routes state file for an interface (on teardown). */
    public static function clearRoutesState(string $interface): void
    {
        @unlink(self::routesStateFile($interface));
    }

    /**
     * Create or update the per-client gateway (PharosVPN_<IF>) bound to the awg
     * device, with the live tunnel address as its far gateway. This is what makes
     * the policy-route route-to + the dashboard gateway health work. Idempotent:
     * re-applying replaces the same named gateway. Returns true if a gateway was
     * written (i.e. an address was known).
     *
     * NOTE: writes the Routing\Gateways model and persists config; the caller is
     * responsible for the subsequent filter reload.
     */
    public static function syncGateway(string $interface, string $address): bool
    {
        if ($address === '') {
            // No address yet -> can't build a far gateway. Leave any prior one;
            // it will be refreshed on the next connect.
            return false;
        }
        $gateways = new \OPNsense\Routing\Gateways();
        $name = self::gatewayName($interface);
        $uuid = self::findGatewayUuid($gateways, $name);
        $gateways->createOrUpdateGateway([
            'name' => $name,
            'interface' => $interface, // raw device; getRealInterface passes it through
            'ipprotocol' => 'inet',
            'gateway' => $address,
            'fargw' => '1',            // far gateway (address not on a directly-attached subnet)
            'monitor_disable' => '1',  // no ICMP monitor: a userspace tunnel needn't be pinged
            'priority' => '255',       // least-attractive: never becomes the system default GW
            'descr' => self::TAG . ' tunnel gateway (' . $interface . ')',
        ], $uuid);
        \OPNsense\Core\Config::getInstance()->save();
        return true;
    }

    /**
     * Remove the per-client gateway on teardown. Idempotent: a no-op if absent.
     * Persists config; the caller reloads the filter afterwards.
     */
    public static function removeGateway(string $interface): void
    {
        $gateways = new \OPNsense\Routing\Gateways();
        $name = self::gatewayName($interface);
        $uuid = self::findGatewayUuid($gateways, $name);
        if ($uuid !== null) {
            $gateways->gateway_item->del($uuid);
            $gateways->serializeToConfig(false, true);
            \OPNsense\Core\Config::getInstance()->save();
        }
    }

    /** Find the uuid of our named gateway in the Gateways model, or null. */
    private static function findGatewayUuid(\OPNsense\Routing\Gateways $gateways, string $name): ?string
    {
        foreach ($gateways->gateway_item->iterateItems() as $key => $item) {
            if ((string)$item->name === $name) {
                return (string)$key;
            }
        }
        return null;
    }
}
