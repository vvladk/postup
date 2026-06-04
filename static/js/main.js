// Nav user dropdown
(function () {
    const btn = document.getElementById('nav-user-btn');
    const dropdown = document.getElementById('nav-dropdown');
    if (!btn || !dropdown) return;
    btn.addEventListener('click', function (e) {
        e.stopPropagation();
        dropdown.classList.toggle('nav-dropdown--open');
    });
    document.addEventListener('click', function () {
        dropdown.classList.remove('nav-dropdown--open');
    });
})();

// Theme radio: apply instantly without page reload
(function () {
    const themeRadios = document.querySelectorAll('input[name="theme"]');
    if (!themeRadios.length) return;
    themeRadios.forEach(function (radio) {
        radio.addEventListener('change', function (e) {
            document.documentElement.dataset.theme = e.target.value;
            const langEl = document.querySelector('input[name="lang"]:checked');
            const lang = langEl ? langEl.value : 'uk';
            fetch('/settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
                body: 'section=personal&lang=' + encodeURIComponent(lang) + '&theme=' + encodeURIComponent(e.target.value)
            });
        });
    });
})();

function initColumnDragDrop(containerSelector) {
    const editor = document.querySelector(containerSelector);
    if (!editor) return;
    const container = editor.querySelector('#regular-columns');
    if (!container) return;

    let dragEl = null;
    let placeholder = null;

    function createPlaceholder() {
        const el = document.createElement('div');
        el.className = 'column-drop-placeholder';
        return el;
    }

    function getDragAfterElement(clientX) {
        const els = [...container.querySelectorAll('.column-regular:not(.dragging)')];
        return els.reduce((closest, child) => {
            const box = child.getBoundingClientRect();
            const offset = clientX - box.left - box.width / 2;
            if (offset < 0 && offset > closest.offset) {
                return { offset, element: child };
            }
            return closest;
        }, { offset: Number.NEGATIVE_INFINITY }).element;
    }

    function syncPositions() {
        container.querySelectorAll('.column-regular').forEach((col, i) => {
            col.dataset.position = i + 1;
            const input = col.querySelector('input[name="positions[]"]');
            if (input) input.value = i + 1;
        });
    }

    container.addEventListener('dragstart', (e) => {
        const col = e.target.closest('.column-regular');
        if (!col) return;
        dragEl = col;
        e.dataTransfer.effectAllowed = 'move';
        setTimeout(() => col.classList.add('dragging'), 0);
    });

    container.addEventListener('dragend', () => {
        if (!dragEl) return;
        dragEl.classList.remove('dragging');
        if (placeholder) { placeholder.remove(); placeholder = null; }
        dragEl = null;
        syncPositions();
    });

    container.addEventListener('dragover', (e) => {
        if (!dragEl) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        if (!placeholder) placeholder = createPlaceholder();
        const after = getDragAfterElement(e.clientX);
        after ? container.insertBefore(placeholder, after) : container.appendChild(placeholder);
    });

    container.addEventListener('drop', (e) => {
        e.preventDefault();
        if (!dragEl || !placeholder || !placeholder.parentNode) return;
        placeholder.parentNode.insertBefore(dragEl, placeholder);
    });

    syncPositions();
}

function copyText(inputId) {
    const input = document.getElementById(inputId);
    if (!input) return;
    navigator.clipboard.writeText(input.value).then(() => {
        const btn = event.currentTarget;
        const original = btn.textContent;
        btn.textContent = 'Скопійовано!';
        setTimeout(() => { btn.textContent = original; }, 2000);
    }).catch(() => {
        input.select();
        document.execCommand('copy');
    });
}

function copyInviteLink() { copyText('invite-link'); }
function copyResetLink()  { copyText('reset-link'); }

function copyToClipboard(text, buttonEl) {
    const original = buttonEl.textContent;
    function showDone() {
        buttonEl.textContent = 'Скопійовано ✓';
        setTimeout(() => { buttonEl.textContent = original; }, 2000);
    }
    if (navigator.clipboard) {
        navigator.clipboard.writeText(text).then(showDone).catch(() => {
            fallbackCopy(text);
            showDone();
        });
    } else {
        fallbackCopy(text);
        showDone();
    }
}

function fallbackCopy(text) {
    const el = document.createElement('textarea');
    el.value = text;
    el.style.cssText = 'position:fixed;opacity:0';
    document.body.appendChild(el);
    el.focus();
    el.select();
    document.execCommand('copy');
    document.body.removeChild(el);
}
