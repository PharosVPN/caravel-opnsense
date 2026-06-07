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
    'use strict';

    $(document).ready(function () {
        const importDialog = '#dialogImportProfile';

        // Settings form (master enable + account-mode keys).
        mapDataToFormUI({ 'frm_general': '/api/pharosvpn/general/get' }).done(function () {
            formatTokenizersUI();
            $('.selectpicker').selectpicker('refresh');
        });

        $('#saveGeneral').click(function () {
            saveFormToEndpoint('/api/pharosvpn/general/set', 'frm_general', function () {
                $('#responseMsg').removeClass('hidden').addClass('alert-info')
                    .html('{{ lang._('Settings saved. Reconfigure to apply.') }}');
            });
        });

        // The Profiles / Connection grid: each row is one importable connection.
        $('#grid-clients').UIBootgrid({
            search: '/api/pharosvpn/client/searchClient',
            get: '/api/pharosvpn/client/getClient/',
            set: '/api/pharosvpn/client/setClient/',
            add: '/api/pharosvpn/client/addClient/',
            del: '/api/pharosvpn/client/delClient/',
            toggle: '/api/pharosvpn/client/toggleClient/',
            options: {
                formatters: {
                    commands: function (column, row) {
                        return '<button type="button" class="btn btn-xs btn-default command-edit bootgrid-tooltip" data-row-id="' + row.uuid + '"><span class="fa fa-pencil fa-fw"></span></button> ' +
                               '<button type="button" class="btn btn-xs btn-default command-copy bootgrid-tooltip" data-row-id="' + row.uuid + '"><span class="fa fa-clone fa-fw"></span></button> ' +
                               '<button type="button" class="btn btn-xs btn-default command-delete bootgrid-tooltip" data-row-id="' + row.uuid + '"><span class="fa fa-trash-o fa-fw"></span></button>';
                    }
                }
            }
        });

        // Reconfigure / connect-disconnect buttons (apply the whole subsystem).
        $('#reconfigureAct').SimpleActionButton();

        // Import flow: upload a .pharos file, base64 it client-side, post to the
        // daemon-backed inspect endpoint, then render the resolved metadata.
        $('#btnImport').click(function () {
            $('#importMsg').addClass('hidden').empty();
            $('#importMeta').addClass('hidden').empty();
            $(importDialog).modal({ backdrop: 'static', keyboard: false });
        });

        $('#doImport').click(function () {
            const fileInput = document.getElementById('profileFile');
            if (!fileInput.files.length) {
                $('#importMsg').removeClass('hidden').text('{{ lang._('Choose a .pharos file first.') }}');
                return;
            }
            const reader = new FileReader();
            reader.onload = function (e) {
                // strip the data: URL prefix → bare base64
                const b64 = e.target.result.split(',').pop();
                ajaxCall('/api/pharosvpn/client/import', {
                    profile: b64,
                    password: $('#importPassword').val()
                }, function (data) {
                    if (data.status === 'ok') {
                        $('#importMsg').removeClass('hidden').removeClass('text-danger')
                            .addClass('text-success').text('{{ lang._('Profile parsed.') }}');
                        renderMeta(data.meta);
                        window._pharosImportedBlob = data.profile;
                    } else {
                        $('#importMsg').removeClass('hidden').addClass('text-danger')
                            .text(data.message || '{{ lang._('Import failed.') }}');
                    }
                });
            };
            reader.readAsDataURL(fileInput.files[0]);
        });

        function renderMeta(meta) {
            if (!meta) { return; }
            let html = '<table class="table table-condensed">';
            html += '<tr><td>{{ lang._('Encryption') }}</td><td>' + (meta.enc || '') + '</td></tr>';
            html += '<tr><td>{{ lang._('Fleet') }}</td><td>' + (meta.fleet_id || '') + '</td></tr>';
            html += '<tr><td>{{ lang._('Expires') }}</td><td>' + (meta.expires_at || '-') +
                    (meta.expired ? ' <span class="text-danger">({{ lang._('expired') }})</span>' : '') + '</td></tr>';
            (meta.profiles || []).forEach(function (p) {
                html += '<tr><td>{{ lang._('Connection') }}</td><td><b>' + p.name + '</b> (' + p.protocol + ')';
                (p.nodes || []).forEach(function (n) {
                    html += '<br>&bull; ' + n.name + ' [' + (n.region || '') + ']' + (n.entry ? ' <i>(entry)</i>' : '');
                });
                if (p.path) {
                    html += '<br>{{ lang._('Path') }}: ' + p.path.hops.map(function (h) { return h.name + ' (' + h.role + ')'; }).join(' &rarr; ');
                }
                html += '</td></tr>';
            });
            html += '</table>';
            $('#importMeta').removeClass('hidden').html(html);
        }
    });
