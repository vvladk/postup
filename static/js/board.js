// Board logic — cards, drag-and-drop, voting, action items
document.addEventListener('DOMContentLoaded', function () {
    initCardDragDrop();
    initCardContextMenu();
    initAddCardForms();
    initVoting();
    initActionItems();
});

// ─── Drag and drop ────────────────────────────────────────────────────────────

function attachDragListeners(cardEl) {
    cardEl.addEventListener('dragstart', function (e) {
        e.dataTransfer.setData('text/plain', cardEl.dataset.cardId);
        e.dataTransfer.effectAllowed = 'move';
        cardEl.classList.add('dragging');
    });
    cardEl.addEventListener('dragend', function () {
        cardEl.classList.remove('dragging');
        document.querySelectorAll('.board-column.drop-target').forEach(function (col) {
            col.classList.remove('drop-target');
        });
    });
}

function initCardDragDrop() {
    document.querySelectorAll('.board-card[draggable="true"]').forEach(attachDragListeners);

    document.querySelectorAll('.board-column').forEach(function (col) {
        col.addEventListener('dragover', function (e) {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            document.querySelectorAll('.board-column.drop-target').forEach(function (el) {
                if (el !== col) el.classList.remove('drop-target');
            });
            col.classList.add('drop-target');
        });

        col.addEventListener('dragleave', function (e) {
            if (!col.contains(e.relatedTarget)) {
                col.classList.remove('drop-target');
            }
        });

        col.addEventListener('drop', function (e) {
            e.preventDefault();
            col.classList.remove('drop-target');
            const cardId = e.dataTransfer.getData('text/plain');
            const columnId = col.dataset.columnId;
            if (!cardId || !columnId) return;
            const isAdmin = document.body.dataset.isAdmin === 'true';
            const isLastColumn = col.dataset.columnType === 'fixed_last';
            if (isAdmin && isLastColumn) {
                convertCardToActionItem(cardId);
            } else {
                moveCard(cardId, columnId);
            }
        });
    });
}

async function moveCard(cardId, columnId) {
    try {
        const resp = await fetch('/cards/' + cardId + '/move', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
            },
            body: JSON.stringify({ column_id: parseInt(columnId, 10) }),
        });

        if (resp.status === 403) {
            showToast('Можна переносити тільки свої картки');
            return;
        }
        if (!resp.ok) {
            showToast('Помилка. Спробуй ще раз.');
            return;
        }

        const cardEl = document.querySelector('.board-card[data-card-id="' + cardId + '"]');
        const targetCards = document.querySelector(
            '.board-column[data-column-id="' + columnId + '"] .board-column-cards'
        );
        if (cardEl && targetCards) {
            targetCards.appendChild(cardEl);
        }
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

// ─── Add card form ────────────────────────────────────────────────────────────

function initAddCardForms() {
    const retroId = document.body.dataset.retroId;
    document.querySelectorAll('.board-add-card-form').forEach(function (form) {
        form.addEventListener('submit', async function (e) {
            e.preventDefault();
            const textarea = form.querySelector('.board-add-card-input');
            const content = textarea.value.trim();
            if (!content) return;

            const columnId = parseInt(form.dataset.columnId, 10);
            try {
                const resp = await fetch('/retros/' + retroId + '/cards', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                    body: JSON.stringify({ column_id: columnId, content: content }),
                });
                if (!resp.ok) {
                    showToast('Помилка. Спробуй ще раз.');
                    return;
                }
                const data = await resp.json();
                textarea.value = '';
                handleCardCreated({
                    id: data.id,
                    content: data.content,
                    author_name: data.author,
                    author_id: parseInt(document.body.dataset.currentUserId, 10),
                    column_id: columnId,
                });
            } catch (_) {
                showToast('Помилка. Спробуй ще раз.');
            }
        });
    });
}

// ─── Context menu ─────────────────────────────────────────────────────────────

function initCardContextMenu() {
    document.addEventListener('click', function (e) {
        if (!e.target.closest('.board-card-menu-btn') && !e.target.closest('.card-menu')) {
            closeAllMenus();
        }
    });

    document.querySelectorAll('.board-card-menu-btn').forEach(attachMenuListener);
}

