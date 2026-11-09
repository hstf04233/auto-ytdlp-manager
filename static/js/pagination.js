// pagination.js - shared page navigation (videos, tasks and the public share page).
// Has no dependencies, so it can be loaded by any page.

let _pgCallbacks = {}
let _pgId = 0

function _registerPgCbs(cbs) {
  let id = ++_pgId
  if (id > 40) {
    _pgId = 1
    id = _pgId
  }
  _pgCallbacks[id] = cbs
  return id
}

// ========== Shared pagination builder (used by videos + tasks) ==========
function buildPaginationHTML(cfg) {
  const { currentPage, totalPages, totalCount, currentItems, label, cbsId } = cfg;
  
  const btn = (labelText, page, disabled, active) => {
    const disabledAttr = disabled ? ' disabled' : '';
    const activeAttr = active ? ' class="active"' : '';
    let onclick = '';
    if (!disabled) {
      if (page < 0) {
        // Prev
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].prev())"`;
      } else if (page >= totalPages) {
        // Next
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].next())"`;
      } else {
        // Page num
        onclick = ` onclick="(_pgCallbacks[${cbsId}]&&_pgCallbacks[${cbsId}].page(${page}))"`;
      }
    }
    return `<button${activeAttr}${disabledAttr}${onclick}>${labelText}</button>`;
  };
  
  //const singlePageMsg = `All ${label} shown (page 1 of 1)`;
  const countMsg = `Showing ${currentItems} ${label} out of ${totalCount}`;
  
  if (totalPages <= 1 && currentPage < totalPages) {
    return `<span class="single-page-msg">${countMsg}</span>`;
  }
  
  // Page numbers group
  let pageButtons = [];
  
  if (totalPages <= 7) {
    for (let i = 0; i < totalPages; i++) {
      pageButtons.push(btn(String(i + 1), i, false, currentPage === i));
    }
  } else {
    let rangeStart = Math.max(1, currentPage - 2);
    let rangeEnd = Math.min(totalPages - 2, currentPage + 2);
    
    if (rangeStart <= 1) {
      rangeEnd = Math.min(totalPages - 2, rangeStart + 5);
    }
    if (rangeEnd >= totalPages - 3) {
      if (rangeEnd == totalPages - 3) {
        rangeEnd += 1;
      }
      rangeStart = Math.max(1, rangeEnd - 5);
    }
    
    const isFirstActive = currentPage === 0;
    pageButtons.push(btn('1', 0, false, isFirstActive));
    
    if (rangeStart > 2) {
      pageButtons.push(pageEllipsisHTML(cbsId, totalPages, currentPage));
    } else if (rangeStart === 2) {
      pageButtons.push(btn('2', 1, false, false));
    }
    
    for (let i = rangeStart; i <= rangeEnd; i++) {
      pageButtons.push(btn(String(i + 1), i, false, currentPage === i));
    }
    
    if (rangeEnd < totalPages - 2) {
      pageButtons.push(pageEllipsisHTML(cbsId, totalPages, currentPage));
    }
    
    pageButtons.push(btn(String(totalPages), totalPages - 1, false, currentPage === totalPages - 1));
  }
  
  const prevDisabled = currentPage === 0;
  const nextDisabled = currentPage >= totalPages - 1;
  const prevBtn = btn('‹', -1, prevDisabled, false);
  const nextBtn = btn('›', totalPages+100, nextDisabled, false);
  return prevBtn + pageButtons.join('') + nextBtn + `<span class="single-page-msg">${countMsg}</span>`;
}

// Page-gap ellipsis: looks identical to the old static "…" until hovered,
// but clicking it swaps in a page-number field (Enter commits, Esc cancels).
function pageEllipsisHTML(cbsId, totalPages, currentPage) {
  return `<button class="page-ellipsis" title="Go to page..." onclick="pageEllipsisToInput(this, ${cbsId}, ${totalPages}, ${currentPage})">…</button>`;
}

function pageEllipsisToInput(btn, cbsId, totalPages, currentPage) {
  const cbs = _pgCallbacks[cbsId];
  if (!cbs || typeof cbs.page !== 'function') return;
  const input = document.createElement('input');
  input.type = 'number';
  input.min = '1';
  input.max = String(totalPages);
  input.value = String(currentPage + 1);
  input.className = 'page-jump-input';
  input.setAttribute('aria-label', `Go to page (1 to ${totalPages})`);
  btn.replaceWith(input);
  input.focus();
  input.select();
  let settled = false;
  const restore = () => {
    if (settled) return;
    settled = true;
    if (input.parentNode) input.replaceWith(btn);
  };
  const commit = () => {
    if (settled) return;
    const n = parseInt(input.value, 10);
    restore();
    // Clamp into range; empty/garbage just closes the field.
    if (!isNaN(n)) cbs.page(Math.min(Math.max(n, 1), totalPages) - 1);
  };
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.stopPropagation();
      commit();
    } else if (e.key === 'Escape') {
      e.stopPropagation();
      restore();
    }
  });
  input.addEventListener('blur', commit);
}
