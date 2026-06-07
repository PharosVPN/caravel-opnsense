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

/**
 * The general settings section (master enable + account-mode signer/device key).
 */
class GeneralController extends ApiMutableModelControllerBase
{
    protected static $internalModelClass = '\OPNsense\PharosVPN\Client';
    protected static $internalModelName = 'general';

    public function getAction()
    {
        return ['general' => $this->getModel()->general->getNodes()];
    }

    public function setAction()
    {
        $result = ['result' => 'failed'];
        if ($this->request->isPost()) {
            $mdl = $this->getModel();
            $mdl->general->setNodes($this->request->getPost('general'));
            $result = $this->validateAndSave($mdl, 'general');
        }
        return $result;
    }
}
