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
 *
 * Plugin hooks OPNsense auto-discovers for the PharosVPN client. Mirrors the
 * in-core WireGuard plugin (wireguard.inc): a Services-widget service, a virtual
 * interface group so pf rules can target the tunnel(s), device declaration +
 * creation, and the reconfigure hooks for the 'vpn' event (the persistence path).
 */

function pharosvpn_enabled()
{
    return (string)(new \OPNsense\PharosVPN\Client())->general->enabled == '1';
}

/**
 * Register a service per enabled client for the Services widget, wiring its
 * start/restart/stop to the configd actions.
 */
function pharosvpn_services()
{
    $services = [];
    if (!pharosvpn_enabled()) {
        return $services;
    }
    foreach ((new OPNsense\PharosVPN\Client())->clients->client->iterateItems() as $key => $client) {
        if (!empty((string)$client->enabled)) {
            $services[] = [
                'description' => 'PharosVPN ' . htmlspecialchars((string)$client->name),
                'configd' => [
                    'start' => ["pharosvpn start {$key}"],
                    'restart' => ["pharosvpn restart {$key}"],
                    'stop' => ["pharosvpn stop {$key}"],
                ],
                'name' => 'pharosvpn',
                'nocheck' => true,
                'id' => $key,
            ];
        }
    }
    return $services;
}

/**
 * Register a virtual interface group 'pharosvpn' so firewall rules can target the
 * tunnel interfaces by group, surviving interface renumbering.
 */
function pharosvpn_interfaces()
{
    $interfaces = [];
    if (!pharosvpn_enabled()) {
        return $interfaces;
    }
    $interfaces['pharosvpn'] = [
        'descr' => gettext('PharosVPN (Group)'),
        'if' => 'pharosvpn',
        'virtual' => true,
        'enable' => true,
        'type' => 'group',
        'networks' => [],
    ];
    return $interfaces;
}

/**
 * Declare the awg devices (^awg) and how to (empty-)create them. The actual tun
 * is created by the userspace pharos-awg daemon; here we only register so
 * OPNsense knows the device class + pattern for interface assignment.
 */
function pharosvpn_devices()
{
    $names = [];
    foreach ((new OPNsense\PharosVPN\Client())->clients->client->iterateItems() as $client) {
        if (!empty((string)$client->enabled)) {
            $if = (string)$client->interface;
            $names[$if] = [
                'descr' => sprintf('%s (PharosVPN - %s)', $if, (string)$client->name),
                'ifdescr' => (string)$client->name,
                'name' => $if,
            ];
        }
    }
    return [[
        'function' => 'pharosvpn_prepare',
        'configurable' => false,
        'pattern' => '^awg',
        'type' => 'pharosvpn',
        'volatile' => true,
        'names' => $names,
    ]];
}

/**
 * Prepare an awg device: ensure the daemon-created tun is in the pharosvpn group.
 * Unlike WireGuard (which can `ifconfig wg create`), the awg tun is created by
 * the userspace daemon, so here we only group it if it already exists. Full
 * (re)creation happens via pharosvpn_configure_do() → configd 'pharosvpn
 * configure', which starts the daemon.
 */
function pharosvpn_prepare($device)
{
    foreach ((new OPNsense\PharosVPN\Client())->clients->client->iterateItems() as $client) {
        if ($device != (string)$client->interface) {
            continue;
        }
        if (does_interface_exist($device)) {
            mwexecf('/sbin/ifconfig %s group pharosvpn', [$device]);
        }
        return $device;
    }
    return null;
}

/**
 * Reconfigure hooks: on the 'vpn' event (and a WAN IP change) reconcile the
 * tunnels — this is the persistence-across-reboot path. configd 'pharosvpn
 * configure' (re)creates the tun, restarts the daemon, and re-applies routing.
 */
function pharosvpn_configure()
{
    return [
        'newwanip' => ['pharosvpn_configure_do'],
        'vpn' => ['pharosvpn_configure_do'],
    ];
}

function pharosvpn_configure_do($verbose = false)
{
    if (!pharosvpn_enabled()) {
        return;
    }
    service_log('Configuring PharosVPN...', $verbose);
    configd_run('pharosvpn configure');
    service_log("done.\n", $verbose);
}

/**
 * Syslog facility registration so the daemon's messages land in their own log.
 */
function pharosvpn_syslog()
{
    return [
        'pharosvpn' => ['facility' => ['pharosvpn']],
    ];
}

/**
 * xmlrpc/HA sync descriptor (so a CARP peer receives the config). Tunnel state
 * itself is recreated locally from the synced model.
 */
function pharosvpn_xmlrpc_sync()
{
    $result = [];
    $result['id'] = 'pharosvpn';
    $result['section'] = 'OPNsense.PharosVPN';
    $result['description'] = gettext('PharosVPN');
    $result['services'] = ['pharosvpn'];
    return [$result];
}
