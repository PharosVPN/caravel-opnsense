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

namespace OPNsense\PharosVPN\Api;

use OPNsense\Base\ApiMutableServiceControllerBase;
use OPNsense\Core\Backend;
use OPNsense\PharosVPN\Client;

/**
 * Lifecycle + live status for the PharosVPN client. reconfigure renders the
 * daemon config from the model and (re)applies the tunnel + pf plumbing;
 * connect/disconnect toggle the chosen client and reconfigure; status reads the
 * running UAPI socket(s) for the diagnostics grid.
 */
class ServiceController extends ApiMutableServiceControllerBase
{
    protected static $internalServiceClass = '\OPNsense\PharosVPN\Client';
    protected static $internalServiceTemplate = 'OPNsense/PharosVPN';
    protected static $internalServiceEnabled = 'general.enabled';
    protected static $internalServiceName = 'pharosvpn';

    /**
     * Render the template and (re)apply every enabled client's tunnel + routing.
     */
    public function reconfigureAction()
    {
        if (!$this->request->isPost()) {
            return ['status' => 'failed'];
        }
        $backend = new Backend();
        $backend->configdRun('interface invoke registration');
        $backend->configdRun('template reload ' . escapeshellarg(static::$internalServiceTemplate));
        $backend->configdpRun('pharosvpn configure');
        return ['status' => 'ok'];
    }

    /**
     * Enable the given client (disabling the others — single awg0 MVP), persist,
     * and reconfigure so the tunnel comes up.
     */
    public function connectAction($uuid = null)
    {
        if (!$this->request->isPost()) {
            return ['status' => 'failed'];
        }
        return $this->setSelected($uuid, true);
    }

    /** Disable the given client (or all) and reconfigure so the tunnel drops. */
    public function disconnectAction($uuid = null)
    {
        if (!$this->request->isPost()) {
            return ['status' => 'failed'];
        }
        return $this->setSelected($uuid, false);
    }

    private function setSelected($uuid, bool $on): array
    {
        $mdl = new Client();
        $found = false;
        foreach ($mdl->clients->client->iterateItems() as $key => $client) {
            if ($uuid !== null && $key === $uuid) {
                $client->enabled = $on ? '1' : '0';
                $found = true;
            } elseif ($on && $uuid !== null) {
                // single-tunnel MVP: only one client active at a time.
                $client->enabled = '0';
            } elseif ($uuid === null && !$on) {
                $client->enabled = '0';
                $found = true;
            }
        }
        if ($on && !$found) {
            return ['status' => 'failed', 'message' => gettext('Unknown client.')];
        }
        // Turning a client on requires the master switch on, too.
        if ($on) {
            $mdl->general->enabled = '1';
        }
        $mdl->serializeToConfig();
        \OPNsense\Core\Config::getInstance()->save();

        $backend = new Backend();
        $backend->configdRun('template reload ' . escapeshellarg(static::$internalServiceTemplate));
        $backend->configdpRun('pharosvpn configure');
        return ['status' => 'ok'];
    }

    /**
     * Live UAPI status for the diagnostics grid: one row per peer across all
     * configured interfaces, with handshake age + RX/TX. Reads the daemon's
     * `pharosvpn status <if>` JSON (the analog of WireGuard's `wg show dump`).
     */
    public function statusAction()
    {
        $records = [];
        $now = time();
        $backend = new Backend();
        foreach ((new Client())->clients->client->iterateItems() as $client) {
            $if = (string)$client->interface;
            if ($if === '') {
                continue;
            }
            $out = $backend->configdRun('pharosvpn status ' . escapeshellarg($if));
            $st = json_decode((string)$out, true);
            if (!is_array($st)) {
                continue;
            }
            $up = !empty($st['up']);
            if (empty($st['peers'])) {
                $records[] = [
                    'if' => $if,
                    'name' => (string)$client->name,
                    'type' => 'interface',
                    'status' => $up ? 'up' : 'down',
                    'endpoint' => '',
                    'latest-handshake-age' => null,
                    'transfer-rx' => 0,
                    'transfer-tx' => 0,
                    'peer-status' => $up ? 'online' : 'offline',
                ];
                continue;
            }
            foreach ($st['peers'] as $peer) {
                $age = isset($peer['handshake_age']) ? (int)$peer['handshake_age'] : null;
                $peerStatus = 'offline';
                if ($age !== null) {
                    $peerStatus = $age <= 300 ? 'online' : 'stale';
                }
                $records[] = [
                    'if' => $if,
                    'name' => (string)$client->name,
                    'type' => 'peer',
                    'status' => $up ? 'up' : 'down',
                    'public-key' => $peer['public_key'] ?? '',
                    'endpoint' => $peer['endpoint'] ?? '',
                    'latest-handshake-age' => $age,
                    'latest-handshake-epoch' => !empty($peer['latest_handshake'])
                        ? date('Y-m-d H:i:s', (int)$peer['latest_handshake']) : null,
                    'transfer-rx' => (int)($peer['transfer_rx'] ?? 0),
                    'transfer-tx' => (int)($peer['transfer_tx'] ?? 0),
                    'peer-status' => $peerStatus,
                ];
            }
        }
        return $this->searchRecordsetBase($records);
    }
}
