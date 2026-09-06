// tasks.js - tasks/logs list, output viewer.
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

  ensureVideoChannelOption(channelId);
  videoPage = 0;
  document.getElementById('videoChannelFilter').value = channelId;
  updateVideoChannelActions();

  showPage(`videos?channel=${encodeURIComponent(channelId)}`);
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
    
    let url = `/api/tasks?limit=${TASK_PAGE_SIZE}&page=${taskPage}`;
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

  // On mobile the output panel sits below the list: bring it into view.
  if (isMobileViewport()) {
    const panel = document.getElementById('taskOutputPanel');
    if (panel) {
      panel.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
  }
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
  }).join('<br>');
  
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
      
      const outputContainer = document.getElementById('taskOutputContent');
      const isAtBottom = outputContainer.scrollHeight - outputContainer.scrollTop <= outputContainer.clientHeight + 4;
      
      const formatedDuration = getFormatedTaskDuration(selectedTask);
      
      const text = await res.text();
      outputContainer.innerHTML = formatTerminalOutput(text, selectedTask.status, selectedTask.run_args);
      
      // Auto-scroll to bottom
      if (isAtBottom) {
        outputContainer.scrollTop = outputContainer.scrollHeight;
      }
      
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

