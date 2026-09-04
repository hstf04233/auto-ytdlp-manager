// channels.js - channels list, channel modals, add-videos form.
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
    
    let isEnabled = (ch.enabled && !programConfig.AllChannels_Disabled && !ch.is_being_checked);
    
    if (delta <= 1 || !isEnabled) {
      channelCheckBtnEl.disabled = true;
    } else {
      channelCheckBtnEl.disabled = false;
    }
    
    let disabledText = "DISABLED!"
    if (ch.is_being_checked) {
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
    let countText = `${ch.tasks_count}`;
    if (ch.tasks_count >= 100) {
      countText = "99+"
    }
    let tasksButtonText = `View ${countText} Logs`;
    if (ch.active_task) {
      tasksButtonText = `View Active Task + ${countText} Logs`;
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
  document.getElementById('channelDownloadDir').value = '';
  document.getElementById('channelOutputTemplate').value = '';
  
  document.getElementById('channelQuality').value = '0';
  document.getElementById('channelPreferredVideoFormat').value = "h264,h265,av01";
  document.getElementById('channelPreferredAudioFormat').value = "aac";
  
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
  document.getElementById('channelPreferredVideoFormat').value = ch.preferred_video_format;
  document.getElementById('channelPreferredAudioFormat').value = ch.preferred_audio_format;
  
  document.getElementById('channelDownloadDir').value = ch.download_dir || '';
  document.getElementById('channelOutputTemplate').value = ch.output_template || '';
  document.getElementById('channelCheckInterval').value = ch.check_interval || '';
  document.getElementById('channelPlaylistEnd').value = ch.playlist_end;
  document.getElementById('channelSubmitBtn').textContent = 'Save Changes';
  
  document.body.classList.add('modal-active');
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
  const preferredVideoFormat = document.getElementById('channelPreferredVideoFormat').value;
  const preferredAudioFormat = document.getElementById('channelPreferredAudioFormat').value;
  
  const downloadDir = document.getElementById('channelDownloadDir').value.trim();
  const outputTemplate = document.getElementById('channelOutputTemplate').value.trim();
  const checkInterval  = document.getElementById('channelCheckInterval').value ? parseInt(document.getElementById('channelCheckInterval').value) : 1800;
  const playlistEnd = parseInt(document.getElementById('channelPlaylistEnd').value);

  const body = {
    name,
    url,
    download_dir: downloadDir,
    output_template: outputTemplate,
    type,
    
    quality_select: quality,
    preferred_video_format: preferredVideoFormat,
    preferred_audio_format: preferredAudioFormat,
    
    check_interval: checkInterval,
    playlist_end: playlistEnd,
  };

  try {
    let newChannelData = null;
    if (id) {
      // Edit
      const patch = {};
      if (name) patch.name = name;
      if (url)  patch.url = url;
      patch.download_dir = downloadDir;
      patch.output_template = outputTemplate;
      patch.type = type;
      
      patch.quality_select = quality;
      patch.preferred_video_format = preferredVideoFormat;
      patch.preferred_audio_format = preferredAudioFormat;
      
      patch.check_interval = checkInterval;
      patch.playlist_end = playlistEnd;
      newChannelData = await API.patch(`/api/channels/${id}`, patch);
      showToast('Channel updated!', 'success');
      
      closeChannelModal();
    } else {
      // Create
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

