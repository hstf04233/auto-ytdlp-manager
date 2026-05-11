// ========== API helpers ==========
const API = {
  async get(url) {
    const res = await fetch(url);
    if (!res.ok) throw new Error(`API GET ${url}: ${res.status}`);
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
      throw new Error(`API POST ${url}: ${res.status} - ${text}`);
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
      throw new Error(`API PATCH ${url}: ${res.status} - ${text}`);
    }
    return res.json();
  },
  async del(url) {
    const res = await fetch(url, {
      method: 'DELETE',
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API DELETE ${url}: ${res.status} - ${text}`);
    }
    return res.json();
  },
};

// ========== State ==========

// These are to stop spamming the api endpoints until the last call is finished!
let areVideosLoading = false;
let areTasksLoading  = false;

let allChannels = [];
let allVideos = [];
let videoPage = 0;
let videoTotalCount = 0;
let videoStats = {};
const VIDEO_PAGE_SIZE = 40;

let lastPageOpen = ''
let _pgCallbacks = {}
let _pgId = 0

function _registerPgCbs(cbs) {
  const id = ++_pgId
  _pgCallbacks[id] = cbs
  return id
}

function showPage(page, dontSaveHistory) {
  if (!dontSaveHistory) {
    if (page != lastPageOpen) {
      history.pushState({}, '', '/' + page);
      lastPageOpen = page
    }
  }
  
  document.querySelectorAll('.sidebar nav a').forEach(l => l.classList.remove('active'));
  document.querySelectorAll('.sidebar nav a').forEach(l => {
    if (l.dataset.page == page) {
      l.classList.add('active');
    }
  })
  
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  let pageDoc = document.getElementById(`page-${page}`)
  if (pageDoc) {
    pageDoc.classList.add('active');
  }
  
  if (page === 'videos') {
    if (!areVideosLoading) {
      loadVideos();
    }
  }
  if (page === 'tasks') {
    stopRealtimePolling();
    if (!areTasksLoading) {
      loadTasks();
    }
  } else {
    stopRealtimePolling();
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
    return '<span class="badge badge-type-videos">List and ignore</span>';
  }
  return '<span class="badge badge-type-videos">Videos?</span>';
}

function qualityLabel(q) {
  if (q === 0) return 'Highest';
  return `${q}p`;
}

function intervalLabel(s) {
  if (s <= 0) return 'Never';
  // TODO: add more detail to dis...
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${(s / 60).toFixed(2)}m`;
  if (s < 86400) return `${Math.floor(s / 3600 | 0).toFixed(2)}h`;
  return `${Math.floor(s / 86400 | 0)}d`;
}

function formatDate(ts) {
  if (!ts) return '\u2014';
  return new Date(ts * 1000).toLocaleDateString();
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
  console.log("closeStatusDropdown");
}

function toggleStatusDropdown(videoId, currentStatus, buttonEl) {
  if (statusDropdownIsActive) {
    closeStatusDropdown();
    return;
  }
  statusDropdownIsActive = true;
  const rect = buttonEl.getBoundingClientRect();
  const dropdown = document.createElement('div');
  dropdown.className = 'status-dropdown';
  dropdown.style.top = (rect.bottom + 4) + 'px';
  dropdown.style.left = rect.left + 'px';
  
  const statuses = [
    [0, 'Queued'],
    [1, 'Downloading'],
    [2, 'Downloaded'],
    [3, 'Failed'],
    [4, 'Ignored'],
  ];
  dropdown.innerHTML = statuses.map(([val, label]) =>
    `<div class="status-option ${val === currentStatus ? 'active' : ''}" data-video-id="${videoId}" data-new-status="${val}">${label}${val === currentStatus ? ' ✓' : ''}</div>`
  ).join('');
  
  dropdown.addEventListener('click', (e) => {
    e.stopPropagation();
    const option = e.target.closest('.status-option');
    if (option) {
      changeVideoStatus(option.dataset.videoId, parseInt(option.dataset.newStatus));
      closeStatusDropdown()
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

// ========== Channels ==========
async function loadChannels() {
  try {
    const data = await API.get('/api/channels');
    allChannels = data.channels || data;
    renderChannels();
  } catch (err) {
    showToast(`Failed to load channels: ${err.message}`, 'error');
  }
}

function renderChannels() {
  const container = document.getElementById('channelsList');
  const statsContainer = document.getElementById('channelStats');

  const total = allChannels.length;
  const enabled = allChannels.filter(c => c.enabled).length;
  const disabled = total - enabled;

  statsContainer.innerHTML = `
    <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--success)">${enabled}</div><div class="stat-label">Enabled</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--text-secondary)">${disabled}</div><div class="stat-label">Disabled</div></div>
  `;

  if (allChannels.length === 0) {
    container.innerHTML = '<div class="loading">No channels yet. Add one to get started!</div>';
    return;
  }

  container.innerHTML = allChannels.map(ch => `
    <div class="card channel-card">
      <div class="channel-info">
        <h3>${escHtml(ch.name)}</h3>
        <p>${escHtml(ch.url)}</p>
        <div class="channel-meta">
          ${statusBadge(ch.type)}
          <span>Quality: ${qualityLabel(ch.quality_select)}</span>
          <span>Check: ${intervalLabel(ch.check_interval)}</span>
          <span>Full: ${intervalLabel(ch.full_check_interval)}</span>
          ${ch.download_dir ? `<span>Dir: ${escHtml(ch.download_dir)}</span>` : ''}
        </div>
      </div>
      <div class="video-actions">
        <label class="toggle">
          <input type="checkbox" ${ch.enabled ? 'checked' : ''} onchange="toggleChannel('${ch.id}', this.checked)">
          <span class="toggle-slider"></span>
        </label>
        <button class="btn btn-secondary btn-sm" onclick="openEditChannelModal('${ch.id}')">Edit</button>
        <button class="btn btn-danger btn-sm" onclick="deleteChannel('${ch.id}', '${ch.name}')">Delete</button>
      </div>
    </div>
  `).join('');
}

async function toggleChannel(id, enabled) {
  try {
    await API.patch(`/api/channels/${id}`, { enabled });
    showToast(`Channel ${enabled ? 'enabled' : 'disabled'}`, 'success');
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

async function deleteChannel(id, name) {
  if (!confirm("Delete channel \"" + name + "\"?")) return;
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
  document.getElementById('channelFullCheckInterval').value = '';
  document.getElementById('channelSubmitBtn').textContent = 'Add Channel';
  document.getElementById('channelModal').classList.add('active');
}

function openEditChannelModal(id) {
  const ch = allChannels.find(c => c.id === id);
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
  document.getElementById('channelFullCheckInterval').value = ch.full_check_interval !== -1 ? ch.full_check_interval : '';
  document.getElementById('channelSubmitBtn').textContent = 'Save Changes';
  document.getElementById('channelModal').classList.add('active');
}

function closeChannelModal() {
  document.getElementById('channelModal').classList.remove('active');
}

async function saveChannel(e) {
  e.preventDefault();
  const id = document.getElementById('channelId').value;
  const name = document.getElementById('channelName').value.trim();
  const url = document.getElementById('channelUrl').value.trim();
  const type = parseInt(document.getElementById('channelType').value);
  const quality = parseInt(document.getElementById('channelQuality').value);
  const downloadDir = document.getElementById('channelDownloadDir').value.trim();
  const outputTemplate = document.getElementById('channelOutputTemplate').value.trim();
  const checkInterval = document.getElementById('channelCheckInterval').value ? parseInt(document.getElementById('channelCheckInterval').value) : 3600;
  const fullCheckInterval = document.getElementById('channelFullCheckInterval').value ? parseInt(document.getElementById('channelFullCheckInterval').value) : 172800;

  const body = {
    name,
    url,
    type,
    quality_select: quality,
    download_dir: downloadDir,
    output_template: outputTemplate,
    check_interval: checkInterval,
    full_check_interval: fullCheckInterval,
  };

  try {
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
      patch.full_check_interval = fullCheckInterval;
      await API.patch(`/api/channels/${id}`, patch);
      showToast('Channel updated!', 'success');
    } else {
      await API.post('/api/channels', body);
      showToast('Channel added!', 'success');
    }
    closeChannelModal();
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
      // TODO: This is bad and inappropriate!!! sanitize this out so the user can't do some funky shi
      url += `&search_query=${search}`
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
  areVideosLoading = false
}

function renderVideos() {
  const container = document.getElementById('videosList');
  const filtered = getFilteredVideos();

  if (filtered.length === 0) {
    container.innerHTML = '<div class="loading">No videos found.</div>';
    return;
  }

  container.innerHTML = filtered.map(v => {
    var thumbUrl = `https://img.youtube.com/vi/${v.id}/mqdefault.jpg`;
    const channel = allChannels.find(c => c.id === v.from_channel);
    const refreshDisabled = v.refresh_state ? 'disabled style="opacity:0.5;cursor:not-allowed"' : '';
    const refreshTitle = v.refresh_state ? 'Refreshing...' : 'Refresh metadata';
    return `
      <div class="card video-card">
        <div class="video-thumb">
          <img src="${thumbUrl}" alt="" onerror="this.style.display='none';this.parentElement.textContent='No thumbnail'">
        </div>
        <div class="video-info">
          <h3 title="${escHtml(v.title)}">${escHtml(v.title)}</h3>
          <p>From: <a href="#" onclick="event.preventDefault();document.getElementById('videoChannelFilter').value='${v.from_channel}';videoPage=0;loadVideos();">${channel ? escHtml(channel.name) : 'Unknown Channel'}</a></p>
          <p>Released: ${formatDate(v.release_date)} \u00b7 ${formatDuration(v.duration)}</p>
          <p>${escHtml(v.availability)}</p>
          <p>Added ${formatRelative(v.added_at)} \u00b7 Updated ${formatRelative(v.updated_at)}</p>
        </div>
        <div class="video-actions">
          ${videoStatusBadge(v.id, v.status)}
          ${v.video_type !== undefined ? videoTypeBadge(v.video_type) : ''}
          <a href="${escHtml(v.url)}" target="_blank" class="btn btn-secondary btn-sm" title="Open video on YouTube">Open Video</a>
          <button class="btn btn-secondary btn-sm" ${refreshDisabled} onclick="refreshVideoInfo('${v.id}')" title="${refreshTitle}">${v.refresh_state ? 'Refreshing...' : 'Refresh'}</button>
          <button class="btn btn-danger btn-sm" onclick="deleteVideo('${v.id}')">Delete</button>
        </div>
      </div>
    `;
  }).join('');
}

function getFilteredVideos() {
  //const search = document.getElementById('videoSearch').value.toLowerCase();
  const statusFilter = document.getElementById('videoStatusFilter').value;
  const channelFilter = document.getElementById('videoChannelFilter').value;

  return allVideos.filter(v => {
    //if (search && !v.title.toLowerCase().includes(search)) return false;
    //if (statusFilter !== '' && String(v.status) !== statusFilter) return false;
    //if (channelFilter && v.from_channel !== channelFilter) return false;
    return true;
  });
}

function clearVideoFilters() {
  document.getElementById('videoSearch').value = '';
  document.getElementById('videoStatusFilter').value = '';
  document.getElementById('videoChannelFilter').value = '';
  videoPage = 0;
  if (!areVideosLoading) {
    loadVideos();
  }
}

let videoSearchDidUpdate = false;
function videoSearchFilterUpdate() {
  videoSearchDidUpdate = true;
  // videoPage = 0;
  // loadVideos();
}
function onVideoFilterChange() {
  videoPage = 0;
  loadVideos();
}

function renderVideoPagination() {
  const container_top    = document.getElementById('videoPagination-top');
  const container_bottom = document.getElementById('videoPagination-bottom');
  const totalPages = Math.ceil(videoTotalCount / VIDEO_PAGE_SIZE) || 1;
  const cbsId = _registerPgCbs({
    prev: () => { if (!areVideosLoading && videoPage > 0) { videoPage--; loadVideos(); } },
    next: () => { if (!areVideosLoading) { videoPage++; loadVideos(); } },
    page: (p) => { if (!areVideosLoading) { videoPage = p; loadVideos(); } },
  });
  
  const paginationHtml = buildPaginationHTML({
    currentPage: videoPage,
    totalPages,
    totalCount: videoTotalCount,
    currentItems: allVideos.length,
    label: 'videos',
    cbsId: cbsId,
  });
  
  container_top.innerHTML = paginationHtml;
  container_bottom.innerHTML = paginationHtml;
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
  
  const singlePageMsg = `All ${label} shown (page 1 of 1)`;
  const countMsg = `Showing ${currentItems} ${label} out of ${totalCount}`;
  
  if (totalPages <= 1) {
    return `<span class="single-page-msg">${singlePageMsg}</span>`;
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
  const cbsId = _registerPgCbs({
    prev: () => { if (!areTasksLoading && taskPage > 0) { taskPage--; loadTasks(); } },
    next: () => { if (!areTasksLoading) { taskPage++; loadTasks(); } },
    page: (p) => { if (!areTasksLoading) { taskPage = p; loadTasks(); } },
  });
  
  console.log("task total: " + taskTotalCount)
  console.log("task totalPages: " + totalPages)
  
  const paginationHtml = buildPaginationHTML({
    currentPage: taskPage,
    totalPages,
    totalCount: taskTotalCount,
    currentItems: allTasks.length,
    label: 'tasks',
    cbsId: cbsId,
  });
  
  container_top.innerHTML = paginationHtml;
  container_bottom.innerHTML = paginationHtml;
}

function updateVideoStats() {
  const container = document.getElementById('videoStats');
  
  const total = videoStats.total || 0;
  const queued      = videoStats.total_queued || 0;
  const downloading = videoStats.total_downloading || 0;
  const downloaded  = videoStats.total_downloaded || 0;
  const failed      = videoStats.total_failed || 0;
  const ignored     = videoStats.total_ignored || 0;

  container.innerHTML = `
    <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Total</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--info)">${queued}</div><div class="stat-label">Queued</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--warning)">${downloading}</div><div class="stat-label">Downloading</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--success)">${downloaded}</div><div class="stat-label">Downloaded</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--danger)">${failed}</div><div class="stat-label">Failed</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--text-secondary)">${ignored}</div><div class="stat-label">Ignored</div></div>
  `;
}

// ========== Channel filter dropdown ==========
function updateChannelFilter() {
  const select = document.getElementById('videoChannelFilter');
  const current = select.value;
  select.innerHTML = '<option value="">All Channels</option>';
  allChannels.forEach(ch => {
    const opt = document.createElement('option');
    opt.value = ch.id;
    opt.textContent = ch.name;
    if (ch.id === current) opt.selected = true;
    select.appendChild(opt);
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
const TASK_STATUS_LABELS = { 0: 'Running', 1: 'Failed', 2: 'Finished' };
const TASK_STATUS_BADGE = {
  0: ['downloading', 'Downloading...'],
  1: ['failed', 'Failed'],
  2: ['downloaded', 'Finished'],
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

async function loadTasks() {
  areTasksLoading = true;
  try {
    const url = `/api/tasks?limit=${TASK_PAGE_SIZE}&offset=${taskPage * TASK_PAGE_SIZE}`;
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
    container.innerHTML = '<div class="loading">No tasks found.</div>';
    return;
  }
  container.innerHTML = allTasks.map(t => `
    <div class="task-item ${selectedTaskId === t.id ? 'active' : ''}" onclick="selectTask('${t.id}')">
      <div class="task-item-header">
        <span class="task-item-id">${escHtml(t.id)}</span>
        <span class="task-item-time">${formatRelative(t.start_time)}</span>
      </div>
      <div class="task-item-status">
        ${taskStatusBadge(t.status)}
        ${taskTypeBadge(t.type)}
      </div>
    </div>
  `).join('');
}

function selectTask(id) {
  selectedTaskId = id;
  //selectedTaskType = type;
  allTasks.forEach(t => {
    if (t.id == selectedTaskId) {
      selectedTask = t;
    }
  })
  renderTasks();
  
  const outputContainer = document.getElementById('taskOutputContent');
  const statusEl = document.getElementById('taskOutputStatus');
  
  outputContainer.innerHTML = '<span class="terminal-prompt">Loading output...</span>';
  statusEl.textContent = '';
  
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
    /*
    if (status === 0) cls += ' terminal-line-running';
    if (/error|fail|exception/i.test(line)) cls = 'terminal-line-error';
    else if (/warn/i.test(line)) cls = 'terminal-line-warning';
    else if (/info|downloaded|saved/i.test(line)) cls = 'terminal-line-info';
    else if (/^\s*$/.test(line)) cls = 'terminal-line-dim';
    */
    return `<span class="${cls}">${escHtml(line)}</span>`;
  }).join('\n');
  
  const runArgsContent = `<span class="terminal-line">${escHtml(run_args)}</span><br><span class="terminal-line"></span>`;
  return runArgsContent + formatted;
}

function startRealtimePolling() {
  if (realtimeOutputTimer) {
    clearInterval(realtimeOutputTimer);
  }
  
  let canRefresh = true;
  
  realtimeOutputTimer = setInterval(async () => {
    if (!selectedTaskId || !selectedTask || selectedTask.status !== 0 || lastPageOpen !== "tasks" || !canRefresh) {
      return;
    }
    
    const statusEl = document.getElementById('taskOutputStatus');
    canRefresh = false;
    try {
      const res = await fetch(`/api/get-realtime-task-output/${selectedTaskId}`);
      if (!res.ok) {
        if (statusEl) statusEl.textContent = 'No output available';
        return;
      }
      
      const text = await res.text();
      const outputContainer = document.getElementById('taskOutputContent');
      outputContainer.innerHTML = formatTerminalOutput(text, selectedTask.status, selectedTask.run_args);
      
      // Auto-scroll to bottom
      outputContainer.scrollTop = outputContainer.scrollHeight;
      
      if (statusEl) statusEl.textContent = 'Live';
    } catch (err) {
      if (statusEl) statusEl.textContent = 'Poll error';
    }
    canRefresh = true;
  }, 200);
}

function stopRealtimePolling() {
  if (realtimeOutputTimer) {
    clearInterval(realtimeOutputTimer);
    realtimeOutputTimer = null;
  }
  selectedTaskId = null;
  selectedTaskType = null;
  const statusEl = document.getElementById('taskOutputStatus');
  const outputContainer = document.getElementById('taskOutputContent');
  if (statusEl) statusEl.textContent = '';
  if (outputContainer) {
    outputContainer.innerHTML = '<span class="terminal-prompt">Select a task to view its output</span>';
  }
}

function escHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

window.addEventListener('popstate', function (e) {
  const tab = window.location.pathname.replace(/^\//, '') || '';
  showPage(tab, true);
});

async function init() {
  await loadChannels();
  
  const lastTab = window.location.pathname.replace(/^\//, '') || 'channels';
  //setTimeout(() => showPage(lastTab), 1);
  showPage(lastTab)
  
  updateChannelFilter();
  // Auto-refresh videos every 10s
  setInterval(() => {
    if (areVideosLoading) {
      return;
    }
    
    const videosPage = document.getElementById('page-videos');
    if (videosPage.classList.contains('active')) {
      loadVideos();
    }
  }, 10_000);
  
  // Auto-refresh tasks every 7.5s
  setInterval(() => {
    if (areTasksLoading) {
      return;
    }
    
    const tasksPage = document.getElementById('page-tasks');
    if (tasksPage.classList.contains('active')) {
      loadTasks();
    }
  }, 7_500);
  
  
  // Check search updates every 100ms (at best)
  setInterval(() => {
    if (areVideosLoading) {
      return;
    }
    
    if (videoSearchDidUpdate) {
      videoSearchDidUpdate = false;
      videoPage = 0;
      
      loadVideos();
    }
  }, 100);
}

init()
