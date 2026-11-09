// groups.js - channel groups (visual only): grouped channels list, group modal, helpers.
// Groups can be nested one level deep: a top level group can hold sub groups,
// sub groups can only hold channels. Viewing a group includes its sub groups.

let allGroups = [];

// ========== Helpers ==========
function getGroupFromId(id) {
  if (!id) return null;
  return allGroups.find(g => g.id === id) || null;
}

function getTopLevelGroups() {
  return allGroups.filter(g => !g.parent_id || !getGroupFromId(g.parent_id));
}

function getSubGroups(groupId) {
  return allGroups.filter(g => g.parent_id === groupId);
}

// Channels directly inside this group (not inside its sub groups).
function getDirectGroupChannels(groupId) {
  return allChannels.filter(ch => !ch.hidden && ch.group_id === groupId);
}

// Every channel inside this group, including the channels of its sub groups.
function getAllGroupChannels(groupId) {
  const ids = new Set([groupId, ...getSubGroups(groupId).map(g => g.id)]);
  return allChannels.filter(ch => !ch.hidden && ids.has(ch.group_id));
}

function groupDisplayPath(group) {
  if (!group) return '';
  const parent = getGroupFromId(group.parent_id);
  return parent ? `${parent.name} / ${group.name}` : group.name;
}

// Ordered like the channels page: each top level group followed by its sub groups.
function getGroupsInDisplayOrder() {
  const out = [];
  for (const top of getTopLevelGroups()) {
    out.push({ group: top, depth: 0 });
    for (const sub of getSubGroups(top.id)) {
      out.push({ group: sub, depth: 1 });
    }
  }
  return out;
}

// Fills a <select> with "(none)" + groups (sub groups indented).
function fillGroupSelect(select, opts) {
  const { noneLabel, selected, filter } = opts;
  select.innerHTML = '';
  select.appendChild(new Option(noneLabel, ''));
  for (const { group, depth } of getGroupsInDisplayOrder()) {
    if (filter && !filter(group, depth)) continue;
    const prefix = depth > 0 ? '   ↳ ' : '';
    select.appendChild(new Option(prefix + group.name, group.id));
  }
  select.value = getGroupFromId(selected) ? selected : '';
}

function pluralize(n, word) {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}

// ========== Collapsed state (per browser) ==========
function getCollapsedGroups() {
  try {
    return new Set(JSON.parse(localStorage.getItem('collapsedChannelGroups') || '[]'));
  } catch (e) {
    return new Set();
  }
}

function toggleGroupCollapsed(groupId) {
  const collapsed = getCollapsedGroups();
  const el = document.getElementById(`group-${groupId}`);
  if (collapsed.has(groupId)) {
    collapsed.delete(groupId);
  } else {
    collapsed.add(groupId);
  }
  if (el) {
    const isCollapsed = collapsed.has(groupId);
    el.classList.toggle('collapsed', isCollapsed);
    const toggleBtn = el.querySelector(':scope > .channel-group-header .channel-group-toggle');
    if (toggleBtn) toggleBtn.setAttribute('aria-expanded', String(!isCollapsed));
  }
  try {
    localStorage.setItem('collapsedChannelGroups', JSON.stringify([...collapsed]));
  } catch (e) { /* storage unavailable (private mode, etc.) */ }
}

// ========== Channels page rendering ==========
function channelsRenderSig() {
  const ch = allChannels.map(c => `${c.id}:${c.group_id || ''}:${c.name}:${c.url}:${c.type}:${c.quality_select}:${c.check_interval}:${c.hidden ? 1 : 0}`).join('|');
  const gr = allGroups.map(g => `${g.id}:${g.parent_id || ''}:${g.name}`).join('|');
  return ch + '#' + gr;
}

function groupSectionHTML(group, depth, collapsed) {
  const subGroups = depth === 0 ? getSubGroups(group.id) : [];
  const directChannels = getDirectGroupChannels(group.id);
  const totalChannels = getAllGroupChannels(group.id).length;

  const countParts = [];
  if (subGroups.length > 0) countParts.push(pluralize(subGroups.length, 'group'));
  countParts.push(pluralize(totalChannels, 'channel'));

  const isCollapsed = collapsed.has(group.id);
  const gid = escHtml(group.id);

  let body = '';
  for (const ch of directChannels) {
    body += channelCardHTML(ch);
  }
  for (const sub of subGroups) {
    body += groupSectionHTML(sub, 1, collapsed);
  }
  if (directChannels.length === 0 && subGroups.length === 0) {
    body = `<div class="channel-group-empty">This group is empty. <a href="#" onclick="event.preventDefault();openEditGroupModal('${gid}');">Add channels</a></div>`;
  }

  return `
    <section class="channel-group ${depth > 0 ? 'channel-subgroup' : ''} ${isCollapsed ? 'collapsed' : ''}" id="group-${gid}">
      <div class="channel-group-header">
        <button type="button" class="channel-group-toggle" onclick="toggleGroupCollapsed('${gid}')" aria-expanded="${!isCollapsed}" title="Collapse / expand">
          <span class="channel-group-caret" aria-hidden="true">&#9662;</span>
          <span class="channel-group-name">${escHtml(group.name)}</span>
          <span class="channel-group-count">${countParts.join(' · ')}</span>
        </button>
        <div class="channel-group-actions">
          <a class="btn btn-secondary btn-sm" href="/videos?group=${encodeURIComponent(group.id)}" onclick="event.preventDefault();gotoVideosPageAndFilterGroup('${gid}');">View videos</a>
          ${depth === 0 ? `<button type="button" class="btn btn-secondary btn-sm" onclick="openNewGroupModal('${gid}')" title="Create a group inside this group">+ Sub group</button>` : ''}
          <button type="button" class="btn btn-secondary btn-sm" onclick="openEditGroupModal('${gid}')">Edit</button>
          <button type="button" class="btn btn-danger btn-sm" onclick="deleteGroup('${gid}')">Delete</button>
        </div>
      </div>
      <div class="channel-group-body">${body}</div>
    </section>
  `;
}

