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
 * The PharosVPN diagnostics page: dumps the running daemon's UAPI socket
 * (handshake age, RX/TX, endpoint dialed) — the analog of WireGuard's status.
 */
class DiagnosticsController extends \OPNsense\Base\IndexController
{
    protected function templateJSIncludes()
    {
        $result = parent::templateJSIncludes();
        $result[] = '/ui/js/moment-with-locales.min.js';
        return $result;
    }

    public function indexAction()
    {
        $this->view->pick('OPNsense/PharosVPN/diagnostics');
    }
}
