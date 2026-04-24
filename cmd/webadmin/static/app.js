// 19box Web Admin - Frontend Logic

const API_BASE = '';

// State
let isRunning = false;
let pollInterval = null;
let currentServerConfig = {};
let currentFiltersConfig = {};

// Filter names (matches server_base.yaml)
const FILTER_NAMES = [
    'kicked_listener_filter',
    'user_pending_filter',
    'duplicate_track_filter',
    'duplicate_artist_filter',
    'blacklist_track_filter',
    'blacklist_artist_filter',
    'duration_limit_filter',
];

// DOM Elements
const setupView = document.getElementById('setupView');
const controlView = document.getElementById('controlView');
const statusIndicator = document.getElementById('statusIndicator');
const presetSelect = document.getElementById('presetSelect');

// Form elements
const sessionTitle = document.getElementById('sessionTitle');
const startTime = document.getElementById('startTime');
const endTime = document.getElementById('endTime');
const keywords = document.getElementById('keywords');
const openingUrl = document.getElementById('openingUrl');
const openingName = document.getElementById('openingName');
const endingUrl = document.getElementById('endingUrl');
const endingName = document.getElementById('endingName');

// Buttons
const startBtn = document.getElementById('startBtn');
const stopBtn = document.getElementById('stopBtn');
const sessionStopBtn = document.getElementById('sessionStopBtn');
const pauseBtn = document.getElementById('pauseBtn');
const resumeBtn = document.getElementById('resumeBtn');
const skipBtn = document.getElementById('skipBtn');

// Status elements
const infoTitle = document.getElementById('infoTitle');
const infoState = document.getElementById('infoState');
const infoSchedule = document.getElementById('infoSchedule');
const infoStats = document.getElementById('infoStats');
const trackName = document.getElementById('trackName');
const trackRequester = document.getElementById('trackRequester');
const trackRemaining = document.getElementById('trackRemaining');
const listenerList = document.getElementById('listenerList');

// Initialize Flatpickr
let startPicker, endPicker;

document.addEventListener('DOMContentLoaded', () => {
    initDatePickers();
    loadConfig();
    bindEvents();
});

function initDatePickers() {
    const now = new Date();
    const options = {
        enableTime: true,
        dateFormat: 'Y-m-d H:i',
        time_24hr: true,
        theme: 'dark',
        allowInput: true,
        defaultHour: now.getHours(),
        defaultMinute: now.getMinutes(),
        onOpen: function (selectedDates, dateStr, instance) {
            if (selectedDates.length === 0) {
                const now = new Date();
                instance.set('defaultHour', now.getHours());
                instance.set('defaultMinute', now.getMinutes());
                instance.jumpToDate(now);
            }
        }
    };

    startPicker = flatpickr(startTime, {
        ...options,
        placeholder: 'Immediate start',
    });

    endPicker = flatpickr(endTime, {
        ...options,
        placeholder: 'Manual end',
    });
}

function bindEvents() {
    presetSelect.addEventListener('change', onPresetChange);
    startBtn.addEventListener('click', onStartServer);
    stopBtn.addEventListener('click', onStopServer);
    sessionStopBtn.addEventListener('click', onStopSession);
    pauseBtn.addEventListener('click', () => sessionAction('pause'));
    resumeBtn.addEventListener('click', () => sessionAction('resume'));
    skipBtn.addEventListener('click', () => sessionAction('skip'));
}

// API Functions
async function api(endpoint, method = 'GET', body = null) {
    const options = {
        method,
        headers: { 'Content-Type': 'application/json' },
    };
    if (body) {
        options.body = JSON.stringify(body);
    }
    const response = await fetch(API_BASE + endpoint, options);
    return response.json();
}

