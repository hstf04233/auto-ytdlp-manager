// main.js - cross-page navigation, global listeners, init. Load last.
function navigateToChannel(channelId) {
  gotoVideosPageAndFilterChannel(channelId);
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
  showPage(tab + window.location.search, true);

  updateVideosTabTitle()
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    closeVideoDetailsModal();
    closeChannelModal();
    closeAddVideosModal();
  }
});

async function logout() {
  try {
    const res = await fetch("/logout", {
      method: 'POST',
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(text);
    }
    
    window.location.href = "/";
  } catch (err) {
    showToast(`Failed to log out... error: ${err.message}`, "error")
  }
}

async function loadUser() {
  try {
    const loggedInUser = await API.get('/api/whoami');
    
    const userAccountEl = document.getElementById("side-bar-user-account")
    if (userAccountEl) {
      userAccountEl.textContent = loggedInUser.username
      userAccountEl.title = `Logged in as: ${loggedInUser.username}`
    }
  } catch (err) {
    showToast(`Failed to get logged in user... error: ${err.message}`, "error")
  }
}

async function init() {
  videoSearchFilterUpdate(true);
  
  loadUser();
  loadConfig();
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
  
  // Auto refresh videos every 10s (soft: patches cards in place, no rebuild flicker)
  setInterval(() => {
    if (areVideosLoading || document.hidden) {
      return;
    }

    const videosPage = document.getElementById('page-videos');
    if (videosPage.classList.contains('active')) {
      loadVideos(true);
    }
  }, 10_000);
  
  // Check search updates every 300ms (at best)
  setInterval(() => {
    if (areVideosLoading) {
      return;
    }
    if (videoSearchDidUpdate) {
      videoSearchDidUpdate = false;
      videoPage = 0;
      syncVideosUrl();

      loadVideos();
    }
  }, 300);
  
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
}


document.addEventListener("visibilitychange", () => {
  if (!document.hidden) {
    softRenderChannels();
    
    const videosPage = document.getElementById('page-videos');
    if (!areVideosLoading && videosPage.classList.contains('active')) {
      loadVideos(true);
    }
    
    const tasksPage = document.getElementById('page-tasks');
    if (!areTasksLoading && tasksPage.classList.contains('active')) {
      loadTasks();
    }
  }
});

init();