// Renders the channels list grouped. Ungrouped channels go in their own section
// (or render plainly when there are no groups at all).
function renderGroupedChannels(container) {
  const collapsed = getCollapsedGroups();
  let html = '';

  for (const top of getTopLevelGroups()) {
    html += groupSectionHTML(top, 0, collapsed);
  }

  const ungrouped = allChannels.filter(ch => !ch.hidden && !getGroupFromId(ch.group_id));
  if (ungrouped.length > 0) {
    if (allGroups.length > 0) {
      const isCollapsed = collapsed.has('__ungrouped__');
      html += `
        <section class="channel-group channel-group-ungrouped ${isCollapsed ? 'collapsed' : ''}" id="group-__ungrouped__">
          <div class="channel-group-header">
            <button type="button" class="channel-group-toggle" onclick="toggleGroupCollapsed('__ungrouped__')" aria-expanded="${!isCollapsed}" title="Collapse / expand">
              <span class="channel-group-caret" aria-hidden="true">&#9662;</span>
              <span class="channel-group-name">Ungrouped</span>
              <span class="channel-group-count">${pluralize(ungrouped.length, 'channel')}</span>
            </button>
          </div>
          <div class="channel-group-body">${ungrouped.map(channelCardHTML).join('')}</div>
        </section>
      `;
    } else {
      html += ungrouped.map(channelCardHTML).join('');
    }
  }

  container.innerHTML = html;
  return allChannels.some(ch => !ch.hidden) || allGroups.length > 0;
}

// ========== Group modal ==========
let groupModalSelected = new Set();

function openNewGroupModal(parentId) {
  openGroupModal(null, parentId || '');
}

function openEditGroupModal(id) {
  const group = getGroupFromId(id);
  if (!group) {
    showToast('Group not found, try again in a moment', 'error');
    return;
  }
  openGroupModal(group, group.parent_id || '');
}

function openGroupModal(group, parentId) {
  const isEdit = !!group;
  document.getElementById('groupModalTitle').textContent = isEdit ? 'Edit Group' : (parentId ? 'New Sub Group' : 'New Group');
  document.getElementById('groupSubmitBtn').textContent = isEdit ? 'Save Changes' : 'Create Group';
  document.getElementById('groupId').value = isEdit ? group.id : '';
  document.getElementById('groupName').value = isEdit ? group.name : '';
  document.getElementById('groupChannelSearch').value = '';

  // Parent select: only top level groups (one level of nesting), never itself.
  const hasSubGroups = isEdit && getSubGroups(group.id).length > 0;
  const parentSelect = document.getElementById('groupParent');
  fillGroupSelect(parentSelect, {
    noneLabel: 'None (top level)',
    selected: parentId,
    filter: (g, depth) => depth === 0 && (!isEdit || g.id !== group.id),
  });
  parentSelect.disabled = hasSubGroups;
  const hint = document.getElementById('groupParentHint');
  hint.hidden = !hasSubGroups;
  hint.textContent = hasSubGroups ? 'This group has sub groups, so it has to stay a top level group.' : '';

  groupModalSelected = new Set(isEdit ? getDirectGroupChannels(group.id).map(ch => ch.id) : []);
  renderGroupChannelPicker();

  document.body.classList.add('modal-active');
  document.getElementById('groupModal').classList.add('active');
  setTimeout(() => document.getElementById('groupName').focus(), 0);
}

function closeGroupModal() {
  const modal = document.getElementById('groupModal');
  if (!modal.classList.contains('active')) return;
  modal.classList.remove('active');
  document.body.classList.remove('modal-active');
}

function toggleGroupModalChannel(channelId, checked) {
  if (checked) {
    groupModalSelected.add(channelId);
  } else {
    groupModalSelected.delete(channelId);
  }
  updateGroupChannelsCount();
}

