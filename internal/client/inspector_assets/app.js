// Fonzygrok Inspector — vanilla JS, SSE-driven live request table.
(function() {
    'use strict';

    const table = document.getElementById('requestTable');
    const filterInput = document.getElementById('filterInput');
    const statusFilter = document.getElementById('statusFilter');
    const clearBtn = document.getElementById('clearBtn');
    const countEl = document.getElementById('requestCount');
    const statusEl = document.getElementById('status');
    const detailPanel = document.getElementById('detailPanel');
    const detailTitle = document.getElementById('detailTitle');
    const reqHeaders = document.getElementById('reqHeaders');
    const respHeaders = document.getElementById('respHeaders');
    const bodyPreview = document.getElementById('bodyPreview');
    const closeDetail = document.getElementById('closeDetail');

    let requests = [];
    let selectedId = null;

    // Load initial data.
    fetch('/api/requests')
        .then(r => r.json())
        .then(data => {
            requests = data || [];
            render();
        });

    // SSE stream for live updates.
    function connectSSE() {
        const es = new EventSource('/api/requests/stream');
        es.onmessage = function(e) {
            const entry = JSON.parse(e.data);
            requests.push(entry);
            if (requests.length > 100) requests.shift();
            render();
            autoScroll();
        };
        es.onopen = function() {
            statusEl.textContent = '● Connected';
            statusEl.className = 'status connected';
        };
        es.onerror = function() {
            statusEl.textContent = '● Disconnected';
            statusEl.className = 'status disconnected';
            es.close();
            setTimeout(connectSSE, 2000);
        };
    }
    connectSSE();

    // Filter logic.
    filterInput.addEventListener('input', render);
    statusFilter.addEventListener('change', render);

    // Clear button.
    clearBtn.addEventListener('click', function() {
        fetch('/api/requests', { method: 'DELETE' })
            .then(() => {
                requests = [];
                render();
                detailPanel.classList.add('hidden');
                selectedId = null;
            });
    });

    // Close detail panel.
    closeDetail.addEventListener('click', function() {
        detailPanel.classList.add('hidden');
        selectedId = null;
        document.querySelectorAll('tr.selected').forEach(r => r.classList.remove('selected'));
    });

    function render() {
        const pathFilter = filterInput.value.toLowerCase();
        const statusPre = statusFilter.value;

        const filtered = requests.filter(r => {
            if (pathFilter && !r.path.toLowerCase().includes(pathFilter)) return false;
            if (statusPre && !String(r.status_code).startsWith(statusPre)) return false;
            return true;
        });

        countEl.textContent = filtered.length + ' request' + (filtered.length !== 1 ? 's' : '');

        table.innerHTML = '';
        filtered.forEach(r => {
            const tr = document.createElement('tr');
            if (r.id === selectedId) tr.classList.add('selected');
            tr.classList.add('new-row');
            tr.dataset.id = r.id;

            const time = new Date(r.timestamp).toLocaleTimeString('en-US', { hour12: false });
            const methodClass = 'method-' + r.method.toLowerCase();
            const statusClass = 'status-' + Math.floor(r.status_code / 100) + 'xx';
            const duration = r.duration_ms < 1 ? '<1ms' : Math.round(r.duration_ms) + 'ms';
            const size = formatBytes(r.response_size);

            tr.innerHTML =
                '<td class="col-time">' + time + '</td>' +
                '<td class="col-method ' + methodClass + '">' + r.method + '</td>' +
                '<td class="col-path" title="' + escapeHtml(r.path) + '">' + escapeHtml(r.path) + '</td>' +
                '<td class="col-status ' + statusClass + '">' + r.status_code + '</td>' +
                '<td class="col-duration">' + duration + '</td>' +
                '<td class="col-size">' + size + '</td>';

            tr.addEventListener('click', function() { showDetail(r); });
            table.appendChild(tr);
        });
    }

    function showDetail(r) {
        selectedId = r.id;
        detailTitle.textContent = r.method + ' ' + r.path;
        reqHeaders.textContent = formatHeaders(r.request_headers);
        respHeaders.textContent = formatHeaders(r.response_headers);
        bodyPreview.textContent = r.body_preview || '(no body)';
        detailPanel.classList.remove('hidden');

        document.querySelectorAll('tr.selected').forEach(row => row.classList.remove('selected'));
        const row = document.querySelector('tr[data-id="' + r.id + '"]');
        if (row) row.classList.add('selected');
    }

    function formatHeaders(headers) {
        if (!headers || Object.keys(headers).length === 0) return '(none)';
        return Object.entries(headers).map(function(kv) { return kv[0] + ': ' + kv[1]; }).join('\n');
    }

    function formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1048576) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / 1048576).toFixed(1) + ' MB';
    }

    function escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function autoScroll() {
        var container = document.querySelector('.table-container');
        var isNearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
        if (isNearBottom) {
            requestAnimationFrame(function() {
                container.scrollTop = container.scrollHeight;
            });
        }
    }
})();