// Load initial config
async function loadConfig() {
    try {
        const data = await api('/api/config');

        // Update running state
        updateRunningState(data.running);

        // Populate presets
        presetSelect.innerHTML = '<option value="">-- Select Preset --</option>';
        if (data.presets) {
            data.presets.forEach(preset => {
                const option = document.createElement('option');
                option.value = preset.name;
                option.textContent = `${preset.name} - ${preset.description}`;
                presetSelect.appendChild(option);
            });
        }

        // Populate form with base config
        populateForm(data);
    } catch (err) {
        showToast('Failed to load config: ' + err.message, 'error');
    }
}

// Populate form fields
function populateForm(data) {
    if (data.session) {
        sessionTitle.value = data.session.title || '';
        if (data.session.start_time) {
            startPicker.setDate(data.session.start_time);
        } else {
            startPicker.clear();
        }
        if (data.session.end_time) {
            endPicker.setDate(data.session.end_time);
        } else {
            endPicker.clear();
        }
        if (data.session.keywords) {
            keywords.value = Array.isArray(data.session.keywords)
                ? data.session.keywords.join(', ')
                : data.session.keywords;
        } else {
            keywords.value = '';
        }
    }

    if (data.playlists) {
        if (data.playlists.opening) {
            openingUrl.value = data.playlists.opening.playlist_url || '';
            openingName.value = data.playlists.opening.display_name || '';
        } else {
            openingUrl.value = '';
            openingName.value = '';
        }
        if (data.playlists.ending) {
            endingUrl.value = data.playlists.ending.playlist_url || '';
            endingName.value = data.playlists.ending.display_name || '';
        } else {
            endingUrl.value = '';
            endingName.value = '';
        }
    } else {
        openingUrl.value = '';
        openingName.value = '';
        endingUrl.value = '';
        endingName.value = '';
    }

    if (data.server) {
        currentServerConfig = data.server;
        renderHooks(data.server.hooks);
    } else {
        currentServerConfig = {};
        renderHooks(null);
    }

    // Populate filters
    if (data.filters) {
        currentFiltersConfig = data.filters;
        populateFilters(data.filters);
    } else {
        currentFiltersConfig = {};
        // Set all filters to enabled by default
        FILTER_NAMES.forEach(name => {
            const checkbox = document.getElementById(`filter_${name}`);
            if (checkbox) checkbox.checked = true;
        });
    }
}

// Populate filter checkboxes
function populateFilters(filters) {
    FILTER_NAMES.forEach(name => {
        const checkbox = document.getElementById(`filter_${name}`);
        if (checkbox && filters[name] !== undefined) {
            checkbox.checked = filters[name].enabled !== false;
        }
    });
}

// Render server hooks
function renderHooks(hooks) {
    const container = document.getElementById('hooksDisplay');
    if (!hooks || Object.keys(hooks).length === 0) {
        container.innerHTML = '<div class="empty-state">No hooks configured</div>';
        return;
    }

    let html = '';
    for (const [name, commands] of Object.entries(hooks)) {
        if (!commands || commands.length === 0) continue;
        html += `
            <div class="hook-block">
                <div class="hook-name">${escapeHtml(name)}</div>
                <div class="hook-commands">
                    ${commands.map(cmd => `<div class="hook-command">${escapeHtml(cmd)}</div>`).join('')}
                </div>
            </div>
        `;
    }

    if (!html) {
        container.innerHTML = '<div class="empty-state">No hooks configured</div>';
    } else {
        container.innerHTML = html;
    }
}

// Filter key to display name mapping
const FILTER_DISPLAY_NAMES = {
    'duration_limit_filter': 'Duration Limit Filter',
    'kicked_listener_filter': 'Kicked Listener Filter',
    'user_pending_filter': 'User Pending Filter',
    'duplicate_track_filter': 'Duplicate Track Filter',
    'duplicate_artist_filter': 'Duplicate Artist Filter',
    'blacklist_track_filter': 'Blacklist Track Filter',
    'blacklist_artist_filter': 'Blacklist Artist Filter',
};

