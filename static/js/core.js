// core.js - API helpers, shared state, page shell, sidebar, toasts. Load first.

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
      console.log(`API GET ${truncateString(url, 128)}: ${res.status} - ${text}`);
      throw new Error(`${res.status} - ${text}`);
    }
    return res.json();
  },
  async getRaw(url) {
    const res = await fetch(url);
    if (!res.ok) {
      const text = await res.text();
      console.log(`API GET ${truncateString(url, 128)}: ${res.status} - ${text}`);
      throw new Error(`${res.status} - ${text}`);
    }
    return res.text();
  },
  async post(url, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!res.ok) {
      const text = await res.text();
      console.log(`API POST ${truncateString(url, 128)}: ${res.status} - ${text}`);
      throw new Error(`${res.status} - ${text}`);
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
      console.log(`API PATCH ${truncateString(url, 128)}: ${res.status} - ${text}`);
      throw new Error(`${res.status} - ${text}`);
    }
    return res.json();
  },
  async del(url, body) {
    const opts = { method: 'DELETE' };
    if (body !== undefined) {
      opts.headers = { 'Content-Type': 'application/json' };
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    if (!res.ok) {
      const text = await res.text();
      console.log(`API DELETE ${truncateString(url, 128)}: ${res.status} - ${text}`);
      throw new Error(`${res.status} - ${text}`);
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
let currentVideos = [];
let videoPage = 0;
let videoTotalCount = 0;
let videoStats = {};
const VIDEO_PAGE_SIZE = 20;

// Defaults for the videos tab. URL params matching these defaults are omitted
// to keep the URL short (no bloated url).
const VIDEO_FILTER_DEFAULTS = {
  channel: '',
  search: '',
  status: '',
  orderBy: 'release_date',
  orderDir: '-1',
};

let lastPageOpen = ''
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
  // The bare root URL "/" carries no tab name: treat it as the default
  // 'channels' tab. Without this, stepping back to "/" (e.g. browser Back
  // after clicking a tab from a fresh "/" load) matches no page div and
  // leaves a blank page.
  const basePage = new URL("/"+page, window.location.origin).pathname.replace(/^\//, '') || 'channels';
  
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
  
  const url = new URL(page, window.location.origin)
  const urlParams = new URLSearchParams(url.search);
  
  if (basePage === 'videos') {
    applyVideosUrlParamsToUI(urlParams);
    videoSearchFilterUpdate(true);

    updateVideosTabTitle();
    title = '';   // Won't set title if blank.

    if (!areVideosLoading) {
      loadVideos();
    }
  }
  if (basePage.startsWith("video/")) {
    const pathArgs = url.pathname.split("/").filter(Boolean);
    const videoId = pathArgs[1];
    
    openVideoDetailsModal(videoId);
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

// ========== Sidebar Toggle ==========
function isMobileViewport() {
  return window.matchMedia('(max-width: 800px)').matches;
}

function setSidebarCollapsed(collapsed, save = true) {
  const sidebar = document.getElementById('sidebar');
  const toggleBtn = document.getElementById('sidebarToggle');
  if (!sidebar) return;

  sidebar.classList.toggle('collapsed', collapsed);
  document.body.classList.toggle('sidebar-collapsed', collapsed);

  if (toggleBtn) {
    toggleBtn.setAttribute('aria-expanded', String(!collapsed));
  }

  // Transient (auto-applied) states are not persisted, so e.g. an automatic
  // mobile collapse never leaks into the desktop preference.
  if (!save) return;
  try {
    localStorage.setItem('sidebarCollapsed', String(collapsed));
  } catch (e) { /* storage unavailable (private mode, etc.) */ }
}

function toggleSidebar() {
  const sidebar = document.getElementById('sidebar');
  if (!sidebar) return;
  setSidebarCollapsed(!sidebar.classList.contains('collapsed'));
}

(function() {
  let stored = null;
  try {
    stored = localStorage.getItem('sidebarCollapsed');
  } catch (e) { /* storage unavailable */ }
  // Mobile always starts collapsed (the expanded sidebar is an overlay
  // drawer, never a useful initial state) without persisting it, so the
  // desktop preference is untouched. Desktop honors the saved preference.
  const isMobile = isMobileViewport();
  if (isMobile || stored === 'true') {
    // Restoring state, not toggling: kill transitions so refresh doesn't
    // visibly animate the sidebar closing, then re-enable them.
    document.body.classList.add('no-transition');
    setSidebarCollapsed(true, !isMobile);
    void document.body.offsetWidth; // flush styles while transitions are off
    document.body.classList.remove('no-transition');
  } else {
    const toggleBtn = document.getElementById('sidebarToggle');
    if (toggleBtn) {
      toggleBtn.setAttribute('aria-expanded', 'true');
    }
  }
})();

// ========== Navigation ==========
document.querySelectorAll('.sidebar nav a').forEach(link => {
  link.addEventListener('click', e => {
    e.preventDefault();
    const page = link.dataset.page;
    showPage(page)
    // On mobile the expanded sidebar overlays content: get out of the way.
    if (isMobileViewport()) {
      setSidebarCollapsed(true, false);
    }
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


// ========== HTML escaping (also used by chat.js, which loads after these files) ==========
function escHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
