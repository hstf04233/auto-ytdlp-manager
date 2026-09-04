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
    
    document.getElementById('videoDetailsModal').classList.remove('active');
    document.body.classList.remove('modal-active');
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
      <img src="${escHtml(thumbnailUrl)}" alt="" onclick="${videoPreviewOnClick}" onerror="this.onerror=null; this.src='/static/images/NoThumbnail_bw.jpg';"
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
    `<a href="/video-file/${escHtml(videoInfo.id)}?download=true" target="_blank" class="btn btn-secondary btn-sm" title="Download video file">Download Video</a>` :
    ''
    }
    ${
      videoInfo.videofile_exists ?
    `<button type="button" class="btn btn-secondary btn-sm" class="btn btn-secondary btn-sm" onclick="copySharedVideoFile('${videoInfo.id}')" title="Copy shared video file link to clipboard">Share video file link</a>` :
    ''
    }
    <button type="button" class="btn btn-secondary btn-sm" ${refreshDisabled} onclick="refreshVideoInfo('${videoInfo.id}');closeVideoDetailsModal();" title="${refreshTitle}">${videoInfo.refresh_state ? 'Refreshing...' : 'Refresh'}</button>
    <button type="button" class="btn btn-danger btn-sm" onclick="deleteVideo('${videoInfo.id}');closeVideoDetailsModal();">Delete</button>
  `;
}

function closeVideoDetailsModal() {
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
}

// ========== Videos ==========
async function loadVideos() {
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
    renderVideos();
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

function renderVideos() {
  const container = document.getElementById('videosList');

  if (currentVideos.length === 0) {
    container.innerHTML = '<div class="loading">No videos found.</div>';
    return;
  }

  container.innerHTML = currentVideos.map(v => {
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
          <p>From: <a title="Filter by this channel" href="/videos?channel=${v.from_channel}" onclick="event.preventDefault();filterVideosTabByChannel('${v.from_channel}');">${channel ? escHtml(channel.name) : 'Unknown Channel'}</a>
          ${v.uploader ? `Uploader: <a href="${escHtml(v.uploader_url)}" target="_blank">${escHtml(v.uploader)}</a>` : ''}
          </p>
          <p>Released: ${formatDateAndTime(v.release_date)}</p>
          <p>${escHtml(v.availability)}</p>
          <p>Added ${formatRelative(v.added_at)} \u00b7 Updated ${formatRelative(v.updated_at)} ${v.filesize ? formatBytesSize(v.filesize) : ''}</p>
          
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
          <a class="btn btn-secondary btn-sm" href="/video/${v.id}" onclick="event.preventDefault(); openVideoDetailsModal('${v.id}');">Details</a>
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

function syncVideosUrl() {
  if (lastPageOpen !== 'videos') return;
  window.history.replaceState(null, '', buildVideosUrl());
}

function applyVideosUrlParamsToUI(urlParams) {
  const validStatuses = new Set(['', '0', '1', '2', '3', '4']);
  const validOrderBy = new Set(['release_date', 'added_at', 'updated_at', 'file_size']);
  const validOrderDir = new Set(['-1', '1']);

  const channelId = urlParams.get('channel') || VIDEO_FILTER_DEFAULTS.channel;
  ensureVideoChannelOption(channelId);
  document.getElementById('videoChannelFilter').value = channelId;

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
    prev: () => { if (!areVideosLoading && videoPage > 0) { videoPage--; syncVideosUrl(); loadVideos(); } },
    next: () => { if (!areVideosLoading) { videoPage++; syncVideosUrl(); loadVideos(); } },
    page: (p) => { if (!areVideosLoading) { videoPage = p; syncVideosUrl(); loadVideos(); } },
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

