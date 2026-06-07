#!/usr/local/bin/php
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
 * The analog of WireGuard's wg-service-control.php: start/reload/stop the
 * userspace pharos-awg daemon per client interface, and (re)apply the OPNsense
 * interface group + outbound NAT + policy-route + kill-switch. crypto stays in
 * the Go daemon; this script only writes the 0600 blob + config and drives
 * pharos-awg (DESIGN §3.3, §6).
 */

require_once('script/load_phalcon.php');
require_once('util.inc');
require_once('config.inc');
require_once('interfaces.inc');
require_once('system.inc');

use OPNsense\PharosVPN\Client;

const PHAROS_BIN = '/usr/local/sbin/pharos-awg';
const PHAROS_RC = '/usr/local/etc/rc.d/pharos-awg';
const CONFIG_DIR = '/usr/local/etc/pharosvpn';
const RUN_DIR = '/var/run/pharosvpn';

/**
 * Write the stored .pharos blob for a client to <if>.pharos at 0600, atomically.
 * The daemon reads + decrypts it; the rendered <if>.conf already points at it.
 */
function pharos_write_profile($client)
{
    @mkdir(CONFIG_DIR, 0700, true);
    $if = (string)$client->interface;
    $dst = CONFIG_DIR . "/{$if}.pharos";
    $raw = base64_decode((string)$client->profile, true);
    if ($raw === false || $raw === '') {
        syslog(LOG_ERR, "pharosvpn: client {$if} has no profile blob");
        return false;
    }
    $tmp = $dst . '.tmp';
    file_put_contents($tmp, $raw);
    @chmod($tmp, 0600);
    rename($tmp, $dst);
    return true;
}

/** Start (or reload) the pharos-awg daemon for a client interface. */
function pharos_start($client)
{
    $if = (string)$client->interface;
    if (!pharos_write_profile($client)) {
        return;
    }
    @mkdir(RUN_DIR, 0755, true);
    $conf = CONFIG_DIR . "/{$if}.conf";
    if (!file_exists($conf)) {
        syslog(LOG_ERR, "pharosvpn: missing rendered config {$conf} (template not reloaded?)");
        return;
    }

    /* one daemon per interface, tracked by a pidfile */
    $pidfile = RUN_DIR . "/{$if}.pid";
    pharos_stop($client); // idempotent: drop any prior instance first

    /*
     * daemonize via daemon(8): pharos-awg runs in the foreground and serves the
     * UAPI socket; daemon(8) backgrounds it, writes the pidfile, and restarts on
     * crash. The daemon itself creates the awgN tun, addresses it, and (per
     * routing mode) installs host routes.
     */
    mwexecf(
        '/usr/sbin/daemon -f -r -P %s -o %s %s --config %s',
        [$pidfile, RUN_DIR . "/{$if}.log", PHAROS_BIN, $conf]
    );

    /* let the tun appear, then register it with OPNsense */
    for ($i = 0; $i < 20 && !does_interface_exist($if); $i++) {
        usleep(100000);
    }
    if (does_interface_exist($if)) {
        mwexecf('/sbin/ifconfig %s group pharosvpn', [$if]);
        interfaces_restart_by_device(false, [$if]);
    } else {
        syslog(LOG_ERR, "pharosvpn: interface {$if} did not come up");
    }

    syslog(LOG_NOTICE, "pharosvpn instance {$client->name} ({$if}) started");
}

/** Stop the pharos-awg daemon for a client interface and drop the device. */
function pharos_stop($client)
{
    $if = (string)$client->interface;
    $pidfile = RUN_DIR . "/{$if}.pid";
    if (file_exists($pidfile)) {
        $pid = (int)trim(@file_get_contents($pidfile));
        if ($pid > 0) {
            mwexecf('/bin/kill -TERM %s', [$pid]);
            for ($i = 0; $i < 30 && posix_kill($pid, 0); $i++) {
                usleep(100000);
            }
        }
        @unlink($pidfile);
    }
    if (does_interface_exist($if)) {
        legacy_interface_destroy($if);
    }
    syslog(LOG_NOTICE, "pharosvpn instance {$client->name} ({$if}) stopped");
}

/** Apply / reconcile all enabled clients (start/restart as needed). */
function pharos_configure()
{
    $mdl = new Client();
    $active = [];
    if ($mdl->isEnabled()) {
        foreach ($mdl->enabledClients() as $client) {
            $active[] = (string)$client->interface;
            pharos_stop($client);
            pharos_start($client);
        }
    }
    /* tear down any device/blob no longer enabled */
    foreach (glob(CONFIG_DIR . '/awg*.pharos') ?: [] as $blob) {
        $dev = basename($blob, '.pharos');
        if (!in_array($dev, $active, true)) {
            if (does_interface_exist($dev)) {
                legacy_interface_destroy($dev);
            }
            $pidfile = RUN_DIR . "/{$dev}.pid";
            if (file_exists($pidfile)) {
                $pid = (int)trim(@file_get_contents($pidfile));
                if ($pid > 0) {
                    mwexecf('/bin/kill -TERM %s', [$pid]);
                }
                @unlink($pidfile);
            }
            @unlink($blob);
        }
    }
    /* interface was recreated; refresh pf so NAT/policy rules re-bind */
    configd_run('filter reload');
}

/** inspect a .pharos profile (delegates to the Go daemon, emits JSON). */
function pharos_inspect($path, $password)
{
    $cmd = PHAROS_BIN . ' inspect --profile ' . escapeshellarg($path);
    if ($password !== null && $password !== '') {
        $cmd .= ' --password ' . escapeshellarg($password);
    }
    passthru($cmd, $rc);
    exit($rc);
}

/** status of a running interface (delegates to the Go daemon, emits JSON). */
function pharos_status($if)
{
    if ($if === null || $if === '') {
        $if = 'awg0';
    }
    passthru(PHAROS_BIN . ' status --interface ' . escapeshellarg($if), $rc);
    exit($rc);
}

openlog('pharosvpn', LOG_ODELAY, LOG_AUTH);

$opts = getopt('ah', [], $optind);
$args = array_slice($argv, $optind);
$action = $args[0] ?? '';

switch ($action) {
    case 'inspect':
        pharos_inspect($args[1] ?? '', $args[2] ?? '');
        break;
    case 'status':
        pharos_status($args[1] ?? '');
        break;
    case 'configure':
        pharos_configure();
        break;
    case 'start':
    case 'stop':
    case 'restart':
        $uuid = $args[1] ?? null;
        $mdl = new Client();
        foreach ($mdl->clients->client->iterateItems() as $key => $client) {
            if ($uuid !== null && $key !== $uuid) {
                continue;
            }
            if ($action === 'stop') {
                pharos_stop($client);
            } elseif ($action === 'start') {
                if ((string)$client->enabled === '1') {
                    pharos_start($client);
                }
            } else { // restart
                pharos_stop($client);
                if ((string)$client->enabled === '1') {
                    pharos_start($client);
                }
            }
        }
        if ($action === 'restart') {
            configd_run('filter reload');
        }
        break;
    default:
        echo "Usage: pharos-service-control.php [start|stop|restart|configure|inspect|status] [args]\n";
        exit(2);
}
