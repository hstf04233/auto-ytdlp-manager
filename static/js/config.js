// config.js - program config form.
async function loadConfig() {
  try {
    const data = await API.get('/api/config');
    programConfig = data;
    
    renderConfig(programConfig);
  } catch (err) {
    showToast(`Failed to load program config: ${err.message}`, 'error');
  }
}

function cancelConfigChanges() {
  renderConfig(programConfig);
}

// Set .value and .placeholder to the same value.
function setInputPV(el, value) {
  el.placeholder = value;
  el.value       = value;
}

const configSettings = [
  {element: "config-YtDlpPath",     value: "YtDlp_Path",     type: "string"},
  {element: "config-YtArchivePath", value: "YtArchive_Path", type: "string"},
  {element: "config-FFmpegPath",    value: "FFmpeg_Path",    type: "string"},
  
  {element: "config-DownloadDir",        value: "Default_DownloadDir",               type: "string"},
  {element: "config-OutputTemplate",     value: "Default_YtDlp_OutputTemplate",      type: "string"},
  {element: "config-OutputTemplateLive", value: "Default_YtDlp_OutputTemplate_Live", type: "string"},
  
  {element: "config-AllChannelsDisabled",      value: "AllChannels_Disabled",       type: "bool"},
  {element: "config-TaskLogAutoDeleteEnabled", value: "TaskLog_AutoDelete_Enabled", type: "bool"},
  {element: "config-DownloadVideoThumbnails", value: "Download_Video_Thumbnails",  type: "bool"},
  {element: "config-DownloadLiveChat",        value: "Download_Live_Chat",         type: "bool"},
  
  {element: "config-TaskLogAutoDeleteSeconds",     value: "TaskLog_AutoDelete_Seconds",      type: "number"},
  {element: "config-TaskLogListAutoDeleteSeconds", value: "TaskLog_List_AutoDelete_Seconds", type: "number"},
  {element: "config-AutoRefreshVideosSeconds",     value: "AutoRefresh_Videos_Seconds",      type: "number"},
];

function areThereConfigChanges() {
  for (const cfg of configSettings) {
    const element = document.getElementById(cfg.element);
    if (!element) continue;
    
    if (cfg.type == "string") {
      if (element.value !== programConfig[cfg.value]) return true;
    } else if (cfg.type == "number") {
      if (parseFloat(element.value) !== programConfig[cfg.value]) return true;
    } else if (cfg.type == "bool") {
      if (element.checked !== programConfig[cfg.value]) return true;
    }
  }
  
  return false;
}

async function saveConfig(event) {
  event.preventDefault();
  
  let body = {};
  
  for (const cfg of configSettings) {
    const element = document.getElementById(cfg.element);
    if (!element) continue;
    
    if (cfg.type == "string") {
      if (element.value !== programConfig[cfg.value]) {
        body[cfg.value] = element.value;
      }
    } else if (cfg.type == "number") {
      if (parseFloat(element.value) !== programConfig[cfg.value]) {
        body[cfg.value] = parseFloat(element.value);
      }
    } else if (cfg.type == "bool") {
      if (element.checked !== programConfig[cfg.value]) {
        body[cfg.value] = element.checked;
      }
    }
  }
  
  try {
    const newProgramConfig = await API.patch("/api/config", body);
    showToast('Config updated!', 'success');
    
    if (newProgramConfig) {
      programConfig = newProgramConfig;
      renderConfig(programConfig);
    } else {
      loadConfig();
    }
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
}

function updateConfigChanges() {
  const configSubmitBtn = document.getElementById("configSubmitBtn");
  const configCancelBtn = document.getElementById("configCancelBtn");
  wereChangesMade = areThereConfigChanges();
  
  if (configSubmitBtn) {
    configSubmitBtn.disabled = !wereChangesMade;
  }
  if (configCancelBtn) {
    configCancelBtn.disabled = !wereChangesMade;
  }
}

document.getElementById("configForm").addEventListener('input', updateConfigChanges);

function renderConfig(config) {
  const ApplicationVersionEl = document.getElementById("side-bar-application-version");
  if (ApplicationVersionEl) {
    ApplicationVersionEl.textContent = config.application_version;
    ApplicationVersionEl.title = config.application_version;
  }
  
  for (const cfg of configSettings) {
    const element = document.getElementById(cfg.element);
    if (!element) continue;
    
    if (cfg.type == "string" || cfg.type == "number") {
      setInputPV(element, programConfig[cfg.value]);
    } else if (cfg.type == "bool") {
      element.checked = programConfig[cfg.value];
    }
  }
  
  const AllChannelsDisabledEl2 = document.getElementById("channels-config-AllChannelsDisabled");
  if (AllChannelsDisabledEl2) {
    AllChannelsDisabledEl2.checked = programConfig.AllChannels_Disabled;
  }
  
  updateConfigChanges();
}


// ========== Channels master toggle (sidebar) ==========
async function quickToggleDisableAllChannels(cb) {
  try {
    let isDisabled = cb.checked
    const newProgramConfig = await API.patch("/api/config", {
        AllChannels_Disabled: isDisabled
      }
    );
    
    if (newProgramConfig) {
      programConfig = newProgramConfig;
      renderConfig(programConfig);
    }
  } catch (err) {
    showToast(`Failed to update AllChannels_Disabled: ${err.message}`, 'error');
    loadConfig();
  }
}
