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

use OPNsense\Base\ApiMutableModelControllerBase;
use OPNsense\Core\Backend;

/**
 * CRUD for client connections plus the PharosVPN-specific actions: import a
 * .pharos blob and inspect its resolved (non-secret) metadata. All profile
 * parsing/decryption happens in the pharos-awg Go daemon via configd — PHP only
 * stores the (already-encrypted-at-rest) blob and renders the daemon's JSON
 * (DESIGN §3.3, §6).
 */
class ClientController extends ApiMutableModelControllerBase
{
    protected static $internalModelName = 'client';
    protected static $internalModelClass = '\OPNsense\PharosVPN\Client';

    public function searchClientAction()
    {
        return $this->searchBase('clients.client', ['enabled', 'name', 'interface', 'routing']);
    }

    public function getClientAction($uuid = null)
    {
        return $this->getBase('client', 'clients.client', $uuid);
    }

    public function addClientAction()
    {
        return $this->addBase('client', 'clients.client');
    }

    public function delClientAction($uuid)
    {
        return $this->delBase('clients.client', $uuid);
    }

    public function setClientAction($uuid = null)
    {
        return $this->setBase('client', 'clients.client', $uuid);
    }

    public function toggleClientAction($uuid)
    {
        return $this->toggleBase('clients.client', $uuid);
    }

    /**
     * Import a .pharos profile blob into a (new or existing) client. Accepts a
     * base64-encoded blob and, optionally, a password (password-mode profile).
     * Stores the blob to a temp file, runs `pharosvpn inspect` so the daemon
     * resolves the (non-secret) metadata, and returns it for the GUI to render.
     * The blob itself is returned base64 so the dialog can persist it on save.
     */
    public function importAction()
    {
        if (!$this->request->isPost()) {
            return ['status' => 'failed', 'message' => gettext('POST required')];
        }
        $b64 = (string)$this->request->getPost('profile', 'string', '');
        $password = (string)$this->request->getPost('password', 'string', '');
        $raw = base64_decode($b64, true);
        if ($raw === false || $raw === '') {
            return ['status' => 'failed', 'message' => gettext('No profile data provided.')];
        }

        $tmp = @tempnam(sys_get_temp_dir(), 'pharos');
        if ($tmp === false) {
            return ['status' => 'failed', 'message' => gettext('Could not create temp file.')];
        }
        try {
            @chmod($tmp, 0600);
            file_put_contents($tmp, $raw);
            $meta = $this->runInspect($tmp, $password);
            if (isset($meta['error'])) {
                return ['status' => 'failed', 'message' => $meta['error'], 'meta' => $meta];
            }
            return ['status' => 'ok', 'profile' => base64_encode($raw), 'meta' => $meta];
        } finally {
            @unlink($tmp);
        }
    }

    /**
     * Inspect an already-stored client's profile blob: write it to a temp file
     * and ask the daemon for its metadata. Used to (re)render node choices for a
     * saved client without re-uploading.
     */
    public function inspectAction($uuid = null)
    {
        $node = $this->getModel()->getNodeByReference('clients.client.' . $uuid);
        if ($node === null) {
            return ['status' => 'failed', 'message' => gettext('Unknown client.')];
        }
        $raw = base64_decode((string)$node->profile, true);
        if ($raw === false || $raw === '') {
            return ['status' => 'failed', 'message' => gettext('Client has no imported profile.')];
        }
        $tmp = @tempnam(sys_get_temp_dir(), 'pharos');
        if ($tmp === false) {
            return ['status' => 'failed', 'message' => gettext('Could not create temp file.')];
        }
        try {
            @chmod($tmp, 0600);
            file_put_contents($tmp, $raw);
            $meta = $this->runInspect($tmp, (string)$node->password);
            if (isset($meta['error'])) {
                return ['status' => 'failed', 'message' => $meta['error'], 'meta' => $meta];
            }
            return ['status' => 'ok', 'meta' => $meta];
        } finally {
            @unlink($tmp);
        }
    }

    /**
     * Run the configd `pharosvpn inspect <path> <password>` action and decode the
     * daemon's JSON. The service-control script forwards <path> (and <password>
     * when non-empty) to `pharos-awg inspect`. Always two positional args so the
     * configd action's parameter count is satisfied (empty password = none mode).
     */
    private function runInspect(string $path, string $password): array
    {
        $params = escapeshellarg($path) . ' ' . escapeshellarg($password);
        $out = (new Backend())->configdRun('pharosvpn inspect ' . $params);
        $decoded = json_decode((string)$out, true);
        if (!is_array($decoded)) {
            return ['error' => gettext('Could not parse the profile (daemon returned no metadata).')];
        }
        return $decoded;
    }
}