</script>

<ul class="nav nav-tabs" role="tablist" id="maintabs">
    <li class="active"><a data-toggle="tab" href="#profiles">{{ lang._('Profiles') }}</a></li>
    <li><a data-toggle="tab" href="#connection">{{ lang._('Connection') }}</a></li>
    <li><a data-toggle="tab" href="#settings">{{ lang._('Settings') }}</a></li>
</ul>

<div class="tab-content content-box">
    <!-- Profiles / Connection share the client grid; the dialog covers both. -->
    <div id="profiles" class="tab-pane fade in active">
        <div class="alert alert-info" role="alert">
            {{ lang._('Import a .pharos profile, then pick a connection and routing mode. For a multi-hop profile the firewall dials only the entry node.') }}
        </div>
        <button class="btn btn-primary" id="btnImport" type="button">
            <span class="fa fa-upload fa-fw"></span> {{ lang._('Import .pharos') }}
        </button>
        <hr/>
        <table id="grid-clients" class="table table-condensed table-hover table-striped" data-editDialog="DialogEditClient" data-editAlert="responseMsg">
            <thead>
                <tr>
                    <th data-column-id="enabled" data-formatter="boolean" data-width="6em">{{ lang._('Enabled') }}</th>
                    <th data-column-id="name" data-type="string">{{ lang._('Name') }}</th>
                    <th data-column-id="interface" data-type="string">{{ lang._('Interface') }}</th>
                    <th data-column-id="routing" data-type="string">{{ lang._('Routing') }}</th>
                    <th data-column-id="commands" data-width="9em" data-formatter="commands" data-sortable="false">{{ lang._('Commands') }}</th>
                </tr>
            </thead>
            <tbody></tbody>
        </table>
    </div>

    <div id="connection" class="tab-pane fade">
        <div class="alert alert-info" role="alert">
            {{ lang._('Edit a profile row to set the entry node, routing (full / split / none), the LAN sources to policy-route, and the kill-switch. Enable a row and reconfigure to connect.') }}
        </div>
    </div>

    <div id="settings" class="tab-pane fade">
        {{ partial('layout_partials/base_form', ['fields': generalForm, 'id': 'frm_general']) }}
        <button class="btn btn-primary" id="saveGeneral" type="button">{{ lang._('Save') }}</button>
    </div>
</div>

<div class="col-md-12">
    <div id="responseMsg" class="alert alert-info hidden" role="alert"></div>
    <button class="btn btn-primary" id="reconfigureAct" type="button"
            data-endpoint="/api/pharosvpn/service/reconfigure"
            data-label="{{ lang._('Apply / Reconnect') }}"
            data-error-title="{{ lang._('Error reconfiguring PharosVPN') }}"></button>
</div>

{{ partial('layout_partials/base_dialog', ['fields': formDialogEditClient, 'id': 'DialogEditClient', 'label': lang._('Edit connection')]) }}

<!-- Import dialog -->
<div class="modal fade" id="dialogImportProfile" tabindex="-1" role="dialog">
    <div class="modal-dialog" role="document">
        <div class="modal-content">
            <div class="modal-header">
                <button type="button" class="close" data-dismiss="modal"><span>&times;</span></button>
                <h4 class="modal-title">{{ lang._('Import .pharos profile') }}</h4>
            </div>
            <div class="modal-body">
                <div class="form-group">
                    <label>{{ lang._('Profile file') }}</label>
                    <input type="file" id="profileFile" accept=".pharos"/>
                </div>
                <div class="form-group">
                    <label>{{ lang._('Password (password-mode profiles)') }}</label>
                    <input type="password" id="importPassword" class="form-control"/>
                </div>
                <div id="importMsg" class="hidden"></div>
                <div id="importMeta" class="hidden"></div>
            </div>
            <div class="modal-footer">
                <button type="button" class="btn btn-primary" id="doImport">{{ lang._('Parse') }}</button>
                <button type="button" class="btn btn-default" data-dismiss="modal">{{ lang._('Close') }}</button>
            </div>
        </div>
    </div>
</div>
