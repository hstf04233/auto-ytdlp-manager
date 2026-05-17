let chatMessages = [];
let chatItemIdMap = {};
let chatLoaded = false;

function formatChatTime(timestampUsec) {
  const ts = parseInt(timestampUsec, 10);
  const date = new Date(ts / 1000);
  const now = new Date();
  const diffMs = now - date;
  if (diffMs < 0) return '';
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) return `${diffSec}s`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h`;
  const diffDay = Math.floor(diffHr / 24);
  if (diffDay < 7) return `${diffDay}d`;
  return date.toLocaleDateString();
}

function formatMessageText(runs) {
  if (!runs || !runs.length) return '';
  let html = '';
  for (const run of runs) {
    if (run.emoji) {
      const emojiData = run.emoji;
      let imgUrl = '';
      if (emojiData.image && emojiData.image.thumbnails && emojiData.image.thumbnails.length) {
        imgUrl = emojiData.image.thumbnails[0].url;
      }
      if (imgUrl) {
        html += `<img src="${escHtml(imgUrl)}" class="chat-emoji" alt="emoji" title="${escHtml(emojiData.image.accessibility.accessibilityData.label)}">`;
      } else if (emojiData.searchTerms) {
        html += `<span class="chat-emoji-text" title="${escHtml(emojiData.searchTerms)}">${escHtml(emojiData.searchTerms)}</span>`;
      }
    } else if (run.text) {
      html += escHtml(run.text);
    }
  }
  return html;
}

function renderChatMessage(msg) {
  const authorName = msg.authorName?.simpleText || 'Unknown User';
  const authorId = msg.authorExternalChannelId || '';
  const avatarUrl = '';
  let thumbnails = msg.authorPhoto?.thumbnails || [];
  if (thumbnails.length) {
    for (const t of thumbnails) {
      if (t.width >= 44) {
        avatarUrl = t.url;
        break;
      }
    }
    if (!avatarUrl) avatarUrl = thumbnails[thumbnails.length - 1].url;
  }
  const messageHtml = formatMessageText(msg.message?.runs || []);
  const timestamp = msg.timestampUsec || '';
  const timeStr = timestamp ? formatChatTime(timestamp) : '';
  const msgId = msg.id || '';

  return `
    <div class="chat-message ${msgId ? '' : ''}" data-id="${escHtml(msgId)}" data-author="${escHtml(authorId)}">
      <div class="chat-avatar">
        ${avatarUrl
          ? `<img src="${escHtml(avatarUrl)}" alt="">`
          : `<div class="chat-avatar-default">${escHtml(authorName.charAt(1))}</div>`
        }
      </div>
      <div class="chat-message-body">
        <div class="chat-message-header">
          <span class="chat-author-name">${escHtml(authorName)}</span>
          <span class="chat-timestamp">${timeStr}</span>
        </div>
        <div class="chat-message-text">${messageHtml}</div>
      </div>
    </div>
  `;
}

function addChatMessage(msg) {
  const msgId = msg.id;
  if (!msgId) return;

  chatMessages.push(msg);
  chatItemIdMap[msgId] = msg;

  const container = document.getElementById('chatMessagesContainer');
  const msgHtml = renderChatMessage(msg);

  if (container.querySelector('.chat-placeholder')) {
    container.innerHTML = '';
  }

  const temp = document.createElement('div');
  temp.innerHTML = msgHtml;
  const el = temp.firstElementChild;
  container.appendChild(el);

  const autoScroll = document.getElementById('autoScrollToggle');
  if (autoScroll && autoScroll.checked) {
    container.scrollTop = container.scrollHeight;
  }
}

function removeChatItemById(itemId) {
  const container = document.getElementById('chatMessagesContainer');
  const el = container.querySelector(`.chat-message[data-id="${itemId}"]`);
  if (el) {
    el.remove();
    delete chatItemIdMap[itemId];
  }
}

function removeChatItemsByAuthor(authorId) {
  const container = document.getElementById('chatMessagesContainer');
  const els = container.querySelectorAll(`.chat-message[data-author="${authorId}"]`);
  for (const el of els) {
    const msgId = el.dataset.id;
    el.remove();
    if (msgId) delete chatItemIdMap[msgId];
  }
}

function clearChatView() {
  chatMessages = [];
  chatItemIdMap = {};
  chatLoaded = false;
  const container = document.getElementById('chatMessagesContainer');
  container.innerHTML = '<div class="chat-placeholder">Enter a Chat ID and click "Load Chat" to view messages.</div>';
  document.getElementById('chatStats').innerHTML = '';
}

function processChatActions(actionsRaw) {
  const container = document.getElementById('chatMessagesContainer');
  if (container.querySelector('.chat-placeholder')) {
    container.innerHTML = '';
  }

  const lines = actionsRaw.trim().split('\n');
  let addCount = 0;
  let removeCount = 0;

  for (const line of lines) {
    if (!line.trim()) continue;
    try {
      const action = JSON.parse(line);

      if (action.addChatItemAction) {
        const msg = action.addChatItemAction.item;
        if (msg && (msg.liveChatTextMessageRenderer || msg.liveChatMessageDeletedRenderer)) {
          const renderer = msg.liveChatTextMessageRenderer || msg.liveChatMessageDeletedRenderer;
          addChatMessage(renderer);
          addCount++;
        }
      } else if (action.removeChatItemAction) {
        const targetId = action.removeChatItemAction.targetItemId;
        if (targetId) {
          removeChatItemById(targetId);
          removeCount++;
        }
      } else if (action.removeChatItemByAuthorAction) {
        const extId = action.removeChatItemByAuthorAction.externalChannelId;
        if (extId) {
          removeChatItemsByAuthor(extId);
          removeCount++;
        }
      }
    } catch (e) {
      console.warn('Failed to parse chat action:', line.substring(0, 100), e);
    }
  }

  const statsEl = document.getElementById('chatStats');
  const totalItems = document.getElementById('chatMessagesContainer').querySelectorAll('.chat-message').length;
  statsEl.innerHTML = `
    <span class="chat-stat">Messages: ${totalItems}</span>
    <span class="chat-stat">Added: ${addCount}</span>
    <span class="chat-stat">Removed: ${removeCount}</span>
  `;

  chatLoaded = true;
}

async function loadChat() {
  const chatId = document.getElementById('chatIdInput').value.trim();
  if (!chatId) {
    showToast('Please enter a Chat ID', 'error');
    return;
  }

  const container = document.getElementById('chatMessagesContainer');
  container.innerHTML = '<div class="loading">Loading chat...</div>';
  document.getElementById('chatStats').innerHTML = '';

  try {
    const res = await fetch(`/api/chat/${encodeURIComponent(chatId)}`);
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API ${res.status}: ${text}`);
    }
    const text = await res.text();
    clearChatView();
    processChatActions(text);
  } catch (err) {
    showToast(`Failed to load chat: ${err.message}`, 'error');
    container.innerHTML = '<div class="loading">Failed to load chat. Check the console for details.</div>';
  }
}

document.addEventListener('DOMContentLoaded', () => {
  const container = document.getElementById('chatMessagesContainer');
  if (container) {
    container.addEventListener('wheel', (e) => {
      const autoScroll = document.getElementById('autoScrollToggle');
      if (autoScroll && autoScroll.checked) {
        const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 50;
        if (isAtBottom) {
          e.preventDefault();
          container.scrollTop = container.scrollHeight;
        }
      }
    });
  }
});