// Render active filters
function renderActiveFilters(filters) {
    const container = document.getElementById('activeFiltersDisplay');
    if (!container) return;

    if (!filters || filters.length === 0) {
        container.innerHTML = '<div class="empty-state">No filters active</div>';
        return;
    }

    // Sort filters for consistent display order
    const sortedFilters = [...filters].sort();

    const html = sortedFilters.map(key => {
        const displayName = FILTER_DISPLAY_NAMES[key] || key;
        return `<div class="filter-tag">${escapeHtml(displayName)}</div>`;
    }).join('');

    container.innerHTML = `<div class="filter-tags">${html}</div>`;
}

// Handle preset change
async function onPresetChange() {
    const presetName = presetSelect.value;
    if (!presetName) {
        loadConfig();
        return;
    }

    try {
        const data = await api(`/api/config/preset/${presetName}`);
        populateForm(data);
        if (data.server) {
            currentServerConfig = data.server;
            renderHooks(data.server.hooks);
        }
        if (data.filters) {
            currentFiltersConfig = data.filters;
            populateFilters(data.filters);
        }
    } catch (err) {
        showToast('Failed to load preset: ' + err.message, 'error');
    }
}

// Get form data
function getFormData() {
    const keywordList = keywords.value
        .split(',')
        .map(k => k.trim())
        .filter(k => k);

    // Collect filter settings from checkboxes
    const filters = {};
    FILTER_NAMES.forEach(name => {
        const checkbox = document.getElementById(`filter_${name}`);
        if (checkbox) {
            filters[name] = { enabled: checkbox.checked };
        }
    });

    return {
        presetName: presetSelect.value || '',
        session: {
            title: sessionTitle.value,
            start_time: startPicker.selectedDates[0]?.toISOString() || '',
            end_time: endPicker.selectedDates[0]?.toISOString() || '',
            keywords: keywordList,
        },
        playlists: {
            opening: {
                playlist_url: openingUrl.value,
                display_name: openingName.value,
            },
            ending: {
                playlist_url: endingUrl.value,
                display_name: endingName.value,
            },
        },
        server: currentServerConfig,
        filters: filters,
    };
}

// Start server
async function onStartServer() {
    startBtn.disabled = true;
    startBtn.textContent = 'Starting...';

    try {
        const formData = getFormData();
        const result = await api('/api/server/start', 'POST', formData);

        if (result.success) {
            showToast('Server started successfully');
            updateRunningState(true);
        } else {
            // Keep startup errors until user dismisses them
            showToast(result.error || 'Failed to start server', 'error', 0);
        }
    } catch (err) {
        showToast('Failed to start server: ' + err.message, 'error', 0);
    } finally {
        startBtn.disabled = false;
        startBtn.textContent = '🚀 Start Server';
    }
}

// Stop server
async function onStopServer() {
    stopBtn.disabled = true;
    stopBtn.textContent = 'Stopping...';

    try {
        const result = await api('/api/server/stop', 'POST');

        if (result.success) {
            showToast('Server stopped');
            updateRunningState(false);
        } else {
            showToast(result.error || 'Failed to stop server', 'error');
        }
    } catch (err) {
        showToast('Failed to stop server: ' + err.message, 'error');
    } finally {
        stopBtn.disabled = false;
        stopBtn.textContent = '⏹ Stop Server';
    }
}

// Stop session (graceful)
async function onStopSession() {
    sessionStopBtn.disabled = true;
    sessionStopBtn.textContent = 'Stopping...';

    try {
        const result = await api('/api/session/stop', 'POST');

        if (result.success) {
            showToast('Session stop requested');
        } else {
            showToast(result.error || 'Failed to stop session', 'error');
        }
    } catch (err) {
        showToast('Failed to stop session: ' + err.message, 'error');
    } finally {
        sessionStopBtn.disabled = false;
        sessionStopBtn.textContent = '🏁 Stop Session';
    }
}