function attachMenuListener(btn) {
    btn.addEventListener('click', function (e) {
        e.stopPropagation();
        const cardEl = btn.closest('.board-card');
        const cardId = cardEl.dataset.cardId;
        const isActive = document.body.dataset.retroActive === 'true';
        const alreadyOpen = btn.parentNode.querySelector('.card-menu');
        closeAllMenus();
        if (alreadyOpen) return;
        const menu = buildCardMenu(cardId, cardEl, isActive);
        btn.parentNode.appendChild(menu);
    });
}

function buildCardMenu(cardId, cardEl, isActive) {
    const menu = document.createElement('div');
    menu.className = 'card-menu';
    const isAdmin = document.body.dataset.isAdmin === 'true';

    if (isActive) {
        menu.appendChild(menuItem('Редагувати', false, function () {
            closeAllMenus();
            startInlineEdit(cardEl);
        }));
        menu.appendChild(menuItem('Видалити', true, function () {
            closeAllMenus();
            if (confirm('Видалити цю картку?')) {
                deleteCard(cardId, cardEl);
            }
        }));
    } else if (isAdmin) {
        menu.appendChild(menuItem('Скопіювати в наступне ретро', false, function () {
            closeAllMenus();
            copyCard(cardId);
        }));
    }

    return menu;
}

function menuItem(label, danger, onClick) {
    const btn = document.createElement('button');
    btn.className = 'card-menu-item' + (danger ? ' card-menu-item--danger' : '');
    btn.textContent = label;
    btn.addEventListener('click', onClick);
    return btn;
}

function closeAllMenus() {
    document.querySelectorAll('.card-menu').forEach(function (m) { m.remove(); });
}

// ─── Inline edit ──────────────────────────────────────────────────────────────

function startInlineEdit(cardEl) {
    const contentEl = cardEl.querySelector('.board-card-content');
    const cardId = cardEl.dataset.cardId;

    contentEl.hidden = true;

    const form = document.createElement('div');
    form.className = 'board-card-edit';

    const textarea = document.createElement('textarea');
    textarea.className = 'board-card-edit-input';
    textarea.value = contentEl.textContent.trim();
    textarea.rows = 3;
    form.appendChild(textarea);

    const actions = document.createElement('div');
    actions.className = 'board-card-edit-actions';

    const saveBtn = document.createElement('button');
    saveBtn.className = 'btn btn-primary board-card-edit-save';
    saveBtn.textContent = 'Зберегти';
    saveBtn.addEventListener('click', async function () {
        const newContent = textarea.value.trim();
        if (!newContent) return;
        await saveCardEdit(cardId, newContent, contentEl, form);
    });

    const cancelBtn = document.createElement('button');
    cancelBtn.className = 'btn btn-secondary board-card-edit-cancel';
    cancelBtn.textContent = 'Скасувати';
    cancelBtn.addEventListener('click', function () {
        form.remove();
        contentEl.hidden = false;
    });

    actions.appendChild(saveBtn);
    actions.appendChild(cancelBtn);
    form.appendChild(actions);

    contentEl.parentNode.insertBefore(form, contentEl.nextSibling);
    textarea.focus();
    textarea.select();
}

async function saveCardEdit(cardId, content, contentEl, formEl) {
    try {
        const resp = await fetch('/cards/' + cardId, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json',
            },
            body: JSON.stringify({ content: content }),
        });

        if (!resp.ok) {
            showToast('Помилка збереження');
            return;
        }

        contentEl.textContent = content;
        formEl.remove();
        contentEl.hidden = false;
    } catch (_) {
        showToast('Помилка збереження');
    }
}

// ─── Delete ───────────────────────────────────────────────────────────────────

