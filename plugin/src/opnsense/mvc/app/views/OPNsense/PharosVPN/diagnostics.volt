{#
 # Copyright (C) 2026 The PharosVPN Authors
 # All rights reserved.
 #
 # Redistribution and use in source and binary forms, with or without modification,
 # are permitted provided that the following conditions are met:
 #
 # 1. Redistributions of source code must retain the above copyright notice,
 #    this list of conditions and the following disclaimer.
 # 2. Redistributions in binary form must reproduce the above copyright notice,
 #    this list of conditions and the following disclaimer in the documentation
 #    and/or other materials provided with the distribution.
 #
 # THIS SOFTWARE IS PROVIDED ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES.
 #}

<script>
    $(document).ready(function () {
        $("#grid-sessions").UIBootgrid({
            search: '/api/pharosvpn/service/status',
            options: {
                multiSelect: false,
                rowSelect: false,
                selection: false,
                formatters: {
                    bytes: function (column, row) {
                        if (row[column.id] && row[column.id] > 0) {
                            return byteFormat(row[column.id], 2);
                        }
                        return row[column.id];
                    },
                    seconds: function (column, row) {
                        if (row[column.id] !== null && row[column.id] !== undefined) {
                            return row[column.id] + "s";
                        }
                        return '';
                    },
                    status: function (column, row) {
                        if (row.type === 'peer' && row['peer-status'] === 'stale') {
                            return '<span class="fa fa-question-circle fa-fw" data-toggle="tooltip" title="{{ lang._('Stale') }}"></span>';
                        }
                        if ((row.type === 'interface' && row.status === 'up') ||
                            (row.type === 'peer' && row['peer-status'] === 'online')) {
                            return '<span class="fa fa-check-circle fa-fw text-success" data-toggle="tooltip" title="{{ lang._('Online') }}"></span>';
                        }
                        return '<span class="fa fa-times-circle fa-fw text-danger" data-toggle="tooltip" title="{{ lang._('Offline') }}"></span>';
                    }
                }
            }
        });
        $("#grid-sessions").on('loaded.rs.jquery.bootgrid', function () {
            $('[data-toggle="tooltip"]').tooltip();
        });
    });
</script>

<div class="tab-content content-box">
    <table id="grid-sessions" class="table table-condensed table-hover table-striped table-responsive">
        <thead>
            <tr>
                <th data-column-id="status" data-formatter="status" data-type="string" data-width="6em">{{ lang._('Status') }}</th>
                <th data-column-id="if" data-type="string" data-width="6em">{{ lang._('Device') }}</th>
                <th data-column-id="name" data-type="string">{{ lang._('Name') }}</th>
                <th data-column-id="type" data-type="string" data-width="6em">{{ lang._('Type') }}</th>
                <th data-column-id="endpoint" data-type="string">{{ lang._('Endpoint dialed') }}</th>
                <th data-column-id="latest-handshake-age" data-formatter="seconds" data-type="numeric">{{ lang._('Handshake Age') }}</th>
                <th data-column-id="transfer-tx" data-formatter="bytes" data-type="numeric">{{ lang._('Sent') }}</th>
                <th data-column-id="transfer-rx" data-formatter="bytes" data-type="numeric">{{ lang._('Received') }}</th>
            </tr>
        </thead>
        <tbody></tbody>
    </table>
</div>