// Session actions (pause/resume/skip)
async function sessionAction(action) {
    try {
        const result = await api(`/api/session/${action}`, 'POST');
        if (result.success) {
            showToast(result.message);
        } else {
            showToast(result.error || result.message, 'error');
        }
    } catch (err) {
        showToast(`Failed to ${action}: ` + err.message, 'error');
    }
}

// Update running state
function updateRunningState(running) {
    isRunning = running;

    // Update status indicator
    const dot = statusIndicator.querySelector('.status-dot');
    const text = statusIndicator.querySelector('.status-text');

    if (running) {
        dot.className = 'status-dot running';
        text.textContent = 'Running';
        setupView.classList.add('hidden');
        controlView.classList.remove('hidden');
        stopBtn.classList.remove('hidden');
        startPolling();
    } else {
        dot.className = 'status-dot stopped';
        text.textContent = 'Stopped';
        setupView.classList.remove('hidden');
        controlView.classList.add('hidden');
        stopBtn.classList.add('hidden');
        stopPolling();
    }
}

// Polling for status updates
function startPolling() {
    if (pollInterval) return;
    pollStatus();
    pollInterval = setInterval(pollStatus, 2000);
}

function stopPolling() {
    if (pollInterval) {
        clearInterval(pollInterval);
        pollInterval = null;
    }
}

async function pollStatus() {
    try {
        const status = await api('/api/server/status');

        if (!status.running) {
            updateRunningState(false);
            return;
        }

        // Update session info
        if (status.sessionInfo) {
            const info = status.sessionInfo;
            infoTitle.textContent = info.playlist_name || '-';
            infoState.textContent = formatState(info.state);

            const start = info.scheduled_start_time ? formatDateTime(info.scheduled_start_time) : '-';
            const end = info.scheduled_end_time ? formatDateTime(info.scheduled_end_time) : '-';
            infoSchedule.textContent = `${start} / ${end}`;
        }

        infoStats.textContent = `${status.queueSize || 0} tracks / ${status.listenerCount || 0} listeners`;

        // Update current track
        if (status.currentTrack) {
            const track = status.currentTrack;
            const artists = track.artists?.join(', ') || '';
            trackName.textContent = `${track.name} - ${artists}`;
            trackRequester.textContent = `by ${track.requester_name || '-'}`;
            trackRemaining.textContent = `Remaining: ${formatDuration(track.remaining_seconds)}`;
        } else {
            trackName.textContent = 'No track playing';
            trackRequester.textContent = '-';
            trackRemaining.textContent = '-';
        }

        // Update active filters
        renderActiveFilters(status.activeFilters);

        // Update healthcheck
        renderHealthcheck(status.healthcheck);

        // Update listeners
        await updateListeners();
    } catch (err) {
        console.error('Poll error:', err);
    }
}

// Render healthcheck results as tabs
let activeHealthcheckTab = null;

