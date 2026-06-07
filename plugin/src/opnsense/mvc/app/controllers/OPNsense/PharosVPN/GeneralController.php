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
 * The PharosVPN client GUI: one page with a Profiles tab (import a .pharos, see
 * resolved nodes/path/expiry) and a Connection tab (pick profile, routing,
 * sources, kill-switch, connect/disconnect, live status).
 */
class GeneralController extends \OPNsense\Base\IndexController
{
    public function indexAction()
    {
        $this->view->generalForm = $this->getForm("general");
        $this->view->formDialogEditClient = $this->getForm("dialogEditClient");
        $this->view->formGridClient = $this->getFormGrid("dialogEditClient");
        $this->view->pick('OPNsense/PharosVPN/general');
    }
}
