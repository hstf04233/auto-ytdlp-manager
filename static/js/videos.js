// videos.js - videos list, filters + URL state, video details modal.
let firstTimeDeleteVideo = true;
async function deleteVideo(id) {
  if (firstTimeDeleteVideo) {
    if (!confirm("Delete this video? (This prompt will only appear once. If you accept, this prompt will NOT pop up again.)")) {
      return;
    }
    firstTimeDeleteVideo = false;
  }
  
  try {
    await API.del(`/api/videos/${encodeURIComponent(id)}`);
    showToast("Video was deleted!", 'success');
    loadVideos();
  } catch (err) {
    showToast(`Failed to delete: ${err.message}`, 'error');
  }
}
async function refreshVideoInfo(id) {
  try {
    await API.patch(`/api/videos/${encodeURIComponent(id)}`, {refresh_state: true});
    loadVideos()
  } catch (err) {
    showToast(`Failed to refresh: ${err.message}`, 'error');
  }
}

let currentVideoDetails = null;
let currentHls = null;

function videoPreviewClick(videoId) {
  let modalVideoPreview = document.getElementById('modal-video-preview');
  if (!modalVideoPreview) return;
  
  if (!currentVideoDetails) return;
  if (currentVideoDetails.id !== videoId) return;
  
  if (currentHls) {
    currentHls.detachMedia();
    currentHls.destroy();
    currentHls = null;
  }
  
  if (!currentVideoDetails.video_stream_url) {
    modalVideoPreview.innerHTML = `
    <video controls autoplay>
      <source src="/video-file/${escHtml(videoId)}" type="video/mp4">
      Your browser does not support the video tag.
    </video>
    `
  } else {
    modalVideoPreview.innerHTML = '';
    const video_stream_url = currentVideoDetails.video_stream_url;
    let videoEl = document.createElement("video");
    videoEl.controls = true;
    videoEl = modalVideoPreview.appendChild(videoEl);
    
    if (Hls.isSupported()) {
      const hls = new Hls({
        liveSyncDurationCount: 3,
        liveMaxLatencyDurationCount: 9999
      });
      currentHls = hls;
      
      hls.loadSource(video_stream_url);
      hls.attachMedia(videoEl);
      
      hls.on(Hls.Events.MANIFEST_PARSED, () => {
          videoEl.play();
      });
      
      hls.on(Hls.Events.ERROR, (event, data) => {
          console.error('HLS error:', data);
      });
    } else if (videoEl.canPlayType('application/vnd.apple.mpegurl')) {
      // Safari (native HLS support)
      videoEl.src = video_stream_url;
      
      videoEl.addEventListener('loadedmetadata', () => {
          videoEl.play();
      });
    }
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

const VIDEO_THUMB_FALLBACK = '/static/images/NoThumbnail_bw.jpg';

// One-shot fallback that survives soft updates (see renderUpdateVideo).
// Records the failure on dataset so the 10s soft poll doesn't clobber
// the fallback with the same broken URL again (onerror was nulled before,
// so the second failure rendered as a blank broken-image icon).
function handleVideoThumbError(img) {
  if (!img) return;
  if (img.getAttribute('src') === VIDEO_THUMB_FALLBACK || img.dataset.thumbFailed === '1') return;
  img.dataset.thumbFailed = '1';
  img.setAttribute('src', VIDEO_THUMB_FALLBACK);
}

function setVideoThumbImg(img, thumbnailUrl) {
  if (!img) return;
  // URL unchanged: keep current src as-is so a fallback isn't reset to
  // the same broken URL (which would blank out since onerror already fired).
  if (img.dataset.thumb === thumbnailUrl) return;
  img.dataset.thumb = thumbnailUrl;
  delete img.dataset.thumbFailed;
  img.setAttribute('src', thumbnailUrl);
}

async function copySharedVideoFile(videoId) {
  try {
    const sharedLink = await API.getRaw(`/api/share-video-file/${videoId}`);
    
    const fullLocation = window.location.origin + sharedLink;
    
    navigator.clipboard.writeText(fullLocation);
    if (isLocalhost) {
      showToast(`Copied to clipboard while in localhost... Please edit the shared link to your public website.`, "error");
    } else {
      showToast(`Copied to clipboard: ${sharedLink}`, "success");
    }
  } catch (err) {
    showToast(`Failed to copy shared video file, error: ${err.message}`, "error");
  }
}

async function openVideoDetailsModal(videoId) {
  document.getElementById('videoDetailsContent').innerHTML = `Loading video...`
  
  document.getElementById('videoDetailsModal').classList.add('active');
  document.body.classList.add('modal-active');
  
  //let videoInfo = currentVideos.find(x => x.id === videoId);
  let videoInfo = null;
  if (!videoInfo) {
    try {
      const returnedVideoInfo = await API.get(`/api/videos/${encodeURIComponent(videoId)}`);
      
      if (returnedVideoInfo) {
        videoInfo = returnedVideoInfo;
      }
    } catch (err) {
      showToast(`Failed to get video, error: ${err.message}`, "error");
    }
  };
  if (!videoInfo) {
    // showToast(`Failed to copy shared video file, error: ${err.message}`, "error");

    // Reuses the close path so a failed open also restores the previous URL.
    closeVideoDetailsModal();
    return
  }
  
  currentVideoDetails = videoInfo;

  const channel = getChannelFromId(videoInfo.from_channel);
  const channelName = channel ? escHtml(channel.name) : 'Unknown Channel';
  const channelUrl = channel ? escHtml(channel.url) : '';

  const resolutionParts = (videoInfo.resolution || '').split('x');
  const resolutionText = resolutionParts.length === 2
    ? `${resolutionParts[0].trim()} x ${resolutionParts[1].trim()}`
    : (videoInfo.resolution || '\u2014');

  const descHtml = videoInfo.description
    ? escHtml(videoInfo.description).replace(/\n/g, '<br>')
    : '\u2014';
  
  const videoIsPlayable = (videoInfo.videofile_exists || (videoInfo.video_stream_url && videoInfo.video_stream_url !== ""))
  
  let videoPreviewOnClick = "";
  if (videoIsPlayable) {
    videoPreviewOnClick = `event.preventDefault(); videoPreviewClick('${videoId}');`;
  }
  
  var thumbnailUrl = getThumbnail(videoInfo);
  
  document.getElementById('videoDetailsContent').innerHTML = `
    <div class="vd-preview" id="modal-video-preview">
      <img src="${escHtml(thumbnailUrl)}" alt="" onclick="${videoPreviewOnClick}" onerror="handleVideoThumbError(this)"
        ${videoIsPlayable ? ` style="cursor: pointer;" title="Click to play video"` : ''}>
        ${videoIsPlayable ? `<span>&#9654;</span>` : ''}
      </img>
      <div class="vd-preview-placeholder" style="display:none">No thumbnail</div>
    </div>
    <h3 class="vd-title">${escHtml(videoInfo.title)}</h3>
    ${videoInfo.uploader ? `<p class="vd-header">Uploader: <a href="${escHtml(videoInfo.uploader_url)}" target="_blank">${escHtml(videoInfo.uploader)}</a></p>` : ''}
    <p class="vd-header">${escHtml(videoInfo.availability)}</p>
    <hr>
    <div class="vd-grid">
      <div class="vd-field"><span class="vd-field-label">Channel</span><span class="vd-field-value">${escHtml(channelName)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video ID</span><span class="vd-field-value" style="font-family:monospace;font-size:0.8rem">${escHtml(videoInfo.id)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video URL</span><a href="${escHtml(videoInfo.url)}" target="_blank" class="vd-field-value" style="font-family:monospace;font-size:0.8rem">${escHtml(videoInfo.url)}</a></div>
      <div class="vd-field"><span class="vd-field-label">Duration</span><span class="vd-field-value">${formatDuration(videoInfo.duration)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Video Type</span><span class="vd-field-value">${videoInfo.video_type !== undefined ? videoTypeBadge(videoInfo.video_type) : '\u2014'}</span></div>
      <div class="vd-field"><span class="vd-field-label">Status</span><span class="vd-field-value">${videoStatusBadge(videoInfo.id, videoInfo.status)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Filename ${(videoInfo.videofile_exists || !videoInfo.filename || videoInfo.status == 1) ? '' : '(Moved or deleted)'}</span><span class="vd-field-value" style="font-family:monospace;font-size:0.78rem;word-break:break-all">${escHtml(videoInfo.filename || '\u2014')}</span></div>
      <div class="vd-field"><span class="vd-field-label">Filesize</span><span class="vd-field-value">${formatBytesSize(videoInfo.filesize || 0)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Resolution</span><span class="vd-field-value">${resolutionText}</span></div>
      <div class="vd-field"><span class="vd-field-label">Release Date</span><span class="vd-field-value">${formatDateAndTime(videoInfo.release_date)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Added</span><span class="vd-field-value">${formatDateAndTime(videoInfo.added_at)}</span></div>
      <div class="vd-field"><span class="vd-field-label">Updated</span><span class="vd-field-value">${formatDateAndTime(videoInfo.updated_at)}</span></div>
    </div>

    
    <div class="vd-description">
      <span class="vd-field-label">Description</span>
      <div class="vd-description-text">${descHtml}</div>
    </div>
  `;

  const refreshDisabled = videoInfo.refresh_state ? 'disabled style="opacity:0.5;cursor:not-allowed"' : '';
  const refreshTitle = videoInfo.refresh_state ? 'Refreshing...' : 'Refresh metadata';

  document.getElementById('videoDetailsActions').innerHTML = `
    ${
      videoInfo.videofile_exists ?
    `<a href="/video-file/${escHtml(videoInfo.id)}?download=true" target="_blank" class="btn btn-secondary btn-sm" title="Download video file">Download Video File</a>` :
    ''
    }
    ${
      videoInfo.videofile_exists ?
    `<button type="button" class="btn btn-secondary btn-sm" class="btn btn-secondary btn-sm" onclick="copySharedVideoFile('${videoInfo.id}')" title="Copy shared video file link to clipboard">Share video file link</a>` :
    ''
    }
    <button type="button" class="btn btn-secondary btn-sm" ${refreshDisabled} onclick="refreshVideoInfo('${videoInfo.id}');closeVideoDetailsModal();" title="${refreshTitle}">${videoInfo.refresh_state ? 'Refreshing...' : 'Refresh'}</button>
    <button type="button" class="btn btn-secondary btn-sm" onclick="openVideoHistoryModal('${videoInfo.id}')" title="View change history for this video">History</button>
    <button type="button" class="btn btn-danger btn-sm" onclick="deleteVideo('${videoInfo.id}');closeVideoDetailsModal();">Delete</button>
  `;
}

let videoModalPushed = false;
let videoModalReturnUrl = '/videos';

// Opened from the videos list (Details button): push /video/{id} so the URL
// reflects the modal, and closing restores the exact list behind it.
function openVideoDetailsFromList(videoId) {
  // Remember the list URL in case there is no usable history to go back to
  // (e.g. the modal URL gets reloaded or shared).
  try {
    videoModalReturnUrl = buildVideosUrl();
  } catch (e) {
    videoModalReturnUrl = '/videos';
  }
  history.pushState({}, '', `/video/${encodeURIComponent(videoId)}`);
  videoModalPushed = true;
  lastPageOpen = `video/${videoId}`;
  return openVideoDetailsModal(videoId);
}

function closeVideoDetailsModal(skipHistory) {
  if (currentHls) {
    currentHls.detachMedia();
    currentHls.destroy();
    currentHls = null;
  }

  let modalVideoPreview = document.getElementById('modal-video-preview');
  if (modalVideoPreview) {
    // Remove any video playing

    const videoElement = modalVideoPreview.querySelector("video");
    if (videoElement) {
      videoElement.pause();

      videoElement.removeAttribute('src');
      videoElement.src = '';
      while (videoElement.firstChild) {
        videoElement.removeChild(videoElement.firstChild);
      }
    }

    modalVideoPreview.innerHTML = "";
  }

  document.getElementById('videoDetailsModal').classList.remove('active');
  document.body.classList.remove('modal-active');

  const wasPushed = videoModalPushed;
  videoModalPushed = false;
  // skipHistory is for popstate: the history navigation already happened,
  // the modal just needs tearing down.
  if (skipHistory) return;
  if (wasPushed) {
    // Back to the exact list (filters + page) we came from; the popstate
    // handler re-renders it.
    history.back();
    return;
  }
  if (window.location.pathname.startsWith('/video/')) {
    // Opened without a usable history entry (direct load / forward-nav):
    // fall back to the videos list instead of leaving the app.
    const returnUrl = videoModalReturnUrl || '/videos';
    window.history.replaceState(null, '', returnUrl);
    showPage(returnUrl.replace(/^\//, ''), true);
  }
}

// ========== Video history ==========
// A history point stores the OLD values (the state before the change), and
// only for fields that changed at that step:
//   Title/Description/Availability/OriginThumbnail/StoredThumbnail: a string
//     means "changed, this was the old value"; null/missing = unchanged.
//   Duration/VideoType: -1 = unchanged, otherwise the old value.
//   AddedAt = when this old state became current; UpdatedAt = when the
//     change was detected (i.e. this state's validity window).
// Url is always recorded but never diffed, and uploader/release_date are
// never recorded at all, so those always resolve to the current video.
//
// Filling a blank field is therefore ALWAYS a forward search: if point i
// leaves a field unset, the field didn't change at step i, so its value at
// point i equals the value right after step i -- the next set value at some
// point j > i, or the current video's value when no later point sets it.
// (Searching backwards would be WRONG: e.g. title A ->(pt0) B ->(pt1,
// availability-only change) B ->(pt2) C stores titles [A, <blank>, B]; a
// backwards lookup for point 1 finds "A", but the title during state 1 was
// "B". The forward search finds it correctly.)
function histGet(point, kind) {
  switch (kind) {
    case 'title':
      return (typeof point.title === 'string') ? point.title : undefined;
    case 'description':
      return (point.description !== undefined && point.description !== null) ? point.description : undefined;
    case 'availability':
      return (typeof point.availability === 'string') ? point.availability : undefined;
    case 'duration':
      return (typeof point.duration === 'number' && point.duration >= 0) ? point.duration : undefined;
    case 'video_type':
      return (typeof point.video_type === 'number' && point.video_type >= 0) ? point.video_type : undefined;
    case 'thumbnail':
      return (typeof point.origin_thumbnail_url === 'string' || typeof point.stored_thumbnail === 'string')
        ? { origin: point.origin_thumbnail_url || '', stored: point.stored_thumbnail || '' }
        : undefined;
    default:
      return undefined;
  }
}

// Value of a field at point `index` (the state before change `index`).
function histResolve(points, index, kind, currentFallback) {
  for (let j = index; j < points.length; j++) {
    const val = histGet(points[j], kind);
    if (val !== undefined) return val;
  }
  return currentFallback;
}

function histCurrentValue(current, kind) {
  if (!current) return undefined;
  switch (kind) {
    case 'title': return current.title;
    case 'description': return current.description;
    case 'availability': return current.availability;
    case 'duration': return current.duration;
    case 'video_type': return current.video_type;
    case 'thumbnail': return { origin: current.origin_thumbnail_url || '', stored: current.stored_thumbnail || '' };
    default: return undefined;
  }
}

function histText(s, maxLen) {
  if (s === null || s === undefined || s === '') return '<span class="vh-empty">\u2014</span>';
  s = String(s);
  const short = s.length > maxLen ? s.slice(0, maxLen) + '\u2026' : s;
  return `<span title="${escHtml(s)}">${escHtml(short)}</span>`;
}

// Thumbnail URL priority mirrors getThumbnail: stored image first, then the
// origin URL, then YouTube's default for the video id.
function histThumbUrl(thumb, videoId) {
  if (thumb && thumb.stored) return `/db-image/${thumb.stored}`;
  if (thumb && thumb.origin) return thumb.origin;
  if (videoId) return `https://img.youtube.com/vi/${videoId}/mqdefault.jpg`;
  return VIDEO_THUMB_FALLBACK;
}

function histDisplayValue(kind, val, videoId) {
  if (val === undefined || val === null) return '<span class="vh-empty">\u2014</span>';
  switch (kind) {
    case 'duration':
      return escHtml(formatDuration(val));
    case 'video_type':
      return videoTypeBadge(val);
    case 'description':
      return `<span class="vh-desc">${histText(val, 300)}</span>`;
    case 'thumbnail': {
      const url = histThumbUrl(val, videoId);
      return `<img class="vh-thumb-img" src="${escHtml(url)}" alt="Thumbnail" title="${escHtml(url)}" loading="lazy" onerror="this.onerror=null;this.src='${VIDEO_THUMB_FALLBACK}'">`;
    }
    default:
      return histText(val, 160);
  }
}

const HIST_FIELDS = [
  { kind: 'title', label: 'Title' },
  { kind: 'description', label: 'Description' },
  { kind: 'availability', label: 'Availability' },
  { kind: 'duration', label: 'Duration' },
  { kind: 'video_type', label: 'Video Type' },
  { kind: 'thumbnail', label: 'Thumbnail' },
];

function videoHistoryHTML(points, current) {
  if (!points || points.length === 0) {
    return '<div class="loading">No history recorded for this video yet.</div>';
  }

  let html = '<div class="vh-timeline">';

  // Live values first.
  html += '<div class="vh-entry vh-current"><div class="vh-head"><span class="vh-rev">Current state</span><span class="vh-time">live</span></div>';
  const currentId = current ? current.id : undefined;
  for (const f of HIST_FIELDS) {
    html += `<div class="vh-change vh-single"><span class="vh-label">${f.label}</span><span class="vh-new">${histDisplayValue(f.kind, histCurrentValue(current, f.kind), currentId)}</span></div>`;
  }
  html += '</div>';

  // Changes, newest first. Point i holds the OLD values; the NEW value of a
  // changed field is whatever comes right after step i.
  for (let i = points.length - 1; i >= 0; i--) {
    const p = points[i];
    const changed = HIST_FIELDS.filter(f => histGet(p, f.kind) !== undefined);
    html += `<div class="vh-entry"><div class="vh-head"><span class="vh-rev">Revision ${p.revision_number}</span><span class="vh-time" title="${escHtml(formatDateAndTime(p.updated_at))}">${escHtml(formatRelative(p.updated_at))}</span></div>`;
    if (p.added_at && p.updated_at) {
      html += `<div class="vh-valid">This state was valid from ${escHtml(formatDateAndTime(p.added_at))} until ${escHtml(formatDateAndTime(p.updated_at))}.</div>`;
    }
    if (changed.length === 0) {
      html += '<div class="vh-valid">No field changes recorded.</div>';
    }
    for (const f of changed) {
      const oldVal = histGet(p, f.kind);
      const newVal = histResolve(points, i + 1, f.kind, histCurrentValue(current, f.kind));
      const vid = p.id || currentId;
      html += `<div class="vh-change"><span class="vh-label">${f.label}</span><span class="vh-old">${histDisplayValue(f.kind, oldVal, vid)}</span><span class="vh-arrow">&rarr;</span><span class="vh-new">${histDisplayValue(f.kind, newVal, vid)}</span></div>`;
    }
    html += '</div>';
  }

  return html + '</div>';
}

async function openVideoHistoryModal(videoId) {
  const modal = document.getElementById('videoHistoryModal');
  const content = document.getElementById('videoHistoryContent');
  const titleEl = document.getElementById('videoHistoryTitle');
  modal.classList.add('active');
  document.body.classList.add('modal-active');
  content.innerHTML = '<div class="loading">Loading history...</div>';
  if (titleEl) titleEl.textContent = 'Video History';
  try {
    // Reuse the details modal's video when it's the same one to save a request.
    let current = (typeof currentVideoDetails !== 'undefined' && currentVideoDetails && currentVideoDetails.id === videoId)
      ? currentVideoDetails : null;
    const histPromise = API.get(`/api/video-history/${encodeURIComponent(videoId)}`);
    const videoPromise = current ? Promise.resolve(null) : API.get(`/api/videos/${encodeURIComponent(videoId)}`);
    const [histRes, videoRes] = await Promise.all([histPromise, videoPromise]);
    if (videoRes) current = videoRes;
    const points = (histRes && histRes.points) || [];
    if (titleEl) titleEl.textContent = `History: ${current && current.title ? current.title : videoId}`;
    content.innerHTML = videoHistoryHTML(points, current);
  } catch (err) {
    content.innerHTML = `<div class="loading">Failed to load history: ${escHtml(err.message)}</div>`;
  }
}

function closeVideoHistoryModal() {
  document.getElementById('videoHistoryModal').classList.remove('active');
  // The history modal opens on top of the details modal: only release the
  // body scroll-lock when nothing else is still open.
  if (!document.getElementById('videoDetailsModal').classList.contains('active')) {
    document.body.classList.remove('modal-active');
  }
}

// ========== Videos ==========
let lastVideosSig = null;
function videosSig(videos) {
  return videos.map(v => v.id).join(',');
}
async function loadVideos(softUpdateOnly) {
  areVideosLoading = true;
  try {
    // Retry once if ?page= in the URL was out of range (e.g. shared link).
    for (let attempt = 0; attempt < 2; attempt++) {
      const statusFilter = document.getElementById('videoStatusFilter').value;
      const channelFilter = document.getElementById('videoChannelFilter').value;
      const orderBy = document.getElementById('videoOrderBy').value;
      const orderDir = document.getElementById('videoOrderDirection').value;

      let url = `/api/videos?limit=${VIDEO_PAGE_SIZE}&page=${videoPage}`;
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
      currentVideos = data.videos || data;
      videoStats = data.stats || {};
      videoTotalCount = videoStats.total || currentVideos.length;

      const totalPages = Math.ceil(videoTotalCount / VIDEO_PAGE_SIZE) || 1;
      if (videoPage >= totalPages && videoPage > 0) {
        videoPage = Math.max(0, totalPages - 1);
        syncVideosUrl();
        continue;   // refetch the clamped page
      }
      break;
    }
    const sig = videosSig(currentVideos);
    if (softUpdateOnly && sig === lastVideosSig) {
      // Same videos in the same order: patch the existing cards in place
      // instead of rebuilding the whole list (no flicker, keeps focus/scroll).
      softRenderVideos();
    } else {
      renderVideos();
      lastVideosSig = sig;
    }
    renderVideoPagination();
    updateVideoStats();
  } catch (err) {
    showToast(`Failed to load videos: ${err.message}`, 'error');
  }
  areVideosLoading = false;
}

function filterVideosTabByChannel(channelId) {
  ensureVideoChannelOption(channelId);
  document.getElementById('videoChannelFilter').value = channelId;

  onVideoFilterChange();
}

function videoDurationText(v) {
  if (v.video_type == 2 && (v.duration <= 1)) {  // VIDEO_TYPE_ISLIVE
    return "LIVE";
  }
  return formatDuration(v.duration);
}

function videoAddedLineText(v) {
  return `Added ${formatRelative(v.added_at)} \u00b7 Updated ${formatRelative(v.updated_at)} ${v.filesize ? formatBytesSize(v.filesize) : ''}`;
}

function videoTasksSig(v) {
  return `${v.tasks_count || 0}|${v.active_task || ''}`;
}

function videoTasksInnerHTML(v) {
  if (!(v.tasks_count > 0 || v.active_task)) return '';
  
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
  
  return `<a href="/tasks?video=${v.id}" onclick="${tasksButtonOnClick}">${tasksButtonText}</a>`;
}

function videoUploaderInnerHTML(v) {
  if (!v.uploader) return '';
  return `Uploader: <a href="${escHtml(v.uploader_url)}" target="_blank">${escHtml(v.uploader)}</a>`;
}

function videoFileInnerHTML(v) {
  if (!v.videofile_exists) return '';
  return `<a href="/video-file/${escHtml(v.id)}" target="_blank" class="video-menu-item" data-action="video-file" title="Open video file">Open video file</a>`;
}

function videoMenuButtonHTML(v) {
  return `<button class="btn btn-secondary btn-sm video-menu-btn" onclick="event.stopPropagation();toggleVideoMenu('${v.id}', this)" title="More actions">...</button>`;
}

function hideAllVideoMenus() {
  document.querySelectorAll('.video-menu-dropdown').forEach(el => el.remove());
}

let _videoMenuListenerActive = false;
let _videoMenuOpenFor = null;

function closeVideoMenu() {
  hideAllVideoMenus();
  _videoMenuOpenFor = null;
  // Note: document listeners stay registered (guarded by
  // _videoMenuListenerActive) so reopening doesn't stack duplicates.
}

function toggleVideoMenu(videoId, buttonEl) {
  // Toggling the same video's menu closes it.
  if (_videoMenuOpenFor === videoId) {
    closeVideoMenu();
    return;
  }
  closeVideoMenu();
  if (typeof closeStatusDropdown === 'function') closeStatusDropdown();
  if (typeof closeBulkStatusDropdown === 'function') closeBulkStatusDropdown();

  const v = (typeof currentVideos !== 'undefined' && currentVideos) ? currentVideos.find(x => x.id === videoId) : null;
  const videofileExists = v ? !!v.videofile_exists : false;
  const refreshing = v ? !!v.refresh_state : false;
  const canDownload = v ? (v.status != 2 && v.status != 1) : false;

  const dropdown = document.createElement('div');
  dropdown.className = 'video-menu-dropdown';
  dropdown.dataset.videoId = videoId;

  const refreshTitle = refreshing ? 'Refreshing...' : 'Refresh metadata';
  const refreshLabel = refreshing ? 'Refreshing...' : 'Refresh';
  dropdown.innerHTML = `
    ${videofileExists ? videoFileInnerHTML(v) : ''}
    ${canDownload ? `<div class="video-menu-item" data-action="download" title="Download this video">Download this video</div>` : ''}
    <div class="video-menu-item" data-action="history" title="View change history for this video">View history</div>
    <div class="video-menu-item ${refreshing ? 'is-disabled' : ''}" data-action="refresh" title="${refreshTitle}">${refreshLabel}</div>
    <div class="video-menu-item video-menu-item-danger" data-action="delete" title="Deleting a video does not remove the video file.">Delete</div>
  `;

  dropdown.addEventListener('click', (e) => {
    e.stopPropagation();
    const item = e.target.closest('.video-menu-item');
    if (!item) return;
    const action = item.dataset.action;
    if (action === 'video-file') {
      // Let the <a target="_blank"> navigate, just close the menu.
      closeVideoMenu();
      return;
    }
    if (action === 'download') {
      closeVideoMenu();
      if (typeof changeVideoStatus === 'function') changeVideoStatus(videoId, -100);
      return;
    }
    if (action === 'history') {
      closeVideoMenu();
      openVideoHistoryModal(videoId);
      return;
    }
    if (action === 'refresh') {
      if (refreshing) return;
      closeVideoMenu();
      refreshVideoInfo(videoId);
      return;
    }
    if (action === 'delete') {
      closeVideoMenu();
      deleteVideo(videoId);
      return;
    }
  });

  document.body.appendChild(dropdown);
  const rect = buttonEl.getBoundingClientRect();
  const menuWidth = 180;
  dropdown.style.top = (rect.bottom + 4) + 'px';
  dropdown.style.left = Math.max(8, rect.right - menuWidth) + 'px';
  // Keep the menu on-screen: flip above the button when it would
  // overflow the bottom edge.
  const menuRect = dropdown.getBoundingClientRect();
  if (menuRect.bottom > window.innerHeight - 8) {
    dropdown.style.top = Math.max(8, rect.top - menuRect.height - 4) + 'px';
  }
  if (menuRect.right > window.innerWidth - 8) {
    dropdown.style.left = Math.max(8, window.innerWidth - menuRect.width - 8) + 'px';
  }

  _videoMenuOpenFor = videoId;
  if (!_videoMenuListenerActive) {
    document.addEventListener('click', closeVideoMenu);
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeVideoMenu(); });
    _videoMenuListenerActive = true;
  }
}

// ========== Videos bulk selection ==========
// Persists across page/filter changes (ids only). Refresh + status use
// PATCH /api/bulk-update-videos and delete uses DELETE /api/bulk-delete-videos,
// each in one request; download has no bulk endpoint yet so it still runs
// single-video requests sequentially.
let selectedVideoIds = new Set();
let bulkActionRunning = false;

function syncVideoCardSelected(videoId, selected) {
  const card = document.getElementById(`video-${videoId}`);
  if (!card) return;
  card.classList.toggle('is-selected', selected);
  const cb = card.querySelector('.video-select-checkbox');
  if (cb && cb.checked !== selected) cb.checked = selected;
}

function toggleVideoSelection(videoId, checked) {
  if (checked) {
    selectedVideoIds.add(videoId);
  } else {
    selectedVideoIds.delete(videoId);
  }
  syncVideoCardSelected(videoId, checked);
  updateBulkBar();
}

// While something is selected, clicking a thumbnail toggles that video
// (bigger target than the checkbox). With nothing selected the thumbnail
// does nothing, as before. The checkbox label stops propagation so it
// never double-toggles.
function videoThumbClick(videoId, event) {
  if (selectedVideoIds.size === 0) return;
  if (event) event.stopPropagation();
  toggleVideoSelection(videoId, !selectedVideoIds.has(videoId));
}

function clearVideoSelection() {
  const ids = [...selectedVideoIds];
  selectedVideoIds.clear();
  // Update visible cards in place (no full rebuild).
  for (const id of ids) syncVideoCardSelected(id, false);
  updateBulkBar();
}

function toggleSelectVideoPage() {
  const pageIds = currentVideos.map(v => v.id);
  const allSelected = pageIds.length > 0 && pageIds.every(id => selectedVideoIds.has(id));
  for (const id of pageIds) {
    if (allSelected) {
      selectedVideoIds.delete(id);
    } else {
      selectedVideoIds.add(id);
    }
    syncVideoCardSelected(id, !allSelected);
  }
  updateBulkBar();
}

function updateBulkBar() {
  const bar = document.getElementById('videoBulkBar');
  if (!bar) return;
  const count = selectedVideoIds.size;
  bar.hidden = count === 0;
  const list = document.getElementById('videosList');
  if (list) list.classList.toggle('has-selection', count > 0);
  const countEl = document.getElementById('videoBulkCount');
  if (countEl) countEl.textContent = count === 1 ? '1 selected' : `${count} selected`;
  const pageIds = currentVideos.map(v => v.id);
  const allPageSelected = pageIds.length > 0 && pageIds.every(id => selectedVideoIds.has(id));
  const pageBtn = document.getElementById('videoBulkPageBtn');
  if (pageBtn) {
    pageBtn.textContent = allPageSelected ? 'Deselect page' : `Select page (${pageIds.length})`;
    pageBtn.disabled = bulkActionRunning || pageIds.length === 0;
  }
  const btns = ['videoBulkClearBtn', 'videoBulkRefreshBtn', 'videoBulkDownloadBtn', 'videoBulkStatusBtn', 'videoBulkDeleteBtn'];
  for (const id of btns) {
    const btn = document.getElementById(id);
    if (btn) btn.disabled = bulkActionRunning;
  }
}

let _bulkStatusOpen = false;
let _bulkStatusListenerActive = false;

function closeBulkStatusDropdown() {
  document.querySelectorAll('.bulk-status-dropdown').forEach(el => el.remove());
  _bulkStatusOpen = false;
  // Note: document listeners stay registered (guarded by
  // _bulkStatusListenerActive) so reopening doesn't stack duplicates.
}

function toggleBulkStatusDropdown(buttonEl) {
  if (_bulkStatusOpen) {
    closeBulkStatusDropdown();
    return;
  }
  closeBulkStatusDropdown();
  if (typeof closeVideoMenu === 'function') closeVideoMenu();
  if (typeof closeStatusDropdown === 'function') closeStatusDropdown();

  const dropdown = document.createElement('div');
  dropdown.className = 'video-menu-dropdown bulk-status-dropdown';
  dropdown.innerHTML = `
    <div class="video-menu-item" data-status="0" title="Set selected videos to Queued">Queued</div>
    <div class="video-menu-item" data-status="4" title="Set selected videos to Ignored">Ignored</div>
  `;

  dropdown.addEventListener('click', (e) => {
    e.stopPropagation();
    const item = e.target.closest('.video-menu-item');
    if (!item) return;
    closeBulkStatusDropdown();
    bulkSetStatusSelected(item.dataset.status);
  });

  document.body.appendChild(dropdown);
  const rect = buttonEl.getBoundingClientRect();
  dropdown.style.top = (rect.bottom + 4) + 'px';
  dropdown.style.left = rect.left + 'px';
  // Keep the menu on-screen: flip above the button when it would
  // overflow the bottom edge.
  const menuRect = dropdown.getBoundingClientRect();
  if (menuRect.bottom > window.innerHeight - 8) {
    dropdown.style.top = Math.max(8, rect.top - menuRect.height - 4) + 'px';
  }
  if (menuRect.right > window.innerWidth - 8) {
    dropdown.style.left = Math.max(8, window.innerWidth - menuRect.width - 8) + 'px';
  }

  _bulkStatusOpen = true;
  if (!_bulkStatusListenerActive) {
    document.addEventListener('click', closeBulkStatusDropdown);
    document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeBulkStatusDropdown(); });
    _bulkStatusListenerActive = true;
  }
}

async function bulkRefreshSelected() {
  const ids = [...selectedVideoIds];
  if (ids.length === 0 || bulkActionRunning) return;
  bulkActionRunning = true;
  updateBulkBar();
  try {
    const res = await API.patch('/api/bulk-update-videos',
      ids.map(id => ({ video_id: id, content: { refresh_state: true } })));
    const ok = res.successes || 0, total = res.total || ids.length;
    showToast(total - ok ? `Bulk refresh: ${ok} of ${total} updated` : `Bulk refresh started for ${ok} video(s)`, total - ok ? 'error' : 'success');
  } catch (err) {
    showToast(`Bulk refresh failed: ${err.message}`, 'error');
  }
  bulkActionRunning = false;
  loadVideos();
}

async function bulkDeleteSelected() {
  const ids = [...selectedVideoIds];
  if (ids.length === 0 || bulkActionRunning) return;
  if (!confirm(`Delete ${ids.length} selected video(s)? (This does not remove video files.)`)) return;
  firstTimeDeleteVideo = false;
  bulkActionRunning = true;
  updateBulkBar();
  try {
    const res = await API.del('/api/bulk-delete-videos', ids);
    const ok = res.successes || 0, total = res.total || ids.length;
    // Deselect exactly the videos the server deleted; anything that
    // failed stays selected for an easy retry.
    const deleted = Array.isArray(res.deleted_video_ids) ? res.deleted_video_ids : ids;
    for (const id of deleted) selectedVideoIds.delete(id);
    showToast(total - ok ? `Bulk delete: ${ok} of ${total} deleted` : `Deleted ${ok} video(s)`, total - ok ? 'error' : 'success');
  } catch (err) {
    showToast(`Bulk delete failed: ${err.message}`, 'error');
  }
  bulkActionRunning = false;
  loadVideos();
}

async function bulkSetStatusSelected(status) {
  const ids = [...selectedVideoIds];
  if (ids.length === 0 || bulkActionRunning) return;
  bulkActionRunning = true;
  updateBulkBar();
  try {
    const res = await API.patch('/api/bulk-update-videos',
      ids.map(id => ({ video_id: id, content: { status: parseInt(status) } })));
    const ok = res.successes || 0, total = res.total || ids.length;
    showToast(total - ok ? `Bulk status: ${ok} of ${total} updated` : `Status changed for ${ok} video(s)`, total - ok ? 'error' : 'success');
  } catch (err) {
    showToast(`Bulk status change failed: ${err.message}`, 'error');
  }
  bulkActionRunning = false;
  loadVideos();
}

async function bulkDownloadSelected() {
  const ids = [...selectedVideoIds];
  if (ids.length === 0 || bulkActionRunning) return;
  bulkActionRunning = true;
  updateBulkBar();
  let ok = 0, failed = 0, skipped = 0;
  for (const id of ids) {
    try {
      // Selection can span pages, so fall back to fetching videos that
      // aren't in the current page.
      let videoData = currentVideos.find(x => x.id === id);
      if (!videoData) videoData = await API.get(`/api/videos/${encodeURIComponent(id)}`);
      if (!videoData || videoData.status == 2 || videoData.status == 1) {
        skipped++;
        continue;
      }
      let body = {
        download_url: videoData.url,
        type: -2,
        target_channel_id: videoData.from_channel,
      };
      const qualitySelect = getQualityFromResolution(videoData.resolution);
      if (qualitySelect > 0) body.quality_select = qualitySelect;
      await API.post(`/api/add-videos?no_wait=true&queue_video_id=${encodeURIComponent(id)}`, body);
      ok++;
    } catch (err) {
      failed++;
    }
  }
  bulkActionRunning = false;
  const parts = [`${ok} queued`];
  if (skipped) parts.push(`${skipped} skipped (already downloading/downloaded)`);
  if (failed) parts.push(`${failed} failed`);
  showToast(`Bulk download: ${parts.join(', ')}`, failed ? 'error' : 'success');
  loadVideos();
}

function videoCardHTML(v) {
  var thumbnailUrl = getThumbnail(v);
  var durationText = videoDurationText(v);

  const channel = getChannelFromId(v.from_channel);
  
  const isSelected = selectedVideoIds.has(v.id);
  return `
    <div class="card video-card${isSelected ? ' is-selected' : ''}" id="video-${v.id}">
      <div class="video-thumb" onclick="videoThumbClick('${v.id}', event)">
        <img class="video-thumb-img" src="${thumbnailUrl}" data-thumb="${escHtml(thumbnailUrl)}" alt="" onerror="handleVideoThumbError(this)">
        <label class="video-select" title="Select this video" onclick="event.stopPropagation()"><input type="checkbox" class="video-select-checkbox" data-video-id="${v.id}" onchange="toggleVideoSelection('${v.id}', this.checked)" ${isSelected ? 'checked' : ''}><span class="video-select-box" aria-hidden="true"></span></label>
        ${v.refresh_state ? `<span class="video-refresh-spinner" title="Metadata is being refreshed..." aria-label="Metadata is being refreshed..."></span>` : ''}
        <span class="video-duration" title="Video duration">${durationText}</span>
      </div>
      <div class="video-info">
        <h3 class="video-title">
          <span class="video-title-text" title="${escHtml(v.title)}">${escHtml(v.title)}</span>
          <a class="video-link" href="${escHtml(v.url)}" target="_blank">[VideoLink]</a>
        </h3>
        <p class="video-channel-line">From: <a class="video-channel-link" title="Filter by this channel" href="/videos?channel=${v.from_channel}" onclick="event.preventDefault();filterVideosTabByChannel('${v.from_channel}');">${channel ? escHtml(channel.name) : 'Unknown Channel'}</a>
        <span class="video-uploader-part" data-sig="${encodeURIComponent(v.uploader || '')}|${encodeURIComponent(v.uploader_url || '')}">${videoUploaderInnerHTML(v)}</span>
        </p>
        <p class="video-release-line">Released: ${formatDateAndTime(v.release_date)}</p>
        <p class="video-availability-line">${escHtml(v.availability)}</p>
        <p class="video-added-line">${videoAddedLineText(v)}</p>
        
        <p class="video-tasks-line" data-sig="${videoTasksSig(v)}">${videoTasksInnerHTML(v)}</p>
      </div>
      <div class="video-actions">
        <span class="video-status-wrap" data-status="${v.status}">${videoStatusBadge(v.id, v.status)}</span>
        <span class="video-type-wrap" data-type="${v.video_type !== undefined ? v.video_type : ''}">${v.video_type !== undefined ? videoTypeBadge(v.video_type) : ''}</span>
        <a class="btn btn-secondary btn-sm video-details-btn" href="/video/${v.id}" onclick="event.preventDefault(); openVideoDetailsFromList('${v.id}');">Details</a>
        <span class="video-menu-wrap" data-present="${v.videofile_exists ? '1' : '0'}" data-refreshing="${v.refresh_state ? '1' : '0'}">${videoMenuButtonHTML(v)}</span>
      </div>
    </div>
  `;
}

function renderVideos() {
  const container = document.getElementById('videosList');

  if (currentVideos.length === 0) {
    container.innerHTML = '<div class="loading">No videos found.</div>';
    updateBulkBar();
    return;
  }

  container.innerHTML = currentVideos.map(videoCardHTML).join('');
  updateBulkBar();
}

function renderUpdateVideo(v) {
  const card = document.getElementById(`video-${v.id}`);
  if (!card) return false;
  const qs = (sel) => card.querySelector(sel);
  const setText = (el, text) => { if (el && el.textContent !== text) el.textContent = text; };
  const selectCb = qs('.video-select-checkbox');
  const shouldCheck = selectedVideoIds.has(v.id);
  if (selectCb && selectCb.checked !== shouldCheck) selectCb.checked = shouldCheck;
  card.classList.toggle('is-selected', shouldCheck);
  
  const thumbImg = qs('.video-thumb-img');
  if (thumbImg) {
    setVideoThumbImg(thumbImg, getThumbnail(v));
  }
  const isRefreshing = !!v.refresh_state;
  const spinner = qs('.video-refresh-spinner');
  if (isRefreshing && !spinner) {
    const thumb = qs('.video-thumb');
    if (thumb) {
      const el = document.createElement('span');
      el.className = 'video-refresh-spinner';
      el.setAttribute('title', 'Metadata is being refreshed...');
      el.setAttribute('aria-label', 'Metadata is being refreshed...');
      thumb.appendChild(el);
    }
  } else if (!isRefreshing && spinner) {
    spinner.remove();
  }
  setText(qs('.video-duration'), videoDurationText(v));
  
  const titleEl = qs('.video-title-text');
  if (titleEl) {
    setText(titleEl, v.title || '');
    if (titleEl.getAttribute('title') !== (v.title || '')) titleEl.setAttribute('title', v.title || '');
  }
  const videoLink = qs('.video-link');
  if (videoLink && videoLink.getAttribute('href') !== v.url) videoLink.setAttribute('href', v.url);
  
  const channel = getChannelFromId(v.from_channel);
  const channelLink = qs('.video-channel-link');
  if (channelLink) {
    setText(channelLink, channel ? channel.name : 'Unknown Channel');
    const channelHref = `/videos?channel=${v.from_channel}`;
    if (channelLink.getAttribute('href') !== channelHref) channelLink.setAttribute('href', channelHref);
    const channelOnClick = `event.preventDefault();filterVideosTabByChannel('${v.from_channel}');`;
    if (channelLink.getAttribute('onclick') !== channelOnClick) channelLink.setAttribute('onclick', channelOnClick);
  }
  const uploaderPart = qs('.video-uploader-part');
  if (uploaderPart) {
    // encodeURIComponent keeps the attribute quote-safe (uploader names
    // may contain quotes, which escHtml does not escape for attributes).
    const uploaderSig = `${encodeURIComponent(v.uploader || '')}|${encodeURIComponent(v.uploader_url || '')}`;
    if ((uploaderPart.dataset.sig || '') !== uploaderSig) {
      uploaderPart.dataset.sig = uploaderSig;
      uploaderPart.innerHTML = videoUploaderInnerHTML(v);
    }
  }
  setText(qs('.video-release-line'), `Released: ${formatDateAndTime(v.release_date)}`);
  setText(qs('.video-availability-line'), v.availability || '');
  setText(qs('.video-added-line'), videoAddedLineText(v));
  
  const tasksLine = qs('.video-tasks-line');
  if (tasksLine) {
    const tasksSig = videoTasksSig(v);
    if ((tasksLine.dataset.sig || '') !== tasksSig) {
      tasksLine.dataset.sig = tasksSig;
      tasksLine.innerHTML = videoTasksInnerHTML(v);
    }
  }
  
  // Don't yank the status badge while its dropdown is open for this video.
  let dropdownVideoId = null;
  const openDropdownOption = document.querySelector('.status-dropdown .status-option');
  if (openDropdownOption) dropdownVideoId = openDropdownOption.dataset.videoId;
  
  const statusWrap = qs('.video-status-wrap');
  if (statusWrap && dropdownVideoId !== v.id) {
    if (String(statusWrap.dataset.status) !== String(v.status)) {
      statusWrap.dataset.status = v.status;
      statusWrap.innerHTML = videoStatusBadge(v.id, v.status);
    }
  }
  const typeWrap = qs('.video-type-wrap');
  if (typeWrap) {
    const typeSig = v.video_type !== undefined ? String(v.video_type) : '';
    if ((typeWrap.dataset.type || '') !== typeSig) {
      typeWrap.dataset.type = typeSig;
      typeWrap.innerHTML = v.video_type !== undefined ? videoTypeBadge(v.video_type) : '';
    }
  }
  const menuWrap = qs('.video-menu-wrap');
  if (menuWrap) {
    const filePresent = v.videofile_exists ? '1' : '0';
    const refreshing = v.refresh_state ? '1' : '0';
    if ((menuWrap.dataset.present || '0') !== filePresent) menuWrap.dataset.present = filePresent;
    if ((menuWrap.dataset.refreshing || '0') !== refreshing) menuWrap.dataset.refreshing = refreshing;
    // The dropdown is built fresh from currentVideos on open, so a stale
    // open menu just needs closing when its data changed.
    const openMenu = document.querySelector('.video-menu-dropdown');
    if (openMenu && openMenu.dataset.videoId === v.id) {
      const openFileItem = openMenu.querySelector('[data-action="video-file"]');
      const openRefreshItem = openMenu.querySelector('[data-action="refresh"]');
      const openDownloadItem = openMenu.querySelector('[data-action="download"]');
      const fileChanged = !!openFileItem !== !!v.videofile_exists;
      const refreshChanged = !!openRefreshItem &&
        ((openRefreshItem.textContent.trim() === 'Refreshing...') !== !!v.refresh_state);
      const canDownload = (v.status != 2 && v.status != 1);
      const downloadChanged = !!openDownloadItem !== canDownload;
      if (fileChanged || refreshChanged || downloadChanged) closeVideoMenu();
    }
  }

  return true;
}

function softRenderVideos() {
  for (const v of currentVideos) {
    if (!renderUpdateVideo(v)) {
      // Card missing (shouldn't happen when the id list matches):
      // fall back to a full rebuild to stay in sync.
      renderVideos();
      return;
    }
  }
}

function clearVideoFilters(dontLoadVideos) {
  document.getElementById('videoSearch').value = '';
  document.getElementById('videoStatusFilter').value = '';
  //document.getElementById('videoChannelFilter').value = '';
  document.getElementById('videoOrderBy').value = 'release_date';
  document.getElementById('videoOrderDirection').value = '-1';
  videoSearchFilterUpdate(true);
  videoPage = 0;
  syncVideosUrl();
  if (!areVideosLoading && !dontLoadVideos) {
    loadVideos();
  } else if (!dontLoadVideos) {
    videoSearchDidUpdate = true;
  }
}

let videoSearchDidUpdate = false;
function videoSearchFilterUpdate(brurhburh) {
  if (!brurhburh) {
    videoSearchDidUpdate = true;
  }
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

// ========== Videos URL state ==========
// Keeps page + filters in the URL (e.g. /videos?page=2&status=2).
// Params matching VIDEO_FILTER_DEFAULTS (and page 1) are omitted
// so the URL stays short.

function ensureVideoChannelOption(channelId) {
  if (!channelId) return;
  const videoChannelFilterEl = document.getElementById('videoChannelFilter');
  if (!videoChannelFilterEl) return;
  const optionExists = Array.from(videoChannelFilterEl.options).some(opt => opt.value === channelId);
  if (!optionExists) {
    const newOption = new Option(channelId, channelId);
    newOption.hidden = true;
    videoChannelFilterEl.add(newOption);
  }
}

function getVideoFilterStateFromUI() {
  return {
    channel:  document.getElementById('videoChannelFilter').value,
    search:   document.getElementById('videoSearch').value,
    status:   document.getElementById('videoStatusFilter').value,
    orderBy:  document.getElementById('videoOrderBy').value,
    orderDir: document.getElementById('videoOrderDirection').value,
    page:     videoPage,
  };
}

function buildVideosUrl(state) {
  const s = state || getVideoFilterStateFromUI();
  const params = new URLSearchParams();
  if (s.channel && s.channel !== VIDEO_FILTER_DEFAULTS.channel) {
    params.set('channel', s.channel);
  }
  if (s.search && s.search.trim() !== '') {
    params.set('search', s.search);
  }
  if (s.status && s.status !== VIDEO_FILTER_DEFAULTS.status) {
    params.set('status', s.status);
  }
  if (s.orderBy && s.orderBy !== VIDEO_FILTER_DEFAULTS.orderBy) {
    params.set('order_by', s.orderBy);
  }
  if (s.orderDir && s.orderDir !== VIDEO_FILTER_DEFAULTS.orderDir) {
    params.set('order_direction', s.orderDir);
  }
  if (s.page && s.page > 0) {
    // URL is 1-indexed for humans, videoPage is 0-indexed internally.
    params.set('page', String(s.page + 1));
  }
  const query = params.toString();
  return query ? `/videos?${query}` : '/videos';
}

function syncVideosUrl(push) {
  if (lastPageOpen !== 'videos') return;
  const url = buildVideosUrl();
  if (push) {
    // History point for page-index changes only (filters/search stay
    // replace-only so typing doesn't spam history). The same-URL guard
    // keeps e.g. double-clicks from stacking duplicate entries.
    if (window.location.pathname + window.location.search !== url) {
      history.pushState({}, '', url);
    }
    return;
  }
  window.history.replaceState(null, '', url);
}

function applyVideosUrlParamsToUI(urlParams) {
  const validStatuses = new Set(['', '0', '1', '2', '3', '4']);
  const validOrderBy = new Set(['release_date', 'added_at', 'updated_at', 'file_size']);
  const validOrderDir = new Set(['-1', '1']);

  const channelId = urlParams.get('channel') || VIDEO_FILTER_DEFAULTS.channel;
  ensureVideoChannelOption(channelId);
  document.getElementById('videoChannelFilter').value = channelId;
  updateVideoChannelActions();

  const search = urlParams.get('search') || VIDEO_FILTER_DEFAULTS.search;
  document.getElementById('videoSearch').value = search;

  const status = urlParams.get('status') ?? VIDEO_FILTER_DEFAULTS.status;
  document.getElementById('videoStatusFilter').value = validStatuses.has(status) ? status : VIDEO_FILTER_DEFAULTS.status;

  const orderBy = urlParams.get('order_by') || VIDEO_FILTER_DEFAULTS.orderBy;
  document.getElementById('videoOrderBy').value = validOrderBy.has(orderBy) ? orderBy : VIDEO_FILTER_DEFAULTS.orderBy;

  const orderDir = urlParams.get('order_direction') || VIDEO_FILTER_DEFAULTS.orderDir;
  document.getElementById('videoOrderDirection').value = validOrderDir.has(orderDir) ? orderDir : VIDEO_FILTER_DEFAULTS.orderDir;

  let page = 0;
  if (urlParams.has('page')) {
    const parsed = parseInt(urlParams.get('page'), 10);
    if (!isNaN(parsed) && parsed >= 1) {
      // URL is 1-indexed, internal videoPage is 0-indexed.
      page = parsed - 1;
    } else {
      page = 0;
    }
  }
  videoPage = page;
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
  // Filters changed -> back to first page (drops any stale ?page=N).
  videoPage = 0;
  syncVideosUrl();
  updateVideosTabTitle();
  updateVideoChannelActions();

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
    prev: () => { if (!areVideosLoading && videoPage > 0) { videoPage--; syncVideosUrl(true); loadVideos(); } },
    next: () => { if (!areVideosLoading) { videoPage++; syncVideosUrl(true); loadVideos(); } },
    page: (p) => { if (!areVideosLoading && p !== videoPage) { videoPage = p; syncVideosUrl(true); loadVideos(); } },
  });
  
  const paginationHtml = buildPaginationHTML({
    currentPage: videoPage,
    totalPages: totalPages,
    totalCount: videoTotalCount,
    currentItems: currentVideos.length,
    label: 'videos',
    cbsId: cbsId,
  });
  
  if (videoPage >= totalPages) {
    videoPage = totalPages-1;
  }
  
  if (container_top)    container_top.innerHTML = paginationHtml;
  if (container_bottom) container_bottom.innerHTML = paginationHtml;
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
// Shows "Check videos now" / "Edit" next to the channel filter when a
// channel is selected, so you don't have to hop to the channels tab.
function updateVideoChannelActions() {
  const channelId = document.getElementById('videoChannelFilter').value;
  const hasChannel = !!channelId;
  for (const id of ['videoChannelCheckBtn', 'videoChannelEditBtn']) {
    const btn = document.getElementById(id);
    if (btn) btn.hidden = !hasChannel;
  }
}

function runSelectedVideoChannelCheck() {
  const channelId = document.getElementById('videoChannelFilter').value;
  if (!channelId) return;
  runChannelCheck(channelId);
  showToast('Channel check started', 'success');
}

function openSelectedVideoChannelEditor() {
  const channelId = document.getElementById('videoChannelFilter').value;
  if (!channelId) return;
  if (!getChannelFromId(channelId)) {
    showToast('Channel data not loaded yet, try again in a moment', 'error');
    return;
  }
  openEditChannelModal(channelId);
}

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

  updateVideoChannelActions();
}

