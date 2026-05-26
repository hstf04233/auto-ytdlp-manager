
function truncateString(str, num) {
  if (str.length > num) {
    return str.slice(0, num) + "...";
  } else {
    return str;
  }
}

// ========== API helpers ==========
const API = {
  async get(url) {
    const res = await fetch(url);
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API GET ${truncateString(url, 128)}: ${res.status} - ${text}`);
    }
    return res.json();
  },
  async post(url, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API POST ${truncateString(url, 128)}: ${res.status} - ${text}`);
    }
    return res.json();
  },
  async patch(url, body) {
    const res = await fetch(url, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API PATCH ${truncateString(url, 128)}: ${res.status} - ${text}`);
    }
    return res.json();
  },
  async del(url) {
    const res = await fetch(url, {
      method: 'DELETE',
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API DELETE ${truncateString(url, 128)}: ${res.status} - ${text}`);
    }
    return res.json();
  },
};

const isLocalhost = Boolean(
  window.location.hostname === 'localhost' ||
  window.location.hostname === '[::1]' || // IPv6
  window.location.hostname.match(/^127(?:\.(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}$/) // IPv4 127.0.0.1 range
);

// ========== State ==========

// These are to stop spamming the api endpoints until the last call is finished!
let areChannelsLoading = false;
let areVideosLoading   = false;
let areTasksLoading    = false;

let programConfig = {};
let allChannels = [];
let allVideos = [];
let videoPage = 0;
let videoTotalCount = 0;
let videoStats = {};
const VIDEO_PAGE_SIZE = 30;

let lastPageOpen = ''
let _pgCallbacks = {}
let _pgId = 0

function _registerPgCbs(cbs) {
  let id = ++_pgId
  if (id > 40) {
    _pgId = 1
    id = _pgId
  }
  _pgCallbacks[id] = cbs
  return id
}

let lastVideoPage = 0;
let lastVideosCount = 0;

let lastTaskPage = 0;
let lastTasksCount = 0;

function setTitle(newTitle) {
  if (newTitle != "") {
    document.title = `${newTitle} - Auto yt-dlp Manager`
  }
}

function showPage(page, dontSaveHistory) {
  const basePage = new URL("/"+page, window.location.origin).pathname.replace(/^\//, '');
  
  let title = basePage;
  
  if (!dontSaveHistory) {
    if (basePage != lastPageOpen) {
      history.pushState({}, '', '/' + page);
    } else {
      window.history.replaceState(null, "", "/" + page);
    }
  } else {
  }
  if (basePage != lastPageOpen) {
    lastVideoPage = 0;
    lastVideosCount = 0;
    
    lastTaskPage = 0;
    lastTasksCount = 0;
  }
  lastPageOpen = basePage
  
  document.querySelectorAll('.sidebar nav a').forEach(l => l.classList.remove('active'));
  document.querySelectorAll('.sidebar nav a').forEach(l => {
    if (l.dataset.page == basePage) {
      l.classList.add('active');
    }
  })
  
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  let pageDoc = document.getElementById(`page-${basePage}`)
  if (pageDoc) {
    pageDoc.classList.add('active');
  }
  
  const urlParams = new URLSearchParams(new URL(page, window.location.origin).search);
  
  if (basePage === 'videos') {
    if (urlParams.has("channel")) {
      let channelId = urlParams.get("channel");
      document.getElementById('videoChannelFilter').value = channelId;
    } else {
      document.getElementById('videoChannelFilter').value = '';
    }
    
    updateVideosTabTitle();
    title = '';   // Won't set title if blank.
    
    if (!areVideosLoading) {
      loadVideos();
    }
  }
  if (basePage === 'tasks') {
    if (urlParams.has("channel")) {
      let channelId = urlParams.get("channel");
      document.getElementById('taskChannelFilter').value = channelId;
    } else {
      document.getElementById('taskChannelFilter').value = '';
    }
    if (urlParams.has("video")) {
      let videoId = urlParams.get("video");
      document.getElementById('taskVideoFilter').value = videoId;
    } else {
      document.getElementById('taskVideoFilter').value = '';
    }
    
    updateTasksTabTitle();
    title = '';   // Won't set title if blank.
    onTaskFilterChange(true);
    
    stopRealtimePolling();
    if (!areTasksLoading) {
      loadTasks();
    }
  } else {
    stopRealtimePolling();
  }
  
  if (title !== "") {
    setTitle(title);
  }
}

// ========== Navigation ==========
document.querySelectorAll('.sidebar nav a').forEach(link => {
  link.addEventListener('click', e => {
    e.preventDefault();
    const page = link.dataset.page;
    showPage(page)
  });
});

// ========== Toast ==========
function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  const MAX_TOASTS = 3;
  while (container.querySelectorAll('.toast').length >= MAX_TOASTS) {
    container.querySelector('.toast').remove();
  }
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

// ========== Channel helpers ==========
function statusBadge(type) {
  if (type === 0) {
    return '<span class="badge badge-type-videos">Videos</span>';
  }
  if (type === 1) {
    return '<span class="badge badge-type-live">Live</span>';
  }
  if (type === 2) {
    return '<span class="badge badge-type-videos">List only</span>';
  }
  if (type === 3) {
    return '<span class="badge badge-type-videos">No Download</span>';
  }
  return '<span class="badge badge-type-videos">Videos?</span>';
}

function qualityLabel(q) {
  if (q === 0) return 'Highest';
  return `${q}p`;
}

function getQualityFromResolution(res) {
  const resolutionParts = (res || '0x0').split('x');
  const width  = parseInt(resolutionParts[0] || 0)
  const height = parseInt(resolutionParts[1] || 0)
  
  if (width > height) {
    return height;
  } else {
    return width;
  }
}

function intervalLabel(s) {
  if (s <= 0) return '...';
  
  const seconds = Math.floor(s) % 60
  if (s < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(s / 60) % 60
  if (s < 3600) {
    return `${minutes}m ${seconds}s`;
  }
  const hours = Math.floor(s / 3600) % 24
  if (s < 86400) {
    return `${hours}h ${minutes}m`;
  }
  const days = Math.floor(s / 86400)
  return `${days}d ${hours}h`;
}

function formatDate(ts) {
  if (!ts) return '\u2014';
  return new Date(ts * 1000).toLocaleDateString();
}
function formatDateAndTime(ts) {
  if (!ts) return '\u2014';
  var date = null;
  if (typeof(ts) == "string") {
    date = new Date(ts);
  } else {
    // time in seconds.
    date = new Date(ts * 1000);
  }
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`;
}

function formatRelative(isoStr) {
  if (!isoStr) return '\u2014';
  const date = new Date(isoStr);
  const now = new Date();
  const diffMs = now - date;
  if (diffMs < 0) return 'Just now';
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) {
    const mins = diffMin % 60;
    return mins ? `${diffHr}h ${mins}m ago` : `${diffHr}h ago`;
  }
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 30) {
    const hrs = diffHr % 24;
    return hrs ? `${diffDay}d ${hrs}h ago` : `${diffDay}d ago`;
  }
  return formatDate(Math.floor(date.getTime() / 1000));
}

function formatDuration(sec) {
  if (!sec || sec <= 0) return '0:00';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

function formatBytesSize(sizeInBytes) {
  const kb = Math.ceil(sizeInBytes/1000*10)/10;   // Rounded by one decimal place.
  if (kb < 1000) {
    return `${kb} KB`
  }
  const mb = Math.ceil(sizeInBytes/1000/1000*10)/10;
  if (mb < 1000) {
    return `${mb} MB`
  }
  const gb = Math.ceil(sizeInBytes/1000/1000/1000*10)/10;
  return `${gb} GB`
}

function getFormatedTaskDuration(task) {
  let startDate = new Date(task.start_time);
  if (task.status == 0) {
    // Task is currently running
    const now = new Date();
    const diffMs = now - startDate;
    return formatDuration(diffMs/1000);
  }
  
  let endDate = new Date(task.end_time);
  let updatedDate = new Date(task.updated_at);
  if (updatedDate > endDate) {
    endDate = updatedDate;
  }
  
  const diffMs = endDate - startDate;
  return formatDuration(diffMs/1000);
}

function videoStatusBadge(videoId, status) {
  const map = {
    0: ['queued', 'Queued'],
    1: ['downloading', 'Downloading...'],
    2: ['downloaded', 'Downloaded'],
    3: ['failed', 'Failed'],
    4: ['ignored', 'Ignored'],
  };
  const [cls, label] = map[status] || ['queued', `Status ${status}`];
  return `<span class="badge badge-${cls} status-badge" onclick="event.stopPropagation();toggleStatusDropdown('${videoId}', ${status}, this)" title="Click to change status">${label}</span>`;
}

function hideAllStatusDropdowns() {
  document.querySelectorAll('.status-dropdown').forEach(el => el.remove());
}

let _dropdownListenerActive = false;
let statusDropdownIsActive = false;

function closeStatusDropdown() {
  hideAllStatusDropdowns();
  _dropdownListenerActive = false;
  statusDropdownIsActive = false;
}

function toggleStatusDropdown(videoId, currentStatus, buttonEl) {
  if (statusDropdownIsActive) {
    closeStatusDropdown();
    return;
  }
  const videoData = allVideos.find(x => x.id === videoId);
  
  statusDropdownIsActive = true;
  const rect = buttonEl.getBoundingClientRect();
  const dropdown = document.createElement('div');
  dropdown.className = 'status-dropdown';
  dropdown.style.top = (rect.bottom + 4) + 'px';
  dropdown.style.left = rect.left + 'px';
  
  let statuses = [
    [0, "Set to 'Queued' (Cancel download, if downloading.)"],
    //[1, 'Downloading'],
    //[2, 'Downloaded'],
    //[3, 'Failed'],
    [4, "Set to 'Ignored' (Cancel download, if downloading.)"],
    //[-100, "Download this video"],
  ];
  
  if (videoData && videoData.status != 2 && videoData.status != 1) {
    statuses.push([-100, "Download this video"]);
  }
  
  dropdown.innerHTML = statuses.map(([val, label]) =>
    `<div class="status-option ${val === currentStatus ? 'active' : ''}" data-video-id="${videoId}" data-new-status="${val}">${label}</div>`
  ).join('');
  
  dropdown.addEventListener('click', (e) => {
    e.stopPropagation();
    const option = e.target.closest('.status-option');
    if (option) {
      closeStatusDropdown();
      changeVideoStatus(option.dataset.videoId, parseInt(option.dataset.newStatus));
    }
  });
  
  document.body.appendChild(dropdown);
  if (!_dropdownListenerActive) {
    document.addEventListener('click', closeStatusDropdown);
    _dropdownListenerActive = true;
  }
}

async function changeVideoStatus(videoId, newStatus) {
  try {
    const videoData = allVideos.find(x => x.id === videoId);
    if (newStatus == -100) {
      // Download this video
      if (!videoData) return;
      
      let body = {
        download_url: videoData.url,
        type: -2,
        
        target_channel_id: videoData.from_channel,
      };
      
      let qualitySelect = getQualityFromResolution(videoData.resolution);
      console.log(videoData.resolution);
      console.log(qualitySelect);
      if (qualitySelect > 0) {
        body.quality_select = qualitySelect;
      }
      
      await API.post(`/api/add-videos?no_wait=true&queue_video_id=${videoId}`, body);
      showToast('Status changed', 'success');
      loadVideos();
      return;
    }
    
    await API.patch(`/api/videos/${videoId}`, { status: parseInt(newStatus) });
    showToast('Status changed', 'success');
    loadVideos();
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

function videoTypeBadge(vtype) {
  const map = {
    0: ['video', 'Video?'],
    1: ['video', 'Video'],
    2: ['live', 'Live'],
    3: ['waslive', 'Was Live'],
  };
  const [cls, label] = map[vtype] || ['video', 'Video'];
  return `<span class="badge badge-${cls}">${label}</span>`;
}

async function loadConfig() {
  try {
    const data = await API.get('/api/config');
    programConfig = data;
    
    renderConfig(programConfig);
  } catch (err) {
    showToast(`Failed to load program config: ${err.message}`, 'error');
  }
}

function cancelConfigChanges() {
  renderConfig(programConfig);
}

// Set .value and .placeholder to the same value.
function setInputPV(el, value) {
  el.placeholder = value
  el.value       = value
}

function areThereConfigChanges() {
  const YtDlpPathEl = document.getElementById("config-YtDlpPath")
  if (YtDlpPathEl && YtDlpPathEl.value !== programConfig.YtDlp_Path) return true
  const YtArchivePathEl = document.getElementById("config-YtArchivePath")
  if (YtArchivePathEl && YtArchivePathEl.value !== programConfig.YtArchive_Path) return true
  const FFmpegPathEl = document.getElementById("config-FFmpegPath")
  if (FFmpegPathEl && FFmpegPathEl.value !== programConfig.FFmpeg_Path) return true
  
  const DownloadDirEl = document.getElementById("config-DownloadDir")
  if (DownloadDirEl && DownloadDirEl.value !== programConfig.Default_DownloadDir) return true
  const OutputTemplateEl = document.getElementById("config-OutputTemplate")
  if (OutputTemplateEl && OutputTemplateEl.value !== programConfig.Default_YtDlp_OutputTemplate) return true
  const OutputTemplateLiveEl = document.getElementById("config-OutputTemplateLive")
  if (OutputTemplateLiveEl && OutputTemplateLiveEl.value !== programConfig.Default_YtDlp_OutputTemplate_Live) return true
  
  const AllChannelsDisabledEl = document.getElementById("config-AllChannelsDisabled")
  if (AllChannelsDisabledEl && AllChannelsDisabledEl.checked !== programConfig.AllChannels_Disabled) return true
  const TaskLogAutoDeleteEnabledEl = document.getElementById("config-TaskLogAutoDeleteEnabled")
  if (TaskLogAutoDeleteEnabledEl && TaskLogAutoDeleteEnabledEl.checked !== programConfig.TaskLog_AutoDelete_Enabled) return true
  
  const TaskLogAutoDeleteSecondsEl = document.getElementById("config-TaskLogAutoDeleteSeconds")
  if (TaskLogAutoDeleteSecondsEl && TaskLogAutoDeleteSecondsEl.value != programConfig.TaskLog_AutoDelete_Seconds) return true
  const TaskLogListAutoDeleteSecondsEl = document.getElementById("config-TaskLogListAutoDeleteSeconds")
  if (TaskLogListAutoDeleteSecondsEl && TaskLogListAutoDeleteSecondsEl.value != programConfig.TaskLog_List_AutoDelete_Seconds) return true
  
  return false
}

async function saveConfig(e) {
  e.preventDefault();
  
  const YtDlpPathEl = document.getElementById("config-YtDlpPath")
  const YtArchivePathEl = document.getElementById("config-YtArchivePath")
  const FFmpegPathEl = document.getElementById("config-FFmpegPath")
  
  const DownloadDirEl = document.getElementById("config-DownloadDir")
  const OutputTemplateEl = document.getElementById("config-OutputTemplate")
  const OutputTemplateLiveEl = document.getElementById("config-OutputTemplateLive")
  
  const AllChannelsDisabledEl = document.getElementById("config-AllChannelsDisabled")
  const TaskLogAutoDeleteEnabledEl = document.getElementById("config-TaskLogAutoDeleteEnabled")
  
  const TaskLogAutoDeleteSecondsEl = document.getElementById("config-TaskLogAutoDeleteSeconds")
  const TaskLogListAutoDeleteSecondsEl = document.getElementById("config-TaskLogListAutoDeleteSeconds")
  
  const body = {
    YtDlp_Path:     YtDlpPathEl.value.trim(),
    YtArchive_Path: YtArchivePathEl.value.trim(),
    FFmpeg_Path:    FFmpegPathEl.value.trim(),
    
    AllChannels_Disabled: AllChannelsDisabledEl.checked,
    TaskLog_AutoDelete_Enabled: TaskLogAutoDeleteEnabledEl.checked,
    
    Default_DownloadDir:  DownloadDirEl.value.trim(),
    Default_YtDlp_OutputTemplate: OutputTemplateEl.value,
    Default_YtDlp_OutputTemplate_Live: OutputTemplateLiveEl.value,
    
    TaskLog_AutoDelete_Seconds: TaskLogAutoDeleteSecondsEl.value ? parseInt(TaskLogAutoDeleteSecondsEl.value) : programConfig.TaskLog_AutoDelete_Seconds,
    TaskLog_List_AutoDelete_Seconds: TaskLogListAutoDeleteSecondsEl.value ? parseInt(TaskLogListAutoDeleteSecondsEl.value) : programConfig.TaskLog_List_AutoDelete_Seconds,
  };
  
  try {
    await API.patch("/api/config", body);
    showToast('Config updated!', 'success');
    
    loadConfig();
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

document.getElementById("configForm").addEventListener('input', () => {
  const configSubmitBtn = document.getElementById("configSubmitBtn")
  const configCancelBtn = document.getElementById("configCancelBtn")
  wereChangesMade = areThereConfigChanges();
  
  if (configSubmitBtn) {
    configSubmitBtn.disabled = !wereChangesMade;
  }
  if (configCancelBtn) {
    configCancelBtn.disabled = !wereChangesMade;
  }
});

function renderConfig(config) {
  const ApplicationVersionEl = document.getElementById("side-bar-application-version")
  if (ApplicationVersionEl) {
    ApplicationVersionEl.textContent = config.application_version;
  }
  
  
  const YtDlpPathEl = document.getElementById("config-YtDlpPath")
  const YtArchivePathEl = document.getElementById("config-YtArchivePath")
  const FFmpegPathEl = document.getElementById("config-FFmpegPath")
  
  const DownloadDirEl = document.getElementById("config-DownloadDir")
  const OutputTemplateEl = document.getElementById("config-OutputTemplate")
  const OutputTemplateLiveEl = document.getElementById("config-OutputTemplateLive")
  
  const AllChannelsDisabledEl = document.getElementById("config-AllChannelsDisabled")
  const AllChannelsDisabledEl2 = document.getElementById("channels-config-AllChannelsDisabled")
  const TaskLogAutoDeleteEnabledEl = document.getElementById("config-TaskLogAutoDeleteEnabled")
  
  const TaskLogAutoDeleteSecondsEl = document.getElementById("config-TaskLogAutoDeleteSeconds")
  const TaskLogListAutoDeleteSecondsEl = document.getElementById("config-TaskLogListAutoDeleteSeconds")
  
  const configSubmitBtn = document.getElementById("configSubmitBtn")
  if (configSubmitBtn) {
    configSubmitBtn.disabled = true
  }
  const configCancelBtn = document.getElementById("configCancelBtn")
  if (configCancelBtn) {
    configCancelBtn.disabled = true
  }
  
  if (YtDlpPathEl) {
    setInputPV(YtDlpPathEl, programConfig.YtDlp_Path)
  }
  if (YtArchivePathEl) {
    setInputPV(YtArchivePathEl, programConfig.YtArchive_Path)
  }
  if (FFmpegPathEl) {
    setInputPV(FFmpegPathEl, programConfig.FFmpeg_Path)
  }
  
  if (DownloadDirEl) {
    setInputPV(DownloadDirEl, programConfig.Default_DownloadDir)
  }
  if (OutputTemplateEl) {
    setInputPV(OutputTemplateEl, programConfig.Default_YtDlp_OutputTemplate)
  }
  if (OutputTemplateLiveEl) {
    setInputPV(OutputTemplateLiveEl, programConfig.Default_YtDlp_OutputTemplate_Live)
  }
  
  if (AllChannelsDisabledEl) {
    AllChannelsDisabledEl.checked = programConfig.AllChannels_Disabled
  }
  if (AllChannelsDisabledEl2) {
    AllChannelsDisabledEl2.checked = programConfig.AllChannels_Disabled
  }
  if (TaskLogAutoDeleteEnabledEl) {
    TaskLogAutoDeleteEnabledEl.checked = programConfig.TaskLog_AutoDelete_Enabled
  }
  
  if (TaskLogAutoDeleteSecondsEl) {
    setInputPV(TaskLogAutoDeleteSecondsEl, programConfig.TaskLog_AutoDelete_Seconds)
  }
  if (TaskLogListAutoDeleteSecondsEl) {
    setInputPV(TaskLogListAutoDeleteSecondsEl, programConfig.TaskLog_List_AutoDelete_Seconds)
  }
}

// ========== Channels ==========
let lastChannelsCount = -1;
async function loadChannels(softUpdateOnly) {
  areChannelsLoading = true;
  try {
    const data = await API.get('/api/channels');
    allChannels = data.channels || data;
    if (!softUpdateOnly || allChannels.length !== lastChannelsCount) {
      renderChannels();
      updateChannelFilters();
      lastChannelsCount = allChannels.length
    } else {
      // Soft update
      softRenderChannels();
    }
  } catch (err) {
    showToast(`Failed to load channels: ${err.message}`, 'error');
  }
  areChannelsLoading = false;
}

function renderUpdateChannel(ch) {
  const channelEl = document.getElementById("channel-" + ch.id);
  if (!channelEl) return;
  
  const nextCheckEl = channelEl.querySelector("#next-check");
  const channelCheckBtnEl = channelEl.querySelector("#channel-check-btn");
  if (nextCheckEl) {
    let checkTime = new Date(ch._nextCheckMsec);
    let now = new Date();
    let delta = checkTime-now;
    
    let isEnabled = (ch.enabled && !programConfig.AllChannels_Disabled && !ch.active_task);
    
    if (delta <= 1 || !isEnabled) {
      channelCheckBtnEl.disabled = true;
    } else {
      channelCheckBtnEl.disabled = false;
    }
    
    let disabledText = "DISABLED!"
    if (ch.active_task) {
      disabledText = "Checking channel now...";
    }
    
    if (isEnabled) {
      if (delta > 0) {
        nextCheckEl.textContent = "Will check in: " + intervalLabel(delta/1000);
      } else {
        nextCheckEl.textContent = "Checking channel now...";
      }
    } else if (nextCheckEl.textContent !== disabledText) {
      nextCheckEl.textContent = disabledText;
    }
  }
  
  const channelTaskBtnEl = channelEl.querySelector("#channel-task-btn");
  
  if (channelTaskBtnEl) {
    let tasksButtonText = `View Logs[${ch.tasks_count}]`;
    if (ch.active_task) {
      tasksButtonText = `View Active Task + Logs[${ch.tasks_count-1}]`;
    }
    
    if (channelTaskBtnEl.textContent != tasksButtonText) {
      channelTaskBtnEl.textContent = tasksButtonText;
    }
  }
  
  const channelEnabledCheckboxEl = channelEl.querySelector("#channel-enabled-checkbox");
  if (channelEnabledCheckboxEl) {
    channelEnabledCheckboxEl.checked = ch.enabled;
  }
}

function softRenderChannels() {
  for (const ch of allChannels) {
    renderUpdateChannel(ch);
  }
}

function renderChannels() {
  const container = document.getElementById('channelsList');
  
  const total = allChannels.length;
  const enabled = allChannels.filter(c => c.enabled).length;
  const disabled = total - enabled;
  
  /*
  const statsContainer = document.getElementById('channelStats');
  statsContainer.innerHTML = `
    <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--success)">${enabled}</div><div class="stat-label">Enabled</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--text-secondary)">${disabled}</div><div class="stat-label">Disabled</div></div>
  `;
  */
  
  container.innerHTML = '';
  
  let visibleChannelsCount = 0;
  
  for (const ch of allChannels) {
    if (ch.hidden) continue;
    
    visibleChannelsCount += 1;
    
    channelHtml = `
      <div class="card channel-card" id="channel-${ch.id}">
        <div class="channel-info">
          <h3>${escHtml(ch.name)}</h3>
          <p>${escHtml(ch.url)}</p>
          <div class="channel-meta">
            ${statusBadge(ch.type)}
            <span>Quality: ${qualityLabel(ch.quality_select)}</span>
            <span id="next-check">Check: ${intervalLabel(ch.check_interval)}</span>
          </div>
          <p><a id="channel-videos-btn" href="/videos?channel=${ch.id}" onclick="event.preventDefault();gotoVideosPageAndFilterChannel('${ch.id}');">View Videos</a></p>
          <p><a id="channel-task-btn" href="/tasks?channel=${ch.id}" onclick="event.preventDefault();gotoTasksPageAndFilterChannel('${ch.id}');">...</a></p>
        </div>
        <div class="video-actions">
          <label class="toggle">
            <input id="channel-enabled-checkbox" title="Channel enabled checkbox" type="checkbox" ${ch.enabled ? 'checked' : ''} onchange="enableChannel('${ch.id}', this.checked)">
            <span class="toggle-slider"></span>
          </label>
          <button class="btn btn-secondary btn-sm" onclick="runChannelCheck('${ch.id}')" id="channel-check-btn">Check videos now</button>
          <button class="btn btn-secondary btn-sm" onclick="openEditChannelModal('${ch.id}')">Edit</button>
          <button class="btn btn-danger btn-sm" onclick="deleteChannel('${ch.id}')">Delete</button>
        </div>
      </div>
    `;
    container.insertAdjacentHTML("beforeend", channelHtml);
    
    renderUpdateChannel(ch);
  }
  
  if (visibleChannelsCount === 0) {
    container.innerHTML = '<div class="loading">No channels yet. Add one to get started!</div>';
    return;
  }
}

async function enableChannel(id, enabled) {
  try {
    await API.patch(`/api/channels/${id}`, {
      enabled: enabled
    });
    const channel = getChannelFromId(id);
    if (channel) {
      channel.enabled = enabled;
    }
    //showToast(`Channel ${enabled ? 'enabled' : 'disabled'}`, 'success');
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

async function runChannelCheck(id, name) {
  try {
    await API.post(`/api/check-channel-now/${id}`, {});
    loadChannels();
  } catch (err) {
    showToast(`Failed to check channel: ${err.message}`, 'error');
  }
}

async function deleteChannel(id) {
  const ch = getChannelFromId(id);
  const name = ch ? (ch.name) : "UNKNOWN CHANNEL"
  
  if (!confirm("Delete channel \"" + name + "\"? This will not delete any saved videos from this channel.")) return;
  try {
    await API.del(`/api/channels/${id}`);
    showToast("Channel \"" + name + "\" was deleted!", 'success');
    loadChannels();
  } catch (err) {
    showToast(`Failed to delete: ${err.message}`, 'error');
  }
}
async function deleteVideo(id) {
  try {
    await API.del(`/api/videos/${id}`);
    showToast("Video was deleted!", 'success');
    loadVideos();
  } catch (err) {
    showToast(`Failed to delete: ${err.message}`, 'error');
  }
}
async function refreshVideoInfo(id,) {
  try {
    await API.patch(`/api/videos/${id}`, {refresh_state: true});
    loadVideos()
  } catch (err) {
    showToast(`Failed to refresh: ${err.message}`, 'error');
  }
}

function videoPreviewClick(videoId) {
  let modalVideoPreview = document.getElementById('modal-video-preview');
  if (modalVideoPreview) {
    modalVideoPreview.innerHTML = `
    <video controls autoplay>
      <source src="/video-file/${escHtml(videoId)}" type="video/mp4">
      Your browser does not support the video tag.
    </video>
    `
  }
}

function getThumbnail(videoInfo) {
  if (videoInfo.stored_thumbnail) {
    return `/db-image/${videoInfo.stored_thumbnail}`
  }
  if (videoInfo.origin_thumbnail_url) {
    return videoInfo.origin_thumbnail_url
  }
  
  // This video contains no thumbnail url?
  // Default to youtube's thumbnail url and hope it's correct...
  return `https://img.youtube.com/vi/${videoInfo.id}/mqdefault.jpg`
}

function openVideoDetailsModal(videoId) {
  const v = allVideos.find(x => x.id === videoId);
  if (!v) return;

  const channel = getChannelFromId(v.from_channel);
  const channelName = channel ? escHtml(channel.name) : 'Unknown Channel';
  const channelUrl = channel ? escHtml(channel.url) : '';

  const resolutionParts = (v.resolution || '').split('x');
  const resolutionText = resolutionParts.length === 2
    ? `${resolutionParts[0].trim()} x ${resolutionParts[1].trim()}`
    : (v.resolution || '\u2014');

  const descHtml = v.description
    ? escHtml(v.description).replace(/\n/g, '<br>')
    : '\u2014';
  
  let videoPreviewOnClick = "";
  if (v.videofile_exists) {
    videoPreviewOnClick = `event.preventDefault(); videoPreviewClick('${videoId}');`;
  }
  
  var thumbnailUrl = getThumbnail(v);
  
  document.getElementById('videoDetailsContent').innerHTML = `
    <div class="vd-preview" id="modal-video-preview">
      <img src="${escHtml(thumbnailUrl)}" alt="" onclick="${videoPreviewOnClick}" onerror="this.onerror=null; this.src='/static/images/NoThumbnail_bw.jpg';"
        ${v.videofile_exists ? ` style="cursor: pointer;" title="Click to play video"` : ''}>
        ${v.videofile_exists ? `<span>&#9654;</span>` : ''}
      </img>
      <div class="vd-preview-placeholder" style="display:none">No thumbnail</div>
    </div>
    <h3 class="vd-title">${escHtml(v.title)}</h3>
    ${v.uploader ? `<p class="vd-header">Uploader: <a href="${escHtml(v.uploader_url)}" target="_blank">${escHtml(v.uploader)}</a></p>` : ''}
    <p class="vd-header">${escHtml(v.availability)}</p>
    <hr>
    <div class="vd-grid">
      <div class="vd-field"><span class="vd-field-label">Channel</span><span class="vd-field-value">${escHtml(channelName)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video ID</span><span class="vd-field-value" style="font-family:monospace;font-size:0.8rem">${escHtml(v.id)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video URL</span><a href="${escHtml(v.url)}" target="_blank" class="vd-field-value" style="font-family:monospace;font-size:0.8rem">${escHtml(v.url)}</a></div>
      <div class="vd-field"><span class="vd-field-label">Duration</span><span class="vd-field-value">${formatDuration(v.duration)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video Type</span><span class="vd-field-value">${v.video_type !== undefined ? videoTypeBadge(v.video_type) : '\u2014'}</span></div>
      <div class="vd-field"><span class="vd-field-label">Status</span><span class="vd-field-value">${videoStatusBadge(v.id, v.status)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Filename ${(v.videofile_exists || !v.filename || v.status == 1) ? '' : '(Moved or deleted)'}</span><span class="vd-field-value" style="font-family:monospace;font-size:0.78rem;word-break:break-all">${escHtml(v.filename || '\u2014')}</span></div>
      <div class="vd-field"><span class="vd-field-label">Filesize</span><span class="vd-field-value">${formatBytesSize(v.filesize || 0)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Resolution</span><span class="vd-field-value">${resolutionText}</span></div>
      <div class="vd-field"><span class="vd-field-label">Release Date</span><span class="vd-field-value">${formatDateAndTime(v.release_date)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Added</span><span class="vd-field-value">${formatDateAndTime(v.added_at)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Updated</span><span class="vd-field-value">${formatDateAndTime(v.updated_at)}</span></div>
    </div>

    
    <div class="vd-description">
      <span class="vd-field-label">Description</span>
      <div class="vd-description-text">${descHtml}</div>
    </div>
  `;

  const refreshDisabled = v.refresh_state ? 'disabled style="opacity:0.5;cursor:not-allowed"' : '';
  const refreshTitle = v.refresh_state ? 'Refreshing...' : 'Refresh metadata';

  document.getElementById('videoDetailsActions').innerHTML = `
    ${
      v.videofile_exists ?
    `<a href="/video-file/${escHtml(v.id)}?download=true" target="_blank" class="btn btn-secondary btn-sm" title="Download video file">Download Video</a>` :
    ''
    }
    <button type="button" class="btn btn-secondary btn-sm" ${refreshDisabled} onclick="refreshVideoInfo('${v.id}');closeVideoDetailsModal();" title="${refreshTitle}">${v.refresh_state ? 'Refreshing...' : 'Refresh'}</button>
    <button type="button" class="btn btn-danger btn-sm" onclick="deleteVideo('${v.id}');closeVideoDetailsModal();">Delete</button>
  `;

  document.getElementById('videoDetailsModal').classList.add('active');
  document.body.classList.add('modal-active');
}

function closeVideoDetailsModal() {
  let modalVideoPreview = document.getElementById('modal-video-preview');
  if (modalVideoPreview) {
    // Remove any video playing
    modalVideoPreview.innerHTML = "";
  }
  
  document.getElementById('videoDetailsModal').classList.remove('active');
  document.body.classList.remove('modal-active');
}

function updateChannelModalPlaceholders() {
  let channelDownloadDir = document.getElementById('channelDownloadDir');
  if (channelDownloadDir) {
    channelDownloadDir.placeholder = programConfig.Default_DownloadDir || "./downloads"
  }
  let channelOutputTemplate = document.getElementById('channelOutputTemplate');
  if (channelOutputTemplate) {
    if (document.getElementById('channelType').value == 1) { // ACHANNEL_TYPE_LIVE
      channelOutputTemplate.placeholder = programConfig.Default_YtDlp_OutputTemplate_Live || '%(title)s %(id)s.%(ext)s';
    } else {
      channelOutputTemplate.placeholder = programConfig.Default_YtDlp_OutputTemplate || '%(title)s %(id)s.%(ext)s';
    }
  }
}

// ========== Channel Modal ==========
function openAddChannelModal() {
  
  document.getElementById('channelModalTitle').textContent = 'Add Channel';
  document.getElementById('channelId').value = '';
  document.getElementById('channelName').value = '';
  document.getElementById('channelUrl').value = '';
  document.getElementById('channelType').value = '0';
  document.getElementById('channelQuality').value = '0';
  document.getElementById('channelDownloadDir').value = '';
  document.getElementById('channelOutputTemplate').value = '';
  document.getElementById('channelCheckInterval').value = '';
  document.getElementById('channelPlaylistEnd').value = '20';
  document.getElementById('channelSubmitBtn').textContent = 'Add Channel';
  
  document.body.classList.add('modal-active');
  document.getElementById('channelModal').classList.add('active');
  
  updateChannelModalPlaceholders();
}

function getChannelFromId(id) {
  const ch = allChannels.find(c => c.id === id);
  return ch;
}

function openEditChannelModal(id) {
  const ch = getChannelFromId(id);
  if (!ch) return;

  document.getElementById('channelModalTitle').textContent = 'Edit Channel';
  document.getElementById('channelId').value = ch.id;
  document.getElementById('channelName').value = ch.name;
  document.getElementById('channelUrl').value = ch.url;
  document.getElementById('channelType').value = ch.type;
  document.getElementById('channelQuality').value = ch.quality_select;
  document.getElementById('channelDownloadDir').value = ch.download_dir || '';
  document.getElementById('channelOutputTemplate').value = ch.output_template || '';
  document.getElementById('channelCheckInterval').value = ch.check_interval || '';
  document.getElementById('channelPlaylistEnd').value = ch.playlist_end;
  document.getElementById('channelSubmitBtn').textContent = 'Save Changes';
  document.getElementById('channelModal').classList.add('active');
  
  updateChannelModalPlaceholders();
}

function closeChannelModal() {
  document.getElementById('channelModal').classList.remove('active');
  document.body.classList.remove('modal-active');
}

let currentChannel;

function openChannelStartModal() {
  document.getElementById('channelStartModal').classList.add('active');
  document.body.classList.add('modal-active');
}
function closeChannelStartModal() {
  document.getElementById('channelStartModal').classList.remove('active');
  document.body.classList.remove('modal-active');
  
  if (currentChannel) {
    enableChannel(currentChannel.id, true);
  }
}

async function checkCurrentChannel(overrideType, allVideos) {
  if (!currentChannel) {
    console.log("checkCurrentChannel called without a currentChannel ?")
    return
  }
  
  try {
    await API.post(`/api/check-channel-now/${currentChannel.id}`, {
      instant_check: true,
      
      check_all_videos: allVideos,
      override_channel_type: overrideType,
    });
    loadChannels();
  } catch (err) {
    showToast(`Failed to check channel: ${err.message}`, 'error');
  }
  
  closeChannelStartModal();
}


function openAddVideosModal() {
  document.querySelectorAll('.videoadd-modal-actions-list button').forEach(l => l.disabled = false);
  
  document.getElementById('addVideosModal').classList.add('active');
  document.body.classList.add('modal-active');
  
  document.getElementById('avm-ChannelUrl').value = '';
  document.getElementById('avm-DownloadDir').textContent = `Default download directory: "${escHtml(programConfig.Default_DownloadDir)}"`;
}
function closeAddVideosModal() {
  document.getElementById('addVideosModal').classList.remove('active');
  document.body.classList.remove('modal-active');
}

async function sendAddVideosForm(type) {
  document.querySelectorAll('.videoadd-modal-actions-list button').forEach(l => l.disabled = true);
  
  const url = document.getElementById('avm-ChannelUrl').value.trim();
  const quality = parseInt(document.getElementById('avm-Quality').value);
  
  const body = {
    download_url: url,
    quality_select: quality,
    type: type,
  };
  
  try {
    await API.post('/api/add-videos', body);
    //showToast('Channel added!', 'success');
    
    closeAddVideosModal();
    loadVideos();
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
    document.querySelectorAll('.videoadd-modal-actions-list button').forEach(l => l.disabled = false);
  }
}

async function saveChannel(e) {
  e.preventDefault();
  const id   = document.getElementById('channelId').value;
  const name = document.getElementById('channelName').value.trim();
  const url  = document.getElementById('channelUrl').value.trim();
  const type = parseInt(document.getElementById('channelType').value);
  const quality = parseInt(document.getElementById('channelQuality').value);
  const downloadDir = document.getElementById('channelDownloadDir').value.trim();
  const outputTemplate = document.getElementById('channelOutputTemplate').value.trim();
  const checkInterval  = document.getElementById('channelCheckInterval').value ? parseInt(document.getElementById('channelCheckInterval').value) : 1800;
  const playlistEnd = parseInt(document.getElementById('channelPlaylistEnd').value);

  const body = {
    name,
    url,
    type,
    quality_select: quality,
    download_dir: downloadDir,
    output_template: outputTemplate,
    check_interval: checkInterval,
    playlist_end: playlistEnd,
  };

  try {
    let newChannelData = null;
    if (id) {
      // Edit
      const patch = {};
      if (name) patch.name = name;
      if (url) patch.url = url;
      if (downloadDir) patch.download_dir = downloadDir;
      if (outputTemplate) patch.output_template = outputTemplate;
      patch.quality_select = quality;
      patch.type = type;
      patch.check_interval = checkInterval;
      patch.playlist_end = playlistEnd;
      newChannelData = await API.patch(`/api/channels/${id}`, patch);
      showToast('Channel updated!', 'success');
      
      closeChannelModal();
    } else {
      newChannelData = await API.post('/api/channels', body);
      showToast('Channel added!', 'success');
      
      currentChannel = newChannelData;
      closeChannelModal();
      openChannelStartModal();
    }
    loadChannels();
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

// ========== Videos ==========
async function loadVideos() {
  areVideosLoading = true;
  try {
    const statusFilter = document.getElementById('videoStatusFilter').value;
    const channelFilter = document.getElementById('videoChannelFilter').value;
    const orderBy = document.getElementById('videoOrderBy').value;
    const orderDir = document.getElementById('videoOrderDirection').value;
    
    let url = `/api/videos?limit=${VIDEO_PAGE_SIZE}&offset=${videoPage * VIDEO_PAGE_SIZE}`;
    if (statusFilter !== '') {
      url += `&status=${statusFilter}`;
    }
    if (channelFilter !== '') {
      url += `&from_channel=${channelFilter}`;
    }
    if (orderBy) {
      url += `&order_by=${orderBy}`;
    }
    url += `&order_direction=${orderDir}`;
    const search = document.getElementById('videoSearch').value;
    if (search !== '') {
      url += `&search_query=${encodeURIComponent(search)}`;
    }
    
    const data = await API.get(url);
    allVideos = data.videos || data;
    videoStats = data.stats || {};
    videoTotalCount = videoStats.total || allVideos.length;
    renderVideos();
    renderVideoPagination();
    updateVideoStats();
  } catch (err) {
    showToast(`Failed to load videos: ${err.message}`, 'error');
  }
  areVideosLoading = false;
}

function filterVideosTabByChannel(channelId) {
  const videoChannelFilterEl = document.getElementById('videoChannelFilter');
  
  const optionExists = Array.from(videoChannelFilterEl.options).some(opt => opt.value === channelId);
  
  if (!optionExists) {
    const newOption = new Option(channelId, channelId);
    newOption.hidden = true;
    videoChannelFilterEl.add(newOption);
  }
  
  videoChannelFilterEl.value = channelId;
  videoPage = 0;
  
  onVideoFilterChange();
}

function renderVideos() {
  const container = document.getElementById('videosList');

  if (allVideos.length === 0) {
    container.innerHTML = '<div class="loading">No videos found.</div>';
    return;
  }

  container.innerHTML = allVideos.map(v => {
    var thumbnailUrl = getThumbnail(v);
    var durationText = formatDuration(v.duration)
    if (v.video_type == 2 && (v.duration <= 1)) {  // VIDEO_TYPE_ISLIVE
      durationText = "LIVE"
    }
    
    const channel = getChannelFromId(v.from_channel);
    const refreshDisabled = v.refresh_state ? 'disabled style="opacity:0.5;cursor:not-allowed"' : '';
    const refreshTitle = v.refresh_state ? 'Refreshing...' : 'Refresh metadata';
    
    let tasksButtonText = "View Logs"
    if (v.active_task) {
      tasksButtonText = `View Active Task + Logs[${v.tasks_count-1}]`
    } else if (v.tasks_count > 1) {
      tasksButtonText = `View Logs[${v.tasks_count}]`
    }
    let tasksButtonOnClick = `
      event.preventDefault();
      gotoTasksPageAndFilterVideo('${v.id}');
    `
    if (v.active_task) {
      tasksButtonOnClick = `
        event.preventDefault();
        gotoTasksPageAndFilterVideo('${v.id}');
        selectTask('${v.active_task}')
      `
    }
    
    return `
      <div class="card video-card">
        <div class="video-thumb">
          <img src="${thumbnailUrl}" alt="" onerror="this.onerror=null; this.src='/static/images/NoThumbnail_bw.jpg'">
          <span class="video-duration" title="Video duration">${durationText}</span>
        </div>
        <div class="video-info">
          <h3 class="video-title">
            <span title="${escHtml(v.title)}">${escHtml(v.title)}</span>
            <a href="${escHtml(v.url)}" target="_blank">[VideoLink]</a>
          </h3>
          <p title="Filter by this channel">From: <a href="/videos?channel=${v.from_channel}" onclick="event.preventDefault();filterVideosTabByChannel('${v.from_channel}');">${channel ? escHtml(channel.name) : 'Unknown Channel'}</a></p>
          <p>Released: ${formatDateAndTime(v.release_date)}</p>
          <p>${escHtml(v.availability)}</p>
          <p>Added ${formatRelative(v.added_at)} \u00b7 Updated ${formatRelative(v.updated_at)}</p>
          
          ${(v.tasks_count > 0 || v.active_task) ? `<p><a href="/tasks?video=${v.id}" onclick="${tasksButtonOnClick}">${tasksButtonText}</a></p>` : ''}
        </div>
        <div class="video-actions">
          ${videoStatusBadge(v.id, v.status)}
          ${v.video_type !== undefined ? videoTypeBadge(v.video_type) : ''}
          ${
            v.videofile_exists ?
            `<a href="/video-file/${escHtml(v.id)}" target="_blank" class="btn btn-secondary btn-sm" title="Open video file">Video File</a>` :
            ''
          }
          <button class="btn btn-secondary btn-sm" ${refreshDisabled} onclick="refreshVideoInfo('${v.id}')" title="${refreshTitle}">${v.refresh_state ? 'Refreshing...' : 'Refresh'}</button>
          <button class="btn btn-secondary btn-sm" onclick="openVideoDetailsModal('${v.id}')">Details</button>
          <button class="btn btn-danger btn-sm" title="Deleting a video does not remove the video file." onclick="deleteVideo('${v.id}')">Delete</button>
        </div>
      </div>
    `;
  }).join('');
}

function clearVideoFilters(dontLoadVideos) {
  document.getElementById('videoSearch').value = '';
  document.getElementById('videoStatusFilter').value = '';
  //document.getElementById('videoChannelFilter').value = '';
  document.getElementById('videoOrderBy').value = 'release_date';
  document.getElementById('videoOrderDirection').value = '-1';
  videoPage = 0;
  if (!areVideosLoading && !dontLoadVideos) {
    loadVideos();
  }
}

let videoSearchDidUpdate = false;
function videoSearchFilterUpdate() {
  videoSearchDidUpdate = true;
  const videoSearchClearEl = document.getElementById("video-search-clear-btn");
  if (videoSearchClearEl) {
    let searchText = document.getElementById('videoSearch').value;
    if (searchText && searchText.length > 0) {
      videoSearchClearEl.style.display = 'block';
    } else {
      videoSearchClearEl.style.display = 'none';
    }
  }
}
function clearVideoSearch() {
  document.getElementById('videoSearch').value = '';
  videoSearchFilterUpdate();
}

function updateVideosTabTitle() {
  if (lastPageOpen != "videos") return;
  
  const channelId = document.getElementById('videoChannelFilter').value
  
  const channel = getChannelFromId(channelId);
  if (channel) {
    setTitle(`Videos: ${channel.name}`)
  } else {
    setTitle("Videos")
  }
}

function onVideoFilterChange() {
  const channelId = document.getElementById('videoChannelFilter').value
  if (channelId) {
    window.history.replaceState(null, "", `/videos?channel=${channelId}`);
  } else {
    window.history.replaceState(null, "", `/videos`);
  }
  updateVideosTabTitle();
  
  videoPage = 0;
  loadVideos();
}

function renderVideoPagination() {
  const container_top    = document.getElementById('videoPagination-top');
  const container_bottom = document.getElementById('videoPagination-bottom');
  const totalPages = Math.ceil(videoTotalCount / VIDEO_PAGE_SIZE) || 1;
  if (lastVideosCount == videoTotalCount && lastVideoPage == videoPage) {
    // Nothing has changed.
    return;
  }
  lastVideoPage = videoPage;
  lastVideosCount = videoTotalCount;
  
  const cbsId = _registerPgCbs({
    prev: () => { if (!areVideosLoading && videoPage > 0) { videoPage--; loadVideos(); } },
    next: () => { if (!areVideosLoading) { videoPage++; loadVideos(); } },
    page: (p) => { if (!areVideosLoading) { videoPage = p; loadVideos(); } },
  });
  
  const paginationHtml = buildPaginationHTML({
    currentPage: videoPage,
    totalPages: totalPages,
    totalCount: videoTotalCount,
    currentItems: allVideos.length,
    label: 'videos',
    cbsId: cbsId,
  });
  
  if (videoPage > totalPages) {
    videoPage = totalPages;
  }
  
  if (container_top)    container_top.innerHTML = paginationHtml;
  if (container_bottom) container_bottom.innerHTML = paginationHtml;
}

function buildPaginationHTML(cfg) {
  const { currentPage, totalPages, totalCount, currentItems, label, cbsId } = cfg;
  
  const btn = (labelText, page, disabled, active) => {
    const disabledAttr = disabled ? ' disabled' : '';
    const activeAttr = active ? ' class="active"' : '';
    let onclick = '';
    if (!disabled) {
      if (page < 0) {
        // Prev
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].prev())"`;
      } else if (page >= totalPages) {
        // Next
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].next())"`;
      } else {
        // Page num
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].page(${page}))"`;
      }
    }
    return `<button${activeAttr}${disabledAttr}${onclick}>${labelText}</button>`;
  };
  
  //const singlePageMsg = `All ${label} shown (page 1 of 1)`;
  const countMsg = `Showing ${currentItems} ${label} out of ${totalCount}`;
  
  if (totalPages <= 1 && currentPage < totalPages) {
    return `<span class="single-page-msg">${countMsg}</span>`;
  }
  
  // Page numbers group
  let pageButtons = [];
  
  if (totalPages <= 7) {
    for (let i = 0; i < totalPages; i++) {
      pageButtons.push(btn(String(i + 1), i, false, currentPage === i));
    }
  } else {
    let rangeStart = Math.max(1, currentPage - 2);
    let rangeEnd = Math.min(totalPages - 2, currentPage + 2);
    
    if (rangeStart <= 1) {
      rangeEnd = Math.min(totalPages - 2, rangeStart + 5);
    }
    if (rangeEnd >= totalPages - 3) {
      if (rangeEnd == totalPages - 3) {
        rangeEnd += 1;
      }
      rangeStart = Math.max(1, rangeEnd - 5);
    }
    
    const isFirstActive = currentPage === 0;
    pageButtons.push(btn('1', 0, false, isFirstActive));
    
    if (rangeStart > 2) {
      pageButtons.push('<span class="page-ellipsis">…</span>');
    } else if (rangeStart === 2) {
      pageButtons.push(btn('2', 1, false, false));
    }
    
    for (let i = rangeStart; i <= rangeEnd; i++) {
      pageButtons.push(btn(String(i + 1), i, false, currentPage === i));
    }
    
    if (rangeEnd < totalPages - 2) {
      pageButtons.push('<span class="page-ellipsis">…</span>');
    }
    
    pageButtons.push(btn(String(totalPages), totalPages - 1, false, currentPage === totalPages - 1));
  }
  
  const prevDisabled = currentPage === 0;
  const nextDisabled = currentPage >= totalPages - 1;
  const prevBtn = btn('‹', -1, prevDisabled, false);
  const nextBtn = btn('›', totalPages+100, nextDisabled, false);
  return prevBtn + pageButtons.join('') + nextBtn + `<span class="single-page-msg">${countMsg}</span>`;
}

function renderTaskPagination() {
  const container_top    = document.getElementById('taskPagination-top');
  const container_bottom = document.getElementById('taskPagination-bottom');
  const totalPages = Math.ceil(taskTotalCount / TASK_PAGE_SIZE) || 1;
  if (lastTasksCount == taskTotalCount && lastTaskPage == taskPage) {
    // Nothing has changed.
    return;
  }
  lastTaskPage = taskPage;
  lastTasksCount = taskTotalCount;
  
  const cbsId = _registerPgCbs({
    prev: () => { if (!areTasksLoading && taskPage > 0) { taskPage--; loadTasks(); } },
    next: () => { if (!areTasksLoading) { taskPage++; loadTasks(); } },
    page: (p) => { if (!areTasksLoading) { taskPage = p; loadTasks(); } },
  });
  
  const paginationHtml = buildPaginationHTML({
    currentPage: taskPage,
    totalPages: totalPages,
    totalCount: taskTotalCount,
    currentItems: allTasks.length,
    label: 'tasks',
    cbsId: cbsId,
  });
  
  if (taskPage > totalPages) {
    taskPage = totalPages;
  }
  
  if (container_top)    container_top.innerHTML = paginationHtml;
  if (container_bottom) container_bottom.innerHTML = paginationHtml;
}

function updateVideoStats() {
  const container = document.getElementById('videoStats');
  
  const total = videoStats.total || 0;
  const queued      = videoStats.total_queued || 0;
  const downloading = videoStats.total_downloading || 0;
  const downloaded  = videoStats.total_downloaded || 0;
  const failed      = videoStats.total_failed || 0;
  const ignored     = videoStats.total_ignored || 0;
  
  if (container) {
    container.innerHTML = `
      <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total</div></div>
      <div class="stat-card"><div class="stat-value" style="color:var(--info)">${queued}</div><div class="stat-label">Queued</div></div>
      <div class="stat-card"><div class="stat-value" style="color:var(--warning)">${downloading}</div><div class="stat-label">Downloading</div></div>
      <div class="stat-card"><div class="stat-value" style="color:var(--success)">${downloaded}</div><div class="stat-label">Downloaded</div></div>
      <div class="stat-card"><div class="stat-value" style="color:var(--danger)">${failed}</div><div class="stat-label">Failed</div></div>
      <div class="stat-card"><div class="stat-value" style="color:var(--text-secondary)">${ignored}</div><div class="stat-label">Ignored</div></div>
    `;
  }
}

// ========== Channel filter dropdown ==========
function updateChannelFilters() {
  const videoSelect = document.getElementById('videoChannelFilter');
  
  let current = videoSelect.value;
  videoSelect.innerHTML = '<option value="">All</option><hr>';
  allChannels.forEach(ch => {
    const opt = document.createElement('option');
    opt.value = ch.id;
    opt.textContent = ch.name;
    if (ch.id === current) opt.selected = true;
    videoSelect.appendChild(opt);
  });
  
  const taskSelect = document.getElementById('taskChannelFilter');
  
  current = taskSelect.value;
  taskSelect.innerHTML = '<option value="">All</option><hr>';
  allChannels.forEach(ch => {
    const opt = document.createElement('option');
    opt.value = ch.id;
    opt.textContent = ch.name;
    if (ch.id === current) opt.selected = true;
    taskSelect.appendChild(opt);
  });
  
}

// ========== Tasks ==========
let allTasks = [];
let taskPage = 0;
const TASK_PAGE_SIZE = 20;
let taskTotalCount = 0;
let taskStatsData = {};
let selectedTask = null;
let selectedTaskId = null;
let selectedTaskType = null;
let realtimeOutputTimer = null;
let realtimeOutputOffset = 0;

const TASK_TYPE_LABELS = { 0: 'Generic', 1: 'Listing', 2: 'Download' };
const TASK_STATUS_BADGE = {
  0: ['downloading', 'Running'],
  1: ['failed', 'Failed'],
  2: ['downloaded', 'Finished'],
  3: ['failed', 'Canceled'],
};

function taskTypeLabel(type) {
  return TASK_TYPE_LABELS[type] || `Type ${type}`;
}

function taskStatusBadge(status) {
  const [cls, label] = TASK_STATUS_BADGE[status] || ['queued', `Status ${status}`];
  return `<span class="badge badge-${cls}">${label}</span>`;
}

function taskTypeBadge(type) {
  const map = { 0: 'generic', 1: 'queued', 2: 'downloading' };
  const cls = map[type] || 'queued';
  return `<span class="badge badge-${cls}">${taskTypeLabel(type)}</span>`;
}

function gotoTasksPageAndFilterVideo(videoId) {
  clearTaskFilters(true);
  
  taskPage = 0;
  document.getElementById('taskVideoFilter').value = videoId;
  
  showPage(`tasks?video=${videoId}`);
  loadTasks();
  renderTaskPagination();
}
function gotoTasksPageAndFilterChannel(channelId) {
  clearTaskFilters(true);
  
  taskPage = 0;
  document.getElementById('taskChannelFilter').value = channelId;
  
  showPage(`tasks?channel=${channelId}`);
  loadTasks();
  renderTaskPagination();
  
  const channel = getChannelFromId(channelId);
  if (channel) {
    if (channel.active_task) {
      selectTask(channel.active_task);
    }
  }
}
function gotoVideosPageAndFilterChannel(channelId) {
  clearVideoFilters(true);
  
  videoPage = 0;
  document.getElementById('videoChannelFilter').value = channelId;
  
  showPage(`videos?channel=${channelId}`);
  loadVideos();
  renderVideoPagination();
}

function updateTasksTabTitle() {
  if (lastPageOpen != "tasks") return;
  
  const channelId = document.getElementById('taskChannelFilter').value
  
  const channel = getChannelFromId(channelId);
  if (channel) {
    setTitle(`Logs: ${channel.name}`)
  } else {
    setTitle("Logs")
  }
}

function onTaskFilterChange(dontLoad) {
  const channelId = document.getElementById('taskChannelFilter').value
  const videoEl = document.getElementById('taskVideoFilter');
  const videoId = videoEl.value;
  if (videoId) {
    videoEl.type = "text";
  } else {
    videoEl.type = "hidden";
    videoEl.value = '';     // why THE FUCK does setting the type to hidden change the value back to it's original value???
  }
  
  if (channelId) {
    let url = `/tasks?channel=${channelId}`;
    if (videoId) {
      url += `&video=${videoId}`;
    }
    window.history.replaceState(null, "", url);
  } else {
    if (videoId) {
      window.history.replaceState(null, "", `/tasks?video=${videoId}`);
    } else {
      window.history.replaceState(null, "", `/tasks`);
    }
  }
  updateTasksTabTitle();
  
  if (!dontLoad) {
    taskPage = 0;
    loadTasks();
  }
}

function clearTaskFilters(dontLoadTasks) {
  //document.getElementById('taskSearch').value = '';
  document.getElementById('taskStatusFilter').value = '';
  document.getElementById('taskTypeFilter').value = '';
  //document.getElementById('taskChannelFilter').value = '';
  document.getElementById('taskVideoFilter').value = '';
  document.getElementById('taskOrderBy').value = 'end_time';
  document.getElementById('taskOrderDirection').value = '-1';
  taskPage = 0;
  onTaskFilterChange(true);     // I HATE THIS CODE BASE FFS
  if (!areTasksLoading && !dontLoadTasks) {
    loadTasks();
  }
}

async function cancelTask(id) {
  try {
    await API.post(`/api/cancel-task/${id}`, {});
    loadTasks();
  } catch (err) {
    showToast(`Failed to cancel task: ${err.message}`, 'error');
  }
}

async function loadTasks() {
  areTasksLoading = true;
  try {
    const statusFilter  = document.getElementById('taskStatusFilter').value;
    const typeFilter    = document.getElementById('taskTypeFilter').value;
    const channelFilter = document.getElementById('taskChannelFilter').value;
    const videoFilter   = document.getElementById('taskVideoFilter').value.trim();
    const orderBy  = document.getElementById('taskOrderBy').value;
    const orderDir = document.getElementById('taskOrderDirection').value;
    
    let url = `/api/tasks?limit=${TASK_PAGE_SIZE}&offset=${taskPage * TASK_PAGE_SIZE}`;
    if (statusFilter !== '') {
      url += `&status=${statusFilter}`;
    }
    if (typeFilter !== '') {
      url += `&type=${typeFilter}`;
    }
    if (channelFilter !== '') {
      url += `&from_channel=${channelFilter}`;
    }
    if (videoFilter !== '') {
      url += `&from_video=${videoFilter}`;
    }
    if (orderBy) {
      url += `&order_by=${orderBy}`;
    }
    url += `&order_direction=${orderDir}`;
    
    const data = await API.get(url);
    allTasks = data.tasks || data;
    if (data.stats) {
      taskStatsData = data.stats;
      taskTotalCount = data.stats.total || allTasks.length;
    }
    
    if (selectedTaskId !== null) {
      allTasks.forEach(t => {
      if (t.id == selectedTaskId) {
          selectedTask = t;
        }
      })
    }
    
    renderTasks();
    renderTaskStats();
    renderTaskPagination();
  } catch (err) {
    showToast(`Failed to load tasks: ${err.message}`, 'error');
  }
  areTasksLoading = false;
}

function renderTaskStats() {
  const container = document.getElementById('taskStats');
  const stats = taskStatsData;
  const total = stats.total || taskTotalCount || allTasks.length;
  const running = stats.running || 0;
  const failed = stats.failed || 0;
  const finished = stats.finished || 0;
  
  container.innerHTML = `
    <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--warning)">${running}</div><div class="stat-label">Running</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--success)">${finished}</div><div class="stat-label">Finished</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--danger)">${failed}</div><div class="stat-label">Failed</div></div>
  `;
}

function renderTasks() {
  const container = document.getElementById('taskList');
  if (allTasks.length === 0) {
    container.innerHTML = '<div class="loading">No logs/ tasks found.</div>';
    return;
  }
  container.innerHTML = allTasks.map(t => {
    formatedDuration = getFormatedTaskDuration(t);
    
    let channelInfo = '';
    if (t.basic_channel_info && t.basic_channel_info.name) {
      const chName = escHtml(t.basic_channel_info.name);
      const chId = t.basic_channel_info.id;
      channelInfo = `<a href="#" class="task-channel" target="_blank" onclick="event.preventDefault();event.stopPropagation();document.getElementById('taskChannelFilter').value='${chId}';taskPage=0;loadTasks();" title="Filter by this channel">${chName}</a>`;
    }
    let cancelTaskHtml = '';
    if (t.status == 0) {
      // This task is running
      let cancelText = "Cancel Task";
      if (t.type == 2) {
        // This is a download task
        cancelText = "Cancel Download";
      }
      cancelTaskHtml = `<a href="#" class="task-channel" target="_blank" onclick="event.preventDefault();cancelTask('${t.id}');">${cancelText}</a>`;
    }
    let videoInfo = '';
    if (t.basic_video_info) {
      const vTitle = escHtml(t.basic_video_info.title || "");
      const vId = t.basic_video_info.id;
      const vUrl = t.basic_video_info.url;
      const displayTitle = vTitle.length > 40 ? vTitle.slice(0, 40) + '...' : vTitle;
      videoInfo = `<span class="task-video" title="${escHtml(vTitle)}">${displayTitle}</span>`;
      if (vUrl) {
        videoInfo += ` <a href="${escHtml(vUrl)}" target="_blank" class="task-video-link" onclick="event.stopPropagation()" title="Open video">[VideoLink]</a>`;
      }
    }
    return `
    <div class="task-item ${selectedTaskId === t.id ? 'active' : ''}" onclick="selectTask('${t.id}')">
      <div class="task-item-header">
        <span class="task-item-title">${escHtml(t.title || t.id)}</span>
        <span class="task-item-time">${formatRelative(t.start_time)}</span>
        <span class="task-item-time">${formatedDuration}</span>
      </div>
      <div class="task-item-status">
        ${taskStatusBadge(t.status)}
        ${taskTypeBadge(t.type)}
      </div>
      ${cancelTaskHtml || "<br>"}
      ${channelInfo ? `<div class="task-item-channel">${channelInfo}</div>` : ''}
      ${videoInfo ? `<div class="task-item-video">${videoInfo}</div>` : ''}
    </div>
  `;
  }).join('');
}

function selectTask(id) {
  selectedTaskId = id;
  //selectedTaskType = type;
  allTasks.forEach(t => {
    if (t.id == selectedTaskId) {
      selectedTask = t;
    }
  });
  renderTasks();
  
  const outputContainer = document.getElementById('taskOutputContent');
  const statusEl = document.getElementById('taskOutputStatus');
  const taskTitleEl = document.getElementById('taskOutputTitle');
  
  outputContainer.innerHTML = '<span class="terminal-prompt">Loading output...</span>';
  statusEl.textContent = '';
  
  if (taskTitleEl) {
    if (selectedTask) {
      formatedDuration = getFormatedTaskDuration(selectedTask);
      taskTitleEl.textContent = (selectedTask.title || selectedTask.id) + " " + formatedDuration;
    } else {
      taskTitleEl.textContent = "Task Output - " + id;
    }
  }
  
  // Start polling for real-time output
  startRealtimePolling();
  
  // Also load the full output from the task list
  loadFullTaskOutput(id);
}

async function loadFullTaskOutput(taskId) {
  const outputContainer = document.getElementById('taskOutputContent');
  
  try {
    const task = allTasks.find(t => t.id === taskId);
    if (task && task.output !== null) {
      outputContainer.innerHTML = formatTerminalOutput(task.output, task.status, task.run_args);
      // Auto-scroll to bottom
      outputContainer.scrollTop = outputContainer.scrollHeight;
    }
  } catch (err) {
    outputContainer.innerHTML = `<span class="terminal-line-error">Error loading output: ${err.message}</span>`;
  }
}

function formatTerminalOutput(text, status, run_args) {
  const lines = text.split('\n');
  const formatted = lines.map(line => {
    let cls = 'terminal-line';
    if (line.trim() === "") {
      return `<span class="${cls}"><br></span>`
    }
    return `<span class="${cls}">${escHtml(line)}</span>`;
  }).join('\n');
  
  let runArgsContent = `<span class="terminal-line">${escHtml(">") + escHtml(run_args)}</span><br><span class="terminal-line"></span>`;
  if (run_args === "") {
    runArgsContent = ""
  }
  return runArgsContent + formatted;
}

function startRealtimePolling() {
  if (realtimeOutputTimer) {
    clearInterval(realtimeOutputTimer);
    realtimeOutputTimer = null;
  }
  
  let canRefresh = true;
  
  realtimeOutputTimer = setInterval(async () => {
    if (!selectedTaskId || !selectedTask || selectedTask.status !== 0 || lastPageOpen !== "tasks" || !canRefresh) {
      return;
    }
    if (document.hidden) {
      return;
    }
    
    const statusEl = document.getElementById('taskOutputStatus');
    const taskTitleEl = document.getElementById('taskOutputTitle');
    canRefresh = false;
    try {
      const res = await fetch(`/api/get-realtime-task-output/${selectedTaskId}`);
      if (!res.ok) {
        if (statusEl) statusEl.textContent = 'No output available';
        return;
      }
      
      const formatedDuration = getFormatedTaskDuration(selectedTask);
      
      const text = await res.text();
      const outputContainer = document.getElementById('taskOutputContent');
      outputContainer.innerHTML = formatTerminalOutput(text, selectedTask.status, selectedTask.run_args);
      
      // Auto-scroll to bottom
      outputContainer.scrollTop = outputContainer.scrollHeight;
      
      if (statusEl) statusEl.textContent = 'Live';
      if (taskTitleEl) {
        taskTitleEl.textContent = (selectedTask.title || selectedTask.id) + " " + formatedDuration;
      }
    } catch (err) {
      if (statusEl) statusEl.textContent = 'Poll error';
    }
    canRefresh = true;
  }, 500);
}

function stopRealtimePolling() {
  if (realtimeOutputTimer) {
    clearInterval(realtimeOutputTimer);
    realtimeOutputTimer = null;
  }
  selectedTaskId = null;
  selectedTaskType = null;
  const statusEl = document.getElementById('taskOutputStatus');
  const taskTitleEl = document.getElementById('taskOutputTitle');
  const outputContainer = document.getElementById('taskOutputContent');
  if (statusEl) statusEl.textContent = '';
  if (taskTitleEl) {
    taskTitleEl.textContent = "Task Output"
  }
  if (outputContainer) {
    outputContainer.innerHTML = '<span class="terminal-prompt">Select a task to view its output</span>';
  }
}

function navigateToChannel(channelId) {
  document.getElementById('videoChannelFilter').value = channelId;
  videoPage = 0;
  loadVideos();
  showPage('videos');
}

function escHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function setupModalClickExit(modalId, callback) {
  let backdropMouseDown = false;
  const overlay = document.getElementById(modalId)
  if (!overlay) {
    console.log("Could not find modal overlay '" + modalId + "'")
    return
  }
  overlay.addEventListener('mousedown', (event) => {
    backdropMouseDown = event.target === overlay;
  });
  
  overlay.addEventListener('click', (event) => {
    if (backdropMouseDown && event.target === overlay) {
      callback();
    }
  
    backdropMouseDown = false;
  });
}
setupModalClickExit("channelModal", closeChannelModal)
setupModalClickExit("videoDetailsModal", closeVideoDetailsModal)
setupModalClickExit("addVideosModal", closeAddVideosModal)

window.addEventListener('popstate', function (e) {
  const tab = window.location.pathname.replace(/^\//, '') || '';
  showPage(tab, true);
  
  updateVideosTabTitle()
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    closeVideoDetailsModal();
    closeChannelModal();
    closeAddVideosModal();
  }
});

async function init() {
  await loadConfig();
  await loadChannels();
  
  const lastTab = window.location.pathname.replace(/^\//, '') || 'channels';
  showPage(lastTab + window.location.search, true)
  
  updateChannelFilters();
  let nextSoftChannelsLoad = (new Date()).getTime() + 5000;
  // Auto refresh channels
  setInterval(() => {
    if (document.hidden) {
      return;
    }
    
    const channelsPage = document.getElementById('page-channels');
    if (channelsPage.classList.contains('active')) {
      softRenderChannels();
      
      if (!areChannelsLoading && (new Date()).getTime() > nextSoftChannelsLoad) {
        nextSoftChannelsLoad = (new Date()).getTime() + 5000
        loadChannels(true);
      }
    }
  }, 200);
  
  // Auto refresh videos every 10s
  setInterval(() => {
    if (areVideosLoading || document.hidden) {
      return;
    }
    
    const videosPage = document.getElementById('page-videos');
    if (videosPage.classList.contains('active')) {
      loadVideos();
    }
  }, 10_000);
  
  // Auto refresh tasks every 7.5s
  setInterval(() => {
    if (areTasksLoading || document.hidden) {
      return;
    }
    
    const tasksPage = document.getElementById('page-tasks');
    if (tasksPage.classList.contains('active')) {
      loadTasks();
    }
  }, 5_000);
  
  
  // Check search updates every 300ms (at best)
  setInterval(() => {
    if (areVideosLoading) {
      return;
    }
    if (videoSearchDidUpdate) {
      videoSearchDidUpdate = false;
      videoPage = 0;
      
      loadVideos();
    }
  }, 300);
}

async function quickToggleDisableAllChannels(cb) {
  try {
    let isDisabled = cb.checked
    await API.patch("/api/config", {
        AllChannels_Disabled: isDisabled
      }
    );
    
    programConfig.AllChannels_Disabled = isDisabled;
  } catch (err) {
    showToast(`Failed to update AllChannels_Disabled: ${err.message}`, 'error');
    loadConfig();
  }
}

document.addEventListener("visibilitychange", () => {
  if (!document.hidden) {
    softRenderChannels();
    
    const videosPage = document.getElementById('page-videos');
    if (!areVideosLoading && videosPage.classList.contains('active')) {
      loadVideos();
    }
    
    const tasksPage = document.getElementById('page-tasks');
    if (!areTasksLoading && tasksPage.classList.contains('active')) {
      loadTasks();
    }
  }
});

init();