function updateGroupChannelsCount() {
  const el = document.getElementById('groupChannelsCount');
  if (el) el.textContent = groupModalSelected.size > 0 ? `(${groupModalSelected.size} selected)` : '';
}

function renderGroupChannelPicker() {
  const container = document.getElementById('groupChannelPicker');
  const editingId = document.getElementById('groupId').value;
  const query = document.getElementById('groupChannelSearch').value.trim().toLowerCase();

  const channels = allChannels.filter(ch => !ch.hidden);
  if (channels.length === 0) {
    container.innerHTML = '<div class="channel-group-empty">No channels yet. Add a channel first.</div>';
    updateGroupChannelsCount();
    return;
  }

  const visible = channels.filter(ch =>
    !query || ch.name.toLowerCase().includes(query) || ch.url.toLowerCase().includes(query));

  container.innerHTML = visible.map(ch => {
    const checked = groupModalSelected.has(ch.id);
    const currentGroup = getGroupFromId(ch.group_id);
    const inOther = currentGroup && currentGroup.id !== editingId;
    const tag = inOther ? `<span class="gcr-tag" title="Selecting this moves the channel out of its current group">in ${escHtml(groupDisplayPath(currentGroup))}</span>` : '';
    return `
      <label class="group-channel-row">
        <input type="checkbox" ${checked ? 'checked' : ''} onchange="toggleGroupModalChannel('${escHtml(ch.id)}', this.checked)">
        <span class="gcr-text">
          <span class="gcr-name">${escHtml(ch.name)}</span>
          <span class="gcr-url">${escHtml(ch.url)}</span>
        </span>
        ${tag}
      </label>
    `;
  }).join('') || '<div class="channel-group-empty">No channels match.</div>';

  updateGroupChannelsCount();
}

async function saveGroup(e) {
  e.preventDefault();
  const id = document.getElementById('groupId').value;
  const name = document.getElementById('groupName').value.trim();
  const parentSelect = document.getElementById('groupParent');
  if (!name) return;

  const body = {
    name,
    channel_ids: [...groupModalSelected],
  };
  if (!parentSelect.disabled) {
    body.parent_id = parentSelect.value;
  }

  const submitBtn = document.getElementById('groupSubmitBtn');
  submitBtn.disabled = true;
  try {
    if (id) {
      await API.patch(`/api/groups/${encodeURIComponent(id)}`, body);
      showToast(`Group "${name}" updated!`, 'success');
    } else {
      await API.post('/api/groups', body);
      showToast(`Group "${name}" created!`, 'success');
    }
    closeGroupModal();
    await loadChannels();
    reloadVideosIfOpen();
  } catch (err) {
    showToast(`Failed: ${err.message}`, 'error');
  }
  submitBtn.disabled = false;
}

async function deleteGroup(id) {
  const group = getGroupFromId(id);
  if (!group) return;
  const subCount = getSubGroups(id).length;
  let msg = `Delete group "${group.name}"?\n\nNo channels or videos are deleted. Channels in this group become ungrouped`;
  msg += subCount > 0 ? `, and its ${pluralize(subCount, 'sub group')} become top level groups.` : '.';
  if (!confirm(msg)) return;

  try {
    await API.del(`/api/groups/${encodeURIComponent(id)}`);
    showToast(`Group "${group.name}" was deleted!`, 'success');
    // Don't leave the videos tab filtered by a group that no longer exists.
    const videoFilter = document.getElementById('videoChannelFilter');
    if (videoFilter && videoFilter.value === VIDEO_GROUP_FILTER_PREFIX + id) {
      videoFilter.value = '';
      syncVideosUrl();
      updateVideosTabTitle();
      updateVideoChannelActions();
    }
    await loadChannels();
    reloadVideosIfOpen();
  } catch (err) {
    showToast(`Failed to delete group: ${err.message}`, 'error');
  }
}

// Group membership changed: the videos tab may be showing a group, refresh it.
function reloadVideosIfOpen() {
  const videosPage = document.getElementById('page-videos');
  if (videosPage && videosPage.classList.contains('active')) {
    updateVideosTabTitle();
    loadVideos();
  }
}

// ========== Videos page ==========
// The videos "From" filter holds channel ids, or "group:<id>" for groups.
const VIDEO_GROUP_FILTER_PREFIX = 'group:';

function isGroupFilterValue(value) {
  return typeof value === 'string' && value.startsWith(VIDEO_GROUP_FILTER_PREFIX);
}

function groupIdFromFilterValue(value) {
  return isGroupFilterValue(value) ? value.slice(VIDEO_GROUP_FILTER_PREFIX.length) : '';
}

function gotoVideosPageAndFilterGroup(groupId) {
  clearVideoFilters(true);

  const value = VIDEO_GROUP_FILTER_PREFIX + groupId;
  ensureVideoChannelOption(value);
  videoPage = 0;
  document.getElementById('videoChannelFilter').value = value;
  updateVideoChannelActions();

  showPage(`videos?group=${encodeURIComponent(groupId)}`);
  loadVideos();
  renderVideoPagination();
}