async function deleteCard(cardId, cardEl) {
    try {
        const resp = await fetch('/cards/' + cardId + '/delete', {
            method: 'POST',
            headers: { 'Accept': 'application/json' },
        });

        if (!resp.ok) {
            showToast('Помилка. Спробуй ще раз.');
            return;
        }

        cardEl.remove();
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

// ─── Convert card to action item ─────────────────────────────────────────────

async function convertCardToActionItem(cardId) {
    try {
        const resp = await fetch('/cards/' + cardId + '/to-action-item', {
            method: 'POST',
            headers: { 'Accept': 'application/json' },
        });
        if (!resp.ok) {
            showToast('Помилка. Спробуй ще раз.');
            return;
        }
        const data = await resp.json();
        if (data.ok && data.item) {
            appendActionItem(data.item, data.item.column_id);
            updateColumnCount(data.item.column_id, 1);
        }
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

// ─── Copy to next retro ───────────────────────────────────────────────────────

async function copyCard(cardId) {
    try {
        const resp = await fetch('/cards/' + cardId + '/copy', {
            method: 'POST',
            headers: { 'Accept': 'application/json' },
        });

        const data = await resp.json();

        if (data.error) {
            showToast(data.error);
            return;
        }

        if (data.ok) {
            showToast('Картку скопійовано в наступне ретро', 'success');
        }
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

// ─── Toast ────────────────────────────────────────────────────────────────────

function showToast(message, type) {
    type = type || 'error';
    const toast = document.createElement('div');
    toast.className = 'toast toast--' + type;
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(function () {
        toast.remove();
    }, 3000);
}

// ─── Voting ───────────────────────────────────────────────────────────────────

function initVoting() {
    document.querySelectorAll('.board-card-vote-btn').forEach(function (btn) {
        btn.addEventListener('click', function () {
            voteCard(btn.dataset.cardId, btn);
        });
    });
}

function createVoteBtn(cardId, count, voted) {
    const btn = document.createElement('button');
    btn.className = 'board-card-vote-btn' + (voted ? ' board-card-vote-btn--voted' : '');
    btn.dataset.cardId = cardId;
    const svgNS = 'http://www.w3.org/2000/svg';
    const svg = document.createElementNS(svgNS, 'svg');
    svg.setAttribute('width', '14');
    svg.setAttribute('height', '14');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('fill', 'currentColor');
    svg.setAttribute('aria-hidden', 'true');
    const path = document.createElementNS(svgNS, 'path');
    path.setAttribute('d', 'M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3zM7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3z');
    svg.appendChild(path);
    btn.appendChild(svg);
    btn.appendChild(document.createTextNode(' '));
    const countEl = document.createElement('span');
    countEl.className = 'board-card-vote-count';
    countEl.textContent = count;
    btn.appendChild(countEl);
    btn.addEventListener('click', function () { voteCard(cardId, btn); });
    return btn;
}

async function voteCard(cardId, buttonEl) {
    try {
        const resp = await fetch('/cards/' + cardId + '/vote', {
            method: 'POST',
            headers: { 'Accept': 'application/json' },
        });
        const data = await resp.json();

        if (data.error) {
            showToast('Голоси вичерпано');
            return;
        }

        if (data.ok) {
            const countEl = buttonEl.querySelector('.board-card-vote-count');
            if (countEl) countEl.textContent = data.total_votes;
            buttonEl.classList.toggle('board-card-vote-btn--voted');
            updateVotesBadge(data.votes_remaining);
        }
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

function updateVotesBadge(remaining) {
    const badge = document.getElementById('votes-badge');
    if (!badge) return;
    const limit = document.body.dataset.voteLimit || '?';
    badge.textContent = 'Голоси: ' + remaining + '/' + limit;
}

// ─── WebSocket ────────────────────────────────────────────────────────────────

function initWebSocket(retroID) {
    if (!retroID) return;
    const wsProto = location.protocol === 'https:' ? 'wss://' : 'ws://';
    const ws = new WebSocket(wsProto + location.host + '/retros/' + retroID + '/ws');

    ws.onopen = function () {
        console.log('WS connected retro=' + retroID);
    };

    ws.onmessage = function (event) {
        try {
            const msg = JSON.parse(event.data);
            switch (msg.type) {
                case 'card_created':                handleCardCreated(msg.payload);              break;
                case 'card_updated':                handleCardUpdated(msg.payload);              break;
                case 'card_deleted':                handleCardDeleted(msg.payload);              break;
                case 'card_moved':                  handleCardMoved(msg.payload);                break;
                case 'vote_updated':                handleVoteUpdated(msg.payload);              break;
                case 'action_item_created':         handleActionItemCreated(msg.payload);        break;
                case 'action_item_status_updated':  handleActionItemStatusUpdated(msg.payload);  break;
            }
        } catch (e) {
            console.error('WS parse error:', e);
        }
    };

    ws.onclose = function () {
        setTimeout(function () { initWebSocket(retroID); }, 3000);
    };
}

function updateColumnCount(columnId, delta) {
    const col = document.querySelector('.board-column[data-column-id="' + columnId + '"]');
    if (!col) return;
    const badge = col.querySelector('.board-column-count');
    if (!badge) return;
    badge.textContent = Math.max(0, (parseInt(badge.textContent, 10) || 0) + delta);
}

function handleCardCreated(payload) {
    if (document.querySelector('.board-card[data-card-id="' + payload.id + '"]')) return;

    const currentUserID = parseInt(document.body.dataset.currentUserId, 10);
    const isAuthor = parseInt(payload.author_id, 10) === currentUserID;
    const isAdmin = document.body.dataset.isAdmin === 'true';
    const isActive = document.body.dataset.retroActive === 'true';

    const cardEl = document.createElement('div');
    cardEl.className = 'board-card';
    cardEl.dataset.cardId = payload.id;
    cardEl.dataset.authorId = payload.author_id;
    if (isAuthor || isAdmin) cardEl.setAttribute('draggable', 'true');

    const contentEl = document.createElement('div');
    contentEl.className = 'board-card-content';
    contentEl.textContent = payload.content;

    const footerEl = document.createElement('div');
    footerEl.className = 'board-card-footer';

    const authorEl = document.createElement('span');
    authorEl.className = 'board-card-author';
    authorEl.textContent = payload.author_name;
    footerEl.appendChild(authorEl);

    if (isAuthor && isActive) {
        const menuBtn = document.createElement('button');
        menuBtn.className = 'board-card-menu-btn';
        menuBtn.title = 'Дії з карткою';
        menuBtn.dataset.cardId = payload.id;
        menuBtn.textContent = '···';
        attachMenuListener(menuBtn);
        footerEl.appendChild(menuBtn);
    }

    cardEl.appendChild(contentEl);
    if (isActive) cardEl.appendChild(createVoteBtn(payload.id, 0, false));
    cardEl.appendChild(footerEl);

    if (isAuthor || isAdmin) attachDragListeners(cardEl);

    const columnCards = document.querySelector(
        '.board-column[data-column-id="' + payload.column_id + '"] .board-column-cards'
    );
    if (columnCards) {
        columnCards.appendChild(cardEl);
        updateColumnCount(payload.column_id, 1);
    }
}

function handleCardUpdated(payload) {
    const cardEl = document.querySelector('.board-card[data-card-id="' + payload.id + '"]');
    if (!cardEl) return;
    const contentEl = cardEl.querySelector('.board-card-content');
    if (contentEl) contentEl.textContent = payload.content;
}

function handleCardDeleted(payload) {
    const cardEl = document.querySelector('.board-card[data-card-id="' + payload.id + '"]');
    if (cardEl) {
        const col = cardEl.closest('.board-column');
        if (col) updateColumnCount(col.dataset.columnId, -1);
        cardEl.remove();
    }
}

function handleCardMoved(payload) {
    const cardEl = document.querySelector('.board-card[data-card-id="' + payload.id + '"]');
    const targetCards = document.querySelector(
        '.board-column[data-column-id="' + payload.column_id + '"] .board-column-cards'
    );
    if (cardEl && targetCards) {
        const oldCol = cardEl.closest('.board-column');
        if (oldCol) updateColumnCount(oldCol.dataset.columnId, -1);
        targetCards.appendChild(cardEl);
        updateColumnCount(payload.column_id, 1);
    }
}

function handleVoteUpdated(payload) {
    const cardEl = document.querySelector('.board-card[data-card-id="' + payload.card_id + '"]');
    if (cardEl) {
        const countEl = cardEl.querySelector('.board-card-vote-count');
        if (countEl) countEl.textContent = payload.total_votes;
    }
    const currentUserID = parseInt(document.body.dataset.currentUserId, 10);
    if (parseInt(payload.voter_id, 10) === currentUserID) {
        updateVotesBadge(payload.votes_remaining);
    }
}

initWebSocket(document.body.dataset.retroId);

// ─── Action items ─────────────────────────────────────────────────────────────

function initActionItems() {
    document.querySelectorAll('.board-add-action-item-form').forEach(function (form) {
        form.addEventListener('submit', function (e) {
            e.preventDefault();
            submitActionItem(form);
        });
    });

    document.querySelectorAll('.action-item-status-select').forEach(attachStatusChangeListener);
}

function attachStatusChangeListener(select) {
    select.dataset.prevValue = select.value;
    select.addEventListener('change', function () {
        const prevValue = select.dataset.prevValue;
        updateActionItemStatus(select.dataset.itemId, select.value, select, prevValue);
    });
}

async function submitActionItem(form) {
    const columnId = parseInt(form.dataset.columnId, 10);
    const retroId = form.dataset.retroId;
    const content = form.querySelector('[name="content"]').value.trim();
    const assigneeId = form.querySelector('[name="assignee_id"]').value;
    const deadline = form.querySelector('[name="deadline"]').value;

    if (!content || !assigneeId || !deadline) {
        showToast('Заповніть всі поля');
        return;
    }

    try {
        const resp = await fetch('/retros/' + retroId + '/action-items', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
            body: JSON.stringify({
                content: content,
                assignee_id: parseInt(assigneeId, 10),
                deadline: deadline,
                column_id: columnId,
            }),
        });

        if (!resp.ok) {
            const text = await resp.text();
            showToast(text || 'Помилка. Спробуй ще раз.');
            return;
        }

        const data = await resp.json();
        form.reset();
        appendActionItem(data, columnId);
    } catch (_) {
        showToast('Помилка. Спробуй ще раз.');
    }
}

async function updateActionItemStatus(itemId, statusId, selectEl, prevValue) {
    try {
        const resp = await fetch('/action-items/' + itemId + '/status', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
            body: JSON.stringify({ status_id: parseInt(statusId, 10) }),
        });
        if (!resp.ok) {
            if (selectEl && prevValue !== undefined) selectEl.value = prevValue;
            showToast('Помилка оновлення статусу');
            return;
        }
        if (selectEl) selectEl.dataset.prevValue = statusId;
        showToast('Статус оновлено', 'success');
    } catch (_) {
        if (selectEl && prevValue !== undefined) selectEl.value = prevValue;
        showToast('Помилка. Спробуй ще раз.');
    }
}

function buildActionItemEl(item) {
    const el = document.createElement('div');
    el.className = 'action-item';
    el.dataset.itemId = item.id;

    const contentEl = document.createElement('div');
    contentEl.className = 'action-item-content';
    contentEl.textContent = item.content;

    const metaEl = document.createElement('div');
    metaEl.className = 'action-item-meta';

    const assigneeEl = document.createElement('span');
    assigneeEl.className = 'action-item-assignee';
    const assigneeName = ((item.assignee_first_name || '') + ' ' + (item.assignee_last_name || '')).trim();
    assigneeEl.textContent = '👤 ' + (assigneeName || '—');

    const deadlineEl = document.createElement('span');
    deadlineEl.className = 'action-item-deadline';
    if (item.deadline) {
        const parts = item.deadline.split('-');
        deadlineEl.textContent = '📅 ' + (parts.length === 3 ? parts[2] + '.' + parts[1] + '.' + parts[0] : item.deadline);
    } else {
        deadlineEl.textContent = '📅 —';
    }

    metaEl.appendChild(assigneeEl);
    metaEl.appendChild(deadlineEl);

    const statusEl = document.createElement('div');
    statusEl.className = 'action-item-status';

    const select = document.createElement('select');
    select.className = 'action-item-status-select';
    select.dataset.itemId = item.id;
    (window.boardStatuses || []).forEach(function (s) {
        const opt = document.createElement('option');
        opt.value = s.ID;
        opt.textContent = s.Name;
        if (s.ID === item.status_id) opt.selected = true;
        select.appendChild(opt);
    });
    attachStatusChangeListener(select);
    statusEl.appendChild(select);

    el.appendChild(contentEl);
    el.appendChild(metaEl);
    el.appendChild(statusEl);
    return el;
}

function appendActionItem(item, columnId) {
    if (document.querySelector('.action-item[data-item-id="' + item.id + '"]')) return;
    const colCards = document.querySelector('.board-column[data-column-id="' + columnId + '"] .board-column-cards');
    if (!colCards) return;
    colCards.appendChild(buildActionItemEl(item));
}

function handleActionItemCreated(payload) {
    appendActionItem(payload, payload.column_id);
}

function handleActionItemStatusUpdated(payload) {
    const select = document.querySelector('.action-item-status-select[data-item-id="' + payload.id + '"]');
    if (select) select.value = payload.status_id;
}
