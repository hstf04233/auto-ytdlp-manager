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
  async put(url, body) {
    const res = await fetch(url, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API PUT ${url}: ${res.status} - ${text}`);
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
let allChannels = [];
let allVideos = [];
let videoPage = 0;
const VIDEO_PAGE_SIZE = 50;

// ========== Navigation ==========
document.querySelectorAll('.sidebar nav a').forEach(link => {
  link.addEventListener('click', e => {
    e.preventDefault();
    const page = link.dataset.page;
    document.querySelectorAll('.sidebar nav a').forEach(l => l.classList.remove('active'));
    link.classList.add('active');
    document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
    document.getElementById(`page-${page}`).classList.add('active');
    if (page === 'videos') loadVideos();
  });
});

// ========== Toast ==========
function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 3500);
}

// ========== Channel helpers ==========
function statusBadge(type) {
  return type ? '<span class="badge badge-type-live">Live</span>' : '<span class="badge badge-type-videos">Videos</span>';
}

function qualityLabel(q) {
  if (q === 0) return 'Best';
  return `${q}p`;
}

function intervalLabel(s) {
  if (s <= 0) return 'Never';
  if (s < 3600) return `${s}s`;
  if (s < 86400) return `${s / 3600 | 0}h`;
  return `${s / 86400 | 0}d`;
}

function formatDate(ts) {
  if (!ts) return '\u2014';
  return new Date(ts * 1000).toLocaleDateString();
}

function formatDuration(sec) {
  if (!sec || sec <= 0) return '\u2014';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.floor(sec % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

function videoStatusBadge(status) {
  const map = {
    0: ['queued', 'Queued'],
    1: ['downloading', 'Downloading'],
    2: ['downloaded', 'Downloaded'],
    3: ['failed', 'Failed'],
  };
  const [cls, label] = map[status] || ['queued', 'Unknown'];
  return `<span class="badge badge-${cls}">${label}</span>`;
}

function videoTypeBadge(vtype) {
  const map = {
    0: ['video', 'Video'],
    1: ['live', 'Live'],
    2: ['waslive', 'Was Live'],
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
        <button class="btn btn-danger btn-sm" onclick="deleteChannel('${ch.id}')">Delete</button>
      </div>
    </div>
  `).join('');
}

async function toggleChannel(id, enabled) {
  try {
    await API.put(`/api/channels/${id}`, { enabled });
    showToast(`Channel ${enabled ? 'enabled' : 'disabled'}`, 'success');
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

async function deleteChannel(id) {
  if (!confirm('Delete this channel?')) return;
  try {
    await API.del(`/api/channels/${id}`);
    showToast('Channel deleted!', 'success');
    loadChannels();
  } catch (err) {
    showToast(`Failed to delete: ${err.message}`, 'error');
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
  document.getElementById('channelFullCheckInterval').value = ch.full_check_interval || '';
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
  const fullCheckInterval = document.getElementById('channelFullCheckInterval').value ? parseInt(document.getElementById('channelFullCheckInterval').value) : 86400;

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
      await API.put(`/api/channels/${id}`, patch);
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
  try {
    const statusFilter = document.getElementById('videoStatusFilter').value;
    const channelFilter = document.getElementById('videoChannelFilter').value;
    let url = `/api/videos?limit=${VIDEO_PAGE_SIZE}&offset=${videoPage * VIDEO_PAGE_SIZE}`;
    if (statusFilter !== '') {
      url += `&status=${statusFilter}`;
    }
    if (channelFilter !== '') {
      url += `&from_channel=${channelFilter}`;
    }
    const data = await API.get(url);
    allVideos = data.videos || data;
    renderVideos();
    renderVideoPagination();
    updateVideoStats();
  } catch (err) {
    showToast(`Failed to load videos: ${err.message}`, 'error');
  }
}

function renderVideos() {
  const container = document.getElementById('videosList');
  const filtered = getFilteredVideos();

  if (filtered.length === 0) {
    container.innerHTML = '<div class="loading">No videos found.</div>';
    return;
  }

  container.innerHTML = filtered.map(v => {
    const thumbUrl = `https://img.youtube.com/vi/${v.id}/mqdefault.jpg`;
    const channel = allChannels.find(c => c.id === v.from_channel);
    return `
      <div class="card video-card">
        <div class="video-thumb">
          <img src="${thumbUrl}" alt="" onerror="this.style.display='none';this.parentElement.textContent='No thumbnail'">
        </div>
        <div class="video-info">
          <h3 title="${escHtml(v.title)}">${escHtml(v.title)}</h3>
          <p>${channel ? escHtml(channel.name) : 'Unknown Channel'}</p>
          <p>Released: ${formatDate(v.releaseDate)} \u00b7 ${formatDuration(v.duration)}</p>
        </div>
        <div class="video-actions">
          ${videoStatusBadge(v.status)}
          ${v.video_type !== undefined ? videoTypeBadge(v.video_type) : ''}
          <a href="${escHtml(v.url)}" target="_blank" class="btn btn-secondary btn-sm" title="Open on YouTube">Open</a>
        </div>
      </div>
    `;
  }).join('');
}

function getFilteredVideos() {
  const search = document.getElementById('videoSearch').value.toLowerCase();
  const statusFilter = document.getElementById('videoStatusFilter').value;
  const channelFilter = document.getElementById('videoChannelFilter').value;

  return allVideos.filter(v => {
    if (search && !v.title.toLowerCase().includes(search)) return false;
    //if (statusFilter !== '' && String(v.status) !== statusFilter) return false;
    if (channelFilter && v.from_channel !== channelFilter) return false;
    return true;
  });
}

function filterVideos() {
  renderVideos();
}

function renderVideoPagination() {
  const container = document.getElementById('videoPagination');
  const hasMore = allVideos.length >= VIDEO_PAGE_SIZE;

  container.innerHTML = `
    <button onclick="videoPage--;loadVideos()" ${videoPage <= 0 ? 'disabled' : ''}>← Prev</button>
    <span style="padding:6px 14px;color:var(--text-secondary)">Page ${videoPage + 1}</span>
    <button onclick="videoPage++;loadVideos()" ${!hasMore ? 'disabled' : ''}>Next →</button>
  `;
}

function updateVideoStats() {
  const container = document.getElementById('videoStats');
  
  const total = allVideos.length;
  const queued      = allVideos.filter(v => v.status === 0).length;
  const downloading = allVideos.filter(v => v.status === 1).length;
  const downloaded  = allVideos.filter(v => v.status === 2).length;
  const failed      = allVideos.filter(v => v.status === 3).length;

  container.innerHTML = `
    <div class="stat-card"><div class="stat-value">${total}</div><div class="stat-label">Showing</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--info)">${queued}</div><div class="stat-label">Queued</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--warning)">${downloading}</div><div class="stat-label">Downloading</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--success)">${downloaded}</div><div class="stat-label">Downloaded</div></div>
    <div class="stat-card"><div class="stat-value" style="color:var(--danger)">${failed}</div><div class="stat-label">Failed</div></div>
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

// ========== Utility ==========
function escHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// ========== Init ==========
async function init() {
  await loadChannels();
  updateChannelFilter();
  // Auto-refresh videos every 10s
  setInterval(() => {
    const videosPage = document.getElementById('page-videos');
    if (videosPage.classList.contains('active')) {
      loadVideos();
    }
  }, 10000);
}

init();
