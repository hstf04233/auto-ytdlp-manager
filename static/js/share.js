// share.js - public, read only view of a shared channel group (/share/<token>).
// Works without logging in. Only talks to /api/public/... and /share-file/...

const SHARE_PAGE_SIZE = 24;
const shareToken = decodeURIComponent(location.pathname.replace(/^\/share\//, '').split('/')[0] || '');
const shareApiBase = `/api/public/groups/${encodeURIComponent(shareToken)}`;

const shareState = {
  group: null,
  subGroup: '',
  search: '',
  order: 'release_date:-1',
  downloadedOnly: false,
  page: 0,
  total: 0,
  videos: [],
};

let shareLoadSeq = 0;

function escHtml(str) {
  if (str === null || str === undefined) return '';
  const div = document.createElement('div');
  div.textContent = String(str);
  return div.innerHTML;
}

function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.textContent = message;
  container.appendChild(toast);
  setTimeout(() => toast.remove(), 4000);
}

async function shareGet(url) {
  const res = await fetch(url, { credentials: 'omit' });
  if (!res.ok) {
    const text = await res.text();
    const err = new Error(text || `${res.status}`);
    err.status = res.status;
    throw err;
  }
  return res.json();
}

function shareThumbnail(v) {
  if (v.stored_thumbnail) return `/db-image/${encodeURIComponent(v.stored_thumbnail)}`;
  if (v.origin_thumbnail_url) return v.origin_thumbnail_url;
  return `https://img.youtube.com/vi/${encodeURIComponent(v.id)}/mqdefault.jpg`;
}

function shareFileUrl(v, download) {
  return `/share-file/${encodeURIComponent(shareToken)}/${encodeURIComponent(v.id)}${download ? '?download=1' : ''}`;
}

function safeExternalUrl(url) {
  // Only allow http(s) links from video metadata.
  return /^https?:\/\//i.test(url || '') ? url : '';
}

function shareDurationText(v) {
  if (v.video_type == 2 && v.duration <= 1) return 'LIVE';
  return v.duration > 0 ? formatDuration(v.duration) : '';
}

// Availability values come from yt-dlp ("public", "unlisted", "needs_auth"...) or the
// archiver itself ("removed", "members-only", "age-restricted", "public?").
const SHARE_AVAILABILITY = {
  'public':          { label: 'Public',          tone: 'ok' },
  'public?':         { label: 'Public?',         tone: 'ok' },
  'unlisted':        { label: 'Unlisted',        tone: 'info' },
  'private':         { label: 'Private',         tone: 'danger' },
  'removed':         { label: 'Removed',         tone: 'danger' },
  'members-only':    { label: 'Members only',    tone: 'warn' },
  'subscriber_only': { label: 'Subscribers only', tone: 'warn' },
  'premium_only':    { label: 'Premium only',    tone: 'warn' },
  'needs_auth':      { label: 'Needs sign-in',   tone: 'warn' },
  'age-restricted':  { label: 'Age restricted',  tone: 'warn' },
};

function shareAvailabilityHTML(availability) {
  const raw = (availability || '').trim();
  if (!raw) return '';
  const known = SHARE_AVAILABILITY[raw.toLowerCase()];
  const label = known ? known.label : raw;
  const tone = known ? known.tone : 'neutral';
  return `<span class="share-avail share-avail-${tone}" title="Availability: ${escHtml(raw)}">${escHtml(label)}</span>`;
}

function pluralize(n, word) {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}

// ========== URL state ==========
function readShareUrl() {
  const p = new URLSearchParams(location.search);
  shareState.subGroup = p.get('sub') || '';
  shareState.search = p.get('q') || '';
  shareState.downloadedOnly = p.get('downloaded') === '1';
  const order = p.get('order');
  if (order && [...document.getElementById('shareOrder').options].some(o => o.value === order)) {
    shareState.order = order;
  }
  const page = parseInt(p.get('page'), 10);
  shareState.page = !isNaN(page) && page > 1 ? page - 1 : 0;
}

function writeShareUrl() {
  const p = new URLSearchParams();
  if (shareState.subGroup) p.set('sub', shareState.subGroup);
  if (shareState.search) p.set('q', shareState.search);
  if (shareState.downloadedOnly) p.set('downloaded', '1');
  if (shareState.order !== 'release_date:-1') p.set('order', shareState.order);
  if (shareState.page > 0) p.set('page', String(shareState.page + 1));
  const qs = p.toString();
  history.replaceState(null, '', location.pathname + (qs ? `?${qs}` : ''));
}

// ========== Rendering ==========
function renderShareHeader() {
  const g = shareState.group;
  document.title = `${g.name} - Shared videos`;
  document.getElementById('shareTitle').textContent = g.name;

  const parts = [];
  if (g.sub_groups.length > 0) parts.push(pluralize(g.sub_groups.length, 'group'));
  parts.push(pluralize(g.channels.length, 'channel'));
  document.getElementById('shareMeta').textContent = parts.join(' · ');

  const tabs = document.getElementById('shareTabs');
  if (g.sub_groups.length === 0) {
    tabs.hidden = true;
    return;
  }
  tabs.hidden = false;
  const tab = (id, label) =>
    `<button type="button" class="share-tab ${shareState.subGroup === id ? 'active' : ''}" onclick="setShareSubGroup('${escHtml(id)}')">${escHtml(label)}</button>`;
  tabs.innerHTML = tab('', 'All') + g.sub_groups.map(sg => tab(sg.id, sg.name)).join('');
}

function shareVideoCardHTML(v, index) {
  const duration = shareDurationText(v);
  const metaParts = [];
  if (v.channel_name) metaParts.push(escHtml(v.channel_name));
  if (v.release_date) metaParts.push(escHtml(formatDate(v.release_date)));
  if (v.has_file && v.filesize > 0) metaParts.push(escHtml(formatBytesSize(v.filesize)));

  return `
    <button type="button" class="share-card ${v.has_file ? '' : 'no-file'}" onclick="openShareVideo(${index})">
      <div class="share-thumb">
        <img src="${escHtml(shareThumbnail(v))}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="this.onerror=null;this.src='/static/images/NoThumbnail_bw.jpg'">
        ${duration ? `<span class="share-duration">${escHtml(duration)}</span>` : ''}
        ${v.has_file ? '<span class="share-play" aria-hidden="true">&#9654;</span>' : '<span class="share-nofile">Not downloaded</span>'}
      </div>
      <div class="share-card-body">
        <div class="share-card-title">${escHtml(v.title || v.id)}</div>
        ${v.availability ? `<div class="share-card-tags">${shareAvailabilityHTML(v.availability)}</div>` : ''}
        <div class="share-card-meta">${metaParts.join(' · ')}</div>
      </div>
    </button>
  `;
}

function renderShareVideos() {
  const grid = document.getElementById('shareVideos');

  if (shareState.videos.length === 0) {
    grid.innerHTML = `<div class="share-empty">${shareState.search || shareState.downloadedOnly ? 'No videos match your filters.' : 'No videos here yet.'}</div>`;
  } else {
    grid.innerHTML = shareState.videos.map(shareVideoCardHTML).join('');
  }

  renderSharePagination();
}

// Same page navigation as the main app's videos tab (pagination.js), shown above and below the videos.
function renderSharePagination() {
  const top = document.getElementById('sharePagination-top');
  const bottom = document.getElementById('sharePagination-bottom');
  if (shareState.total === 0) {
    top.innerHTML = '';
    bottom.innerHTML = '';
    return;
  }

  const totalPages = Math.max(1, Math.ceil(shareState.total / SHARE_PAGE_SIZE));
  const cbsId = _registerPgCbs({
    prev: () => setSharePage(shareState.page - 1),
    next: () => setSharePage(shareState.page + 1),
    page: (p) => { if (p !== shareState.page) setSharePage(p); },
  });
  const html = buildPaginationHTML({
    currentPage: shareState.page,
    totalPages: totalPages,
    totalCount: shareState.total,
    currentItems: shareState.videos.length,
    label: 'videos',
    cbsId: cbsId,
  });
  top.innerHTML = html;
  bottom.innerHTML = html;
}

function showShareError(message) {
  document.getElementById('shareContent').hidden = true;
  document.getElementById('shareTitle').textContent = 'Not available';
  document.getElementById('shareMeta').textContent = '';
  const el = document.getElementById('shareError');
  el.hidden = false;
  el.textContent = message;
  document.title = 'Shared videos';
}

// ========== Loading ==========
async function loadShareVideos() {
  const seq = ++shareLoadSeq;
  const [orderBy, orderDir] = shareState.order.split(':');
  const p = new URLSearchParams({
    limit: String(SHARE_PAGE_SIZE),
    page: String(shareState.page),
    order_by: orderBy,
    order_direction: orderDir,
  });
  if (shareState.subGroup) p.set('sub_group', shareState.subGroup);
  if (shareState.search) p.set('search_query', shareState.search);
  if (shareState.downloadedOnly) p.set('downloaded_only', '1');

  try {
    const data = await shareGet(`${shareApiBase}/videos?${p}`);
    if (seq !== shareLoadSeq) return;  // A newer request is in flight.
    shareState.videos = data.videos || [];
    shareState.total = data.total || 0;

    const totalPages = Math.max(1, Math.ceil(shareState.total / SHARE_PAGE_SIZE));
    if (shareState.page >= totalPages && shareState.page > 0) {
      shareState.page = totalPages - 1;
      writeShareUrl();
      return loadShareVideos();
    }
    renderShareVideos();
  } catch (err) {
    if (seq !== shareLoadSeq) return;
    if (err.status === 404 && !shareState.subGroup) {
      showShareError('This shared group does not exist or is no longer public.');
    } else {
      showToast(`Failed to load videos: ${err.message}`, 'error');
    }
  }
}

function setShareSubGroup(id) {
  shareState.subGroup = id;
  shareState.page = 0;
  writeShareUrl();
  renderShareHeader();
  loadShareVideos();
}

function setSharePage(page) {
  const totalPages = Math.max(1, Math.ceil(shareState.total / SHARE_PAGE_SIZE));
  page = Math.min(Math.max(0, page), totalPages - 1);
  if (page === shareState.page) return;
  shareState.page = page;
  writeShareUrl();
  loadShareVideos();
  // Clicked from the bottom bar: jump back up to the first videos.
  const top = document.getElementById('sharePagination-top');
  if (top && top.getBoundingClientRect().top < 0) {
    top.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }
}

// ========== Video modal ==========
async function openShareVideo(index) {
  const listVideo = shareState.videos[index];
  if (!listVideo) return;

  const modal = document.getElementById('shareVideoModal');
  document.getElementById('shareVideoTitle').textContent = listVideo.title || listVideo.id;
  document.getElementById('shareVideoBody').innerHTML = '<div class="share-empty">Loading&hellip;</div>';
  modal.classList.add('active');
  document.body.classList.add('modal-active');

  let v = listVideo;
  try {
    v = await shareGet(`${shareApiBase}/videos/${encodeURIComponent(listVideo.id)}`);
  } catch (err) {
    showToast(`Failed to load video details: ${err.message}`, 'error');
  }
  if (!modal.classList.contains('active')) return;

  const ytUrl = safeExternalUrl(v.url);
  const uploaderUrl = safeExternalUrl(v.uploader_url);
  const facts = [];
  facts.push(['Availability', shareAvailabilityHTML(v.availability) || '<span class="share-muted">Unknown</span>']);
  if (v.channel_name) facts.push(['Channel', escHtml(v.channel_name)]);
  if (v.uploader) facts.push(['Uploader', uploaderUrl ? `<a href="${escHtml(uploaderUrl)}" target="_blank" rel="noopener noreferrer">${escHtml(v.uploader)}</a>` : escHtml(v.uploader)]);
  if (v.release_date) facts.push(['Released', escHtml(formatDateAndTime(v.release_date))]);
  if (shareDurationText(v)) facts.push(['Duration', escHtml(shareDurationText(v))]);
  if (v.has_file && v.filesize > 0) facts.push(['File size', escHtml(formatBytesSize(v.filesize))]);

  const player = v.has_file
    ? `<video class="share-player" controls autoplay preload="metadata" src="${escHtml(shareFileUrl(v, false))}" poster="${escHtml(shareThumbnail(v))}"></video>`
    : `<div class="share-player share-player-empty"><img src="${escHtml(shareThumbnail(v))}" alt="" referrerpolicy="no-referrer" onerror="this.remove()"><span>This video hasn't been downloaded.</span></div>`;

  document.getElementById('shareVideoBody').innerHTML = `
    ${player}
    <div class="share-video-actions">
      ${v.has_file ? `<a class="btn btn-primary" href="${escHtml(shareFileUrl(v, true))}" download>Download${v.filesize > 0 ? ` (${escHtml(formatBytesSize(v.filesize))})` : ''}</a>` : ''}
      ${ytUrl ? `<a class="btn btn-secondary" href="${escHtml(ytUrl)}" target="_blank" rel="noopener noreferrer">Open original</a>` : ''}
    </div>
    <dl class="share-facts">${facts.map(([k, val]) => `<dt>${k}</dt><dd>${val}</dd>`).join('')}</dl>
    ${v.description ? `<div class="share-description">${escHtml(v.description)}</div>` : ''}
  `;
}

function closeShareVideo() {
  const modal = document.getElementById('shareVideoModal');
  if (!modal.classList.contains('active')) return;
  const video = modal.querySelector('video');
  if (video) {
    video.pause();
    video.removeAttribute('src');
    video.load();  // Stop downloading the file.
  }
  modal.classList.remove('active');
  document.body.classList.remove('modal-active');
  document.getElementById('shareVideoBody').innerHTML = '';
}

// ========== Init ==========
async function initShare() {
  if (!shareToken) {
    showShareError('This share link is invalid.');
    return;
  }
  readShareUrl();
  document.getElementById('shareSearch').value = shareState.search;
  document.getElementById('shareOrder').value = shareState.order;
  document.getElementById('shareDownloadedOnly').checked = shareState.downloadedOnly;

  try {
    shareState.group = await shareGet(shareApiBase);
  } catch (err) {
    showShareError(err.status === 404 ? 'This shared group does not exist or is no longer public.' : `Failed to load: ${err.message}`);
    return;
  }
  if (shareState.subGroup && !shareState.group.sub_groups.some(sg => sg.id === shareState.subGroup)) {
    shareState.subGroup = '';
  }
  document.getElementById('shareContent').hidden = false;
  renderShareHeader();
  loadShareVideos();

  let searchTimer = null;
  document.getElementById('shareSearch').addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      shareState.search = e.target.value.trim();
      shareState.page = 0;
      writeShareUrl();
      loadShareVideos();
    }, 300);
  });
  document.getElementById('shareOrder').addEventListener('change', (e) => {
    shareState.order = e.target.value;
    shareState.page = 0;
    writeShareUrl();
    loadShareVideos();
  });
  document.getElementById('shareDownloadedOnly').addEventListener('change', (e) => {
    shareState.downloadedOnly = e.target.checked;
    shareState.page = 0;
    writeShareUrl();
    loadShareVideos();
  });
}

document.getElementById('shareVideoModal').addEventListener('click', (e) => {
  if (e.target.id === 'shareVideoModal') closeShareVideo();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') closeShareVideo();
});

initShare();
