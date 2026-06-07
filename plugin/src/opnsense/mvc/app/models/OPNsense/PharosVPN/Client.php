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
 * THIS SOFTWARE IS PROVIDED ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES,
 * INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY
 * AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE
 * AUTHOR BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY,
 * OR CONSEQUENTIAL DAMAGES.
 */

namespace OPNsense\PharosVPN;

use OPNsense\Base\BaseModel;

/**
 * The PharosVPN client model: a master enable plus a list of importable
 * connections (clients.client), each carrying an encrypted-at-rest .pharos blob
 * and the routing/firewall choices for it. Crypto stays in the pharos-awg
 * daemon (DESIGN §6) — this model never decrypts a profile.
 */
class Client extends BaseModel
{
    /** Directory holding the rendered 0600 daemon configs and profile blobs. */
    public const CONFIG_DIR = '/usr/local/etc/pharosvpn';

    /** Whether the master switch is on. */
    public function isEnabled(): bool
    {
        return (string)$this->general->enabled === '1';
    }

    /**
     * Iterate the enabled client connections, keyed by uuid.
     * @return \Generator
     */
    public function enabledClients()
    {
        foreach ($this->clients->client->iterateItems() as $key => $client) {
            if ((string)$client->enabled === '1') {
                yield $key => $client;
            }
        }
    }

    /** Rendered daemon config path for an interface, e.g. awg0.conf. */
    public static function cnfFilename(string $interface): string
    {
        return self::CONFIG_DIR . '/' . $interface . '.conf';
    }

    /** Stored .pharos blob path for an interface, e.g. awg0.pharos. */
    public static function profileFilename(string $interface): string
    {
        return self::CONFIG_DIR . '/' . $interface . '.pharos';
    }
}
