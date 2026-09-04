// format.js - badges, formatters, video status dropdown.
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
  const kb = Math.ceil(sizeInBytes/1024*10)/10;   // Rounded by one decimal place.
  if (kb < 1000) {
    return `${kb} KB`
  }
  const mb = Math.ceil(sizeInBytes/1024/1024*10)/10;
  if (mb < 1000) {
    return `${mb} MB`
  }
  const gb = Math.ceil(sizeInBytes/1024/1024/1024*10)/10;
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
  const videoData = currentVideos.find(x => x.id === videoId);
  
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
    const videoData = currentVideos.find(x => x.id === videoId);
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
      
      await API.post(`/api/add-videos?no_wait=true&queue_video_id=${encodeURIComponent(videoId)}`, body);
      showToast('Status changed', 'success');
      loadVideos();
      return;
    }
    
    await API.patch(`/api/videos/${encodeURIComponent(videoId)}`, { status: parseInt(newStatus) });
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

