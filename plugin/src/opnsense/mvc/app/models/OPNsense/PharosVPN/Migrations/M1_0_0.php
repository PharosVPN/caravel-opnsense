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

namespace OPNsense\PharosVPN\Migrations;

use OPNsense\Base\BaseModelMigration;

/**
 * Initial schema. Nothing to migrate from — this is the first version of the
 * PharosVPN client model. Present so the model versioning is established and
 * future migrations have a baseline.
 */
class M1_0_0 extends BaseModelMigration
{
    public function run($model)
    {
        // no-op: baseline schema
    }
}