function renderHealthcheck(hc) {
    const card = document.getElementById('healthcheckCard');
    const tabsContainer = document.getElementById('healthcheckTabs');
    const contentContainer = document.getElementById('healthcheckContent');
    if (!card || !tabsContainer || !contentContainer) return;

    if (!hc || Object.keys(hc).length === 0) {
        card.style.display = 'none';
        activeHealthcheckTab = null;
        return;
    }

    card.style.display = '';
    const names = Object.keys(hc).sort();

    // Default to first tab if current selection is gone
    if (!activeHealthcheckTab || !hc[activeHealthcheckTab]) {
        activeHealthcheckTab = names[0];
    }

    // Build tabs
    tabsContainer.innerHTML = names.map(name => {
        const active = name === activeHealthcheckTab ? ' active' : '';
        return `<button class="hc-tab${active}" data-hc-tab="${name}">${name}</button>`;
    }).join('');

    // Attach click handlers
    tabsContainer.querySelectorAll('.hc-tab').forEach(btn => {
        btn.addEventListener('click', () => {
            activeHealthcheckTab = btn.dataset.hcTab;
            renderHealthcheck(hc);
        });
    });

    // Show active tab content (preserve scroll position)
    const result = hc[activeHealthcheckTab];
    let text = result.output || '';
    if (result.error) {
        text += (text ? '\n' : '') + '⚠ Error: ' + result.error;
    }
    const existingOutput = contentContainer.querySelector('.healthcheck-output');
    const scrollTop = existingOutput ? existingOutput.scrollTop : 0;
    contentContainer.innerHTML = `<div class="healthcheck-output"><pre>${escapeHtml(text || '-')}</pre></div>`;
    contentContainer.querySelector('.healthcheck-output').scrollTop = scrollTop;
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

async function updateListeners() {
    try {
        const data = await api('/api/listeners');
        const listeners = data.listeners || [];

        if (listeners.length === 0) {
            listenerList.innerHTML = '<div class="empty-state">No listeners</div>';
            return;
        }

        // Sort listeners by joined_at (earliest first), with listener_id as tie-breaker
        listeners.sort((a, b) => {
            const timeDiff = new Date(a.joined_at) - new Date(b.joined_at);
            if (timeDiff !== 0) return timeDiff;
            return a.listener_id.localeCompare(b.listener_id);
        });

        listenerList.innerHTML = listeners.map(l => `
            <div class="listener-item ${l.is_kicked ? 'kicked' : ''}">
                <div class="listener-info">
                    <div class="listener-name-row">
                        <span class="listener-name">${escapeHtml(l.display_name)}</span>
                        ${l.is_kicked ? '<span class="listener-tag">Kicked</span>' : ''}
                    </div>
                    <span class="listener-meta">${l.pending_tracks || 0} pending · joined ${formatTime(l.joined_at)}</span>
                </div>
                <button class="btn btn-secondary btn-kick" 
                    onclick="kickListener('${l.listener_id}')" 
                    ${l.is_kicked ? 'disabled' : ''}>
                    ${l.is_kicked ? 'Kicked' : 'Kick'}
                </button>
            </div>
        `).join('');
    } catch (err) {
        console.error('Failed to fetch listeners:', err);
    }
}

// Kick listener
async function kickListener(listenerId) {
    try {
        const result = await api(`/api/listeners/${listenerId}/kick`, 'POST');
        if (result.success) {
            showToast('Listener kicked');
            updateListeners();
        } else {
            showToast(result.error || result.message, 'error');
        }
    } catch (err) {
        showToast('Failed to kick listener: ' + err.message, 'error');
    }
}

// Utility functions
function formatState(state) {
    const states = {
        0: 'Unknown',
        1: 'Waiting',
        2: 'Running',
        3: 'Paused',
        4: 'Waiting for Tracks',
        5: 'Ending',
        6: 'Terminated',
    };
    return states[state] || state;
}

function formatDateTime(isoString) {
    if (!isoString) return '-';
    const date = new Date(isoString);
    return date.toLocaleString('ja-JP', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
    });
}

function formatTime(isoString) {
    if (!isoString) return '-';
    const date = new Date(isoString);
    return date.toLocaleTimeString('ja-JP', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
    });
}

function formatDuration(seconds) {
    if (!seconds || seconds < 0) return '-';
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Toast notifications
function showToast(message, type = 'success', duration = 3000) {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;

    const content = document.createElement('span');
    content.textContent = message;
    toast.appendChild(content);

    const closeBtn = document.createElement('span');
    closeBtn.className = 'toast-close';
    closeBtn.innerHTML = '×';
    closeBtn.onclick = () => toast.remove();
    toast.appendChild(closeBtn);

    container.appendChild(toast);

    if (duration > 0) {
        setTimeout(() => {
            if (toast.parentElement) toast.remove();
        }, duration);
    }
}

// Make kickListener available globally
window.kickListener = kickListener;
