/* PageTree Admin UI — vanilla JS, no frameworks */
(function () {
  'use strict';

  const API = '/api/admin/content';
  let vditor = null;
  let currentPath = null;
  let currentRoot = 'pages';
  let mediaFiles = [];

  // --- Status ---
  function showStatus(msg, ok) {
    const el = document.getElementById('status');
    el.textContent = msg;
    el.className = 'status ' + (ok ? 'ok' : 'err');
    setTimeout(() => { el.className = 'status'; }, 3000);
  }

  // --- API helpers ---
  async function api(method, path, body) {
    const opts = { method, headers: {} };
    if (body) {
      if (body instanceof FormData) {
        opts.body = body;
      } else {
        opts.headers['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(body);
      }
    }
    // Attach JWT from localStorage (mcp-is convention)
    const token = localStorage.getItem('token');
    if (token) opts.headers['Authorization'] = 'Bearer ' + token;

    const resp = await fetch(API + path, opts);
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.json();
  }

  // --- Vditor init ---
  function initEditor() {
    vditor = new Vditor('vditor', {
      height: '100%',
      mode: 'ir',
      lang: 'ru_RU',
      cdn: '/content/admin/vditor',
      toolbar: [
        'emoji', 'headings', 'bold', 'italic', 'strike', '|',
        'line', 'quote', 'list', 'ordered-list', 'check', '|',
        'code', 'inline-code', 'table', '|',
        'link', 'upload', '|',
        'undo', 'redo', '|',
        'edit-mode', 'preview', 'fullscreen'
      ],
      upload: {
        url: API + '/upload',
        accept: 'image/*',
        fieldName: 'file',
        extraData: { dir: 'media' },
        // Vditor шлёт свой XHR без общего api() — JWT нужно передавать явно
        headers: { Authorization: 'Bearer ' + (localStorage.getItem('token') || '') },
        format(files, response) {
          // Vditor требует форму {data:{errFiles,succMap}} — иначе Object.keys(undefined)
          const r = JSON.parse(response);
          const name = files[0] ? files[0].name : 'file';
          return JSON.stringify({
            code: 0,
            msg: '',
            data: { errFiles: [], succMap: { [name]: r.url } },
          });
        },
        error(msg) {
          showStatus('Ошибка загрузки: ' + msg, false);
        }
      },
      preview: { markdown: { sanitize: true } },
      cache: { enable: false },
      input(val) { markDirty(); },
    });
    // Paste sanitization via DOMPurify (belt-and-suspenders beyond lute sanitize)
    setTimeout(() => {
      const el = document.querySelector('.vditor-content [contenteditable], .vditor-wysiwyg [contenteditable], .vditor-ir [contenteditable]');
      if (el && window.DOMPurify) {
        el.addEventListener('paste', (e) => {
          const html = e.clipboardData.getData('text/html');
          if (html) {
            e.preventDefault();
            const clean = window.DOMPurify.sanitize(html, { ALLOWED_TAGS: ['b','i','em','strong','a','ul','ol','li','p','br','h1','h2','h3','h4','h5','h6','code','pre','blockquote','table','thead','tbody','tr','th','td','hr','img'], ALLOWED_ATTR: ['href','src','alt','title','class'] });
            document.execCommand('insertHTML', false, clean);
          }
        }, true);
      }
    }, 500);
  }

  // --- File tree ---
  async function loadTree() {
    try {
      const tree = await api('GET', '/list?root=' + currentRoot);
      renderTree(tree || []);
    } catch (e) {
      showStatus('Ошибка загрузки дерева: ' + e.message, false);
    }
  }

  function renderTree(entries) {
    const el = document.getElementById('file-tree');
    el.innerHTML = '';
    entries.forEach(entry => {
      el.appendChild(renderTreeItem(entry, 0));
    });
  }

  function renderTreeItem(entry, depth) {
    const div = document.createElement('div');
    div.className = 'tree-item' + (entry.path === currentPath ? ' active' : '');
    div.style.paddingLeft = (0.8 + depth * 1) + 'rem';

    const typeSpan = document.createElement('span');
    typeSpan.className = 'type ' + entry.type;
    typeSpan.textContent = entry.type === 'orphan' ? '!' : entry.type === 'dir' ? '📁' : '📄';
    div.appendChild(typeSpan);

    const titleSpan = document.createElement('span');
    titleSpan.textContent = entry.title || entry.path;
    if (entry.is_draft) titleSpan.textContent += ' (черновик)';
    div.appendChild(titleSpan);

    // Actions on hover
    const actions = document.createElement('span');
    actions.className = 'actions';
    if (entry.type !== 'orphan') {
      const renameBtn = document.createElement('button');
      renameBtn.textContent = '✏';
      renameBtn.title = 'Переименовать';
      renameBtn.onclick = (e) => { e.stopPropagation(); renameEntry(entry.path); };
      actions.appendChild(renameBtn);
    }
    const delBtn = document.createElement('button');
    delBtn.textContent = '🗑';
    delBtn.title = 'Удалить';
    delBtn.onclick = (e) => { e.stopPropagation(); deleteEntry(entry.path, entry.type); };
    actions.appendChild(delBtn);
    div.appendChild(actions);

    if (entry.type !== 'orphan' && entry.type !== 'dir') {
      div.onclick = () => openPage(entry.path);
    }

    // Children (for dirs)
    const frag = document.createDocumentFragment();
    frag.appendChild(div);
    if (entry.children && entry.children.length > 0) {
      entry.children.forEach(child => {
        frag.appendChild(renderTreeItem(child, depth + 1));
      });
    }
    return frag;
  }

  // --- Page operations ---
  async function openPage(path) {
    try {
      const data = await api('GET', '/file?path=' + encodeURIComponent(path));
      currentPath = path;
      currentRoot = path.startsWith('news/') ? 'news' : 'pages';
      vditor.setValue(data.content);
      document.getElementById('file-label').textContent = path;
      // Load sidecar
      const jsonPath = path.replace(/\.md$/, '.json');
      try {
        const meta = await api('GET', '/file?path=' + encodeURIComponent(jsonPath));
        setMeta(JSON.parse(meta.content));
      } catch { setMeta({}); }
      loadSectionForm(currentPath.replace(/\/[^/]+\.md$/, ''));
      loadTree();
    } catch (e) {
      showStatus('Ошибка открытия: ' + e.message, false);
    }
  }

  function setMeta(meta) {
    document.getElementById('meta-title').value = meta.title || '';
    document.getElementById('meta-menu').checked = !!meta.menu;
    document.getElementById('meta-order').value = meta.order || 0;
    document.getElementById('meta-date').value = meta.date || '';
    document.getElementById('meta-modified').value = meta.modified || '';
    document.getElementById('meta-draft').checked = !!meta.draft;
    const seo = meta.seo || {};
    document.getElementById('seo-title').value = seo.meta_title || '';
    document.getElementById('seo-desc').value = seo.meta_description || '';
    document.getElementById('seo-robots').value = seo.robots || 'index, follow';
    document.getElementById('seo-og-image').value = seo.og_image || '';
    document.getElementById('seo-canonical').value = seo.canonical || '';
    document.getElementById('seo-breadcrumb').value = seo.breadcrumb_title || '';
    updateSeoPreview();
    updateCounts();
  }

  function getMeta() {
    return {
      title: document.getElementById('meta-title').value,
      menu: document.getElementById('meta-menu').checked,
      order: parseInt(document.getElementById('meta-order').value) || 0,
      date: document.getElementById('meta-date').value,
      modified: document.getElementById('meta-modified').value,
      draft: document.getElementById('meta-draft').checked,
      seo: {
        meta_title: document.getElementById('seo-title').value,
        meta_description: document.getElementById('seo-desc').value,
        robots: document.getElementById('seo-robots').value,
        og_image: document.getElementById('seo-og-image').value,
        canonical: document.getElementById('seo-canonical').value,
        breadcrumb_title: document.getElementById('seo-breadcrumb').value,
      }
    };
  }

  async function savePage() {
    if (!currentPath) {
      showStatus('Сначала откройте или создайте страницу', false);
      return;
    }
    try {
      const md = vditor.getValue();
      const meta = getMeta();
      await api('PUT', '/page', { path: currentPath, md: md, json: meta });
      showStatus('Сохранено: ' + currentPath, true);
      loadTree();
    } catch (e) {
      showStatus('Ошибка сохранения: ' + e.message, false);
    }
  }

  async function newPage(root) {
    const name = prompt('Имя файла (без .md, например "about"):');
    if (!name) return;
    const dir = root === 'news' ? 'news' : 'pages';
    const path = dir + '/' + name.replace(/\.md$/, '') + '.md';
    try {
      await api('PUT', '/page', {
        path: path,
        md: '# ' + name + '\n\n',
        json: { title: name, menu: false, order: 0, draft: true }
      });
      currentPath = path;
      currentRoot = root;
      openPage(path);
      showStatus('Создано: ' + path, true);
    } catch (e) {
      showStatus('Ошибка создания: ' + e.message, false);
    }
  }

  async function deletePage() {
    if (!currentPath) return;
    if (!confirm('Удалить ' + currentPath + ' (включая .json и _media/)?')) return;
    try {
      await api('DELETE', '/page?path=' + encodeURIComponent(currentPath));
      currentPath = null;
      vditor.setValue('');
      document.getElementById('file-label').textContent = '';
      showStatus('Удалено', true);
      loadTree();
    } catch (e) {
      showStatus('Ошибка удаления: ' + e.message, false);
    }
  }

  async function renamePage() {
    if (!currentPath) return;
    const newPath = prompt('Новый путь:', currentPath);
    if (!newPath || newPath === currentPath) return;
    try {
      await api('POST', '/rename', { old_path: currentPath, new_path: newPath });
      currentPath = newPath;
      document.getElementById('file-label').textContent = newPath;
      showStatus('Переименовано', true);
      loadTree();
    } catch (e) {
      showStatus('Ошибка переименования: ' + e.message, false);
    }
  }

  async function renameEntry(path) {
    const newPath = prompt('Новый путь:', path);
    if (!newPath || newPath === path) return;
    try {
      await api('POST', '/rename', { old_path: path, new_path: newPath });
      showStatus('Переименовано', true);
      loadTree();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  async function deleteEntry(path, type) {
    if (type === 'orphan') {
      if (!confirm('Удалить сироту ' + path + '?')) return;
      try {
        await api('DELETE', '/media?path=' + encodeURIComponent(path));
        showStatus('Удалено', true);
        loadTree();
      } catch (e) {
        showStatus('Ошибка: ' + e.message, false);
      }
      return;
    }
    if (!confirm('Удалить ' + path + '?')) return;
    try {
      await api('DELETE', '/page?path=' + encodeURIComponent(path));
      showStatus('Удалено', true);
      loadTree();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  // --- Section ---
  async function saveSection() {
    // Save section.json for current directory
    if (!currentPath) {
      showStatus('Сначала откройте страницу', false);
      return;
    }
    const dir = currentPath.replace(/\/[^/]+\.md$/, '');
    const sec = {
      title: document.getElementById('sec-title').value,
      order: parseInt(document.getElementById('sec-order').value) || 0,
      menu: document.getElementById('sec-menu').checked,
    };
    try {
      await api('PUT', '/section', { path: dir, json: sec });
      showStatus('Секция сохранена: ' + dir, true);
    } catch (e) {
      showStatus('Ошибка секции: ' + e.message, false);
    }
  }

  async function loadSectionForm(dir) {
    try {
      const data = await api('GET', '/file?path=' + encodeURIComponent(dir + '/section.json'));
      const sec = JSON.parse(data.content);
      document.getElementById('sec-title').value = sec.title || '';
      document.getElementById('sec-order').value = sec.order || 0;
      document.getElementById('sec-menu').checked = !!sec.menu;
    } catch {
      document.getElementById('sec-title').value = '';
      document.getElementById('sec-order').value = 0;
      document.getElementById('sec-menu').checked = false;
    }
  }

  // --- Media ---
  function openMedia() {
    document.getElementById('media-modal').classList.add('show');
    loadMedia();
  }

  function closeMedia() {
    document.getElementById('media-modal').classList.remove('show');
  }

  async function loadMedia() {
    const dir = document.getElementById('media-dir').value;
    try {
      mediaFiles = await api('GET', '/media?path=' + encodeURIComponent(dir === '_page' ? getPageMediaDir() : 'media'));
      renderMediaGrid();
    } catch (e) {
      showStatus('Ошибка загрузки картинок: ' + e.message, false);
    }
  }

  function getPageMediaDir() {
    if (!currentPath) return 'media';
    return currentPath.replace(/\.md$/, '_media');
  }

  function renderMediaGrid() {
    const grid = document.getElementById('media-grid');
    const filter = document.getElementById('media-filter').value.toLowerCase();
    grid.innerHTML = '';
    (mediaFiles || [])
      .filter(f => !filter || f.name.toLowerCase().includes(filter))
      .forEach(f => {
        const item = document.createElement('div');
        item.className = 'media-item';
        item.innerHTML = `<img src="${f.url}" alt="${f.name}" loading="lazy">
          <div class="name">${f.name}</div>
          <button class="del" onclick="app.deleteMedia('${f.url}')">✕</button>`;
        item.onclick = (e) => {
          if (e.target.classList.contains('del')) return;
          insertImageToEditor(f.url);
        };
        grid.appendChild(item);
      });
  }

  function filterMedia() {
    renderMediaGrid();
  }

  async function uploadMedia(input) {
    const file = input.files[0];
    if (!file) return;
    const dir = document.getElementById('media-dir').value;
    const targetDir = dir === '_page' ? getPageMediaDir() : 'media';
    const formData = new FormData();
    formData.append('file', file);
    formData.append('dir', targetDir);
    try {
      const resp = await api('POST', '/upload', formData);
      showStatus('Загружено: ' + resp.url, true);
      loadMedia();
    } catch (e) {
      showStatus('Ошибка загрузки: ' + e.message, false);
    }
    input.value = '';
  }

  async function deleteMedia(url) {
    if (!confirm('Удалить ' + url + '?')) return;
    try {
      await api('DELETE', '/media?path=' + encodeURIComponent(url.replace(/^\//, '')));
      showStatus('Удалено', true);
      loadMedia();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  function insertImageToEditor(url) {
    const alt = prompt('Alt-текст:', '') || '';
    const img = `<img src="${url}" alt="${alt}">`;
    vditor.insertValue(img);
    closeMedia();
  }

  // --- Image properties ---
  function openImageDialog() {
    document.getElementById('img-modal').classList.add('show');
  }

  function closeImageDialog() {
    document.getElementById('img-modal').classList.remove('show');
  }

  function applyImageProps() {
    const src = document.getElementById('img-src').value;
    const width = document.getElementById('img-width').value;
    const height = document.getElementById('img-height').value;
    const align = document.getElementById('img-align').value;
    const alt = document.getElementById('img-alt').value;

    if (!src) {
      showStatus('URL картинки обязателен', false);
      return;
    }

    let img = `<img src="${src}" alt="${alt}"`;
    if (width) img += ` width="${width}"`;
    if (height) img += ` height="${height}"`;
    if (align) img += ` align="${align}"`;
    img += '>';

    vditor.insertValue(img);
    closeImageDialog();
    showStatus('Картинка вставлена', true);
  }

  // --- SEO preview ---
  function updateSeoPreview() {
    const title = document.getElementById('seo-title').value || document.getElementById('meta-title').value || 'Без заголовка';
    const desc = document.getElementById('seo-desc').value || 'Без описания';
    const url = document.getElementById('seo-canonical').value || (location.origin + (currentPath ? '/' + currentPath.replace(/\.md$/, '') : '/'));
    const preview = document.getElementById('seo-preview');
    preview.innerHTML = `<div class="title">${esc(title)}</div>
      <div class="url">${esc(url)}</div>
      <div class="desc">${esc(desc.substring(0, 160))}</div>`;
  }

  function updateCounts() {
    const titleLen = document.getElementById('seo-title').value.length;
    const descLen = document.getElementById('seo-desc').value.length;
    document.getElementById('seo-title-count').textContent = titleLen + ' / 60';
    document.getElementById('seo-desc-count').textContent = descLen + ' / 160';
  }

  function esc(s) {
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  // --- Preview ---
  function preview() {
    if (!currentPath) return;
    const url = currentPath.replace(/\.md$/, '').replace(/^pages\//, '/pages/').replace(/^news\//, '/news/');
    window.open(url, '_blank');
  }

  // Ник для шапки: localStorage → фолбэк из JWT-клейма (сессия может жить без ключа nickname)
  function displayNickname() {
    const fromLS = localStorage.getItem('nickname');
    if (fromLS) return fromLS;
    try {
      const raw = (localStorage.getItem('token') || '').split('.')[1] || '';
      const b64 = raw.replace(/-/g, '+').replace(/_/g, '/');
      const claims = JSON.parse(atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4)));
      return claims.nickname || '';
    } catch (e) {
      return '';
    }
  }

  // --- Session / export-import ---
  function logout() {
    // Конвенция mcp-is API.clearSession(): все ключи сессии + следы имперсонации
    ['token', 'nickname', 'role', 'user_id', 'impersonating', 'admin_token', 'admin_user_id', 'admin_nickname', 'admin_role']
      .forEach((k) => localStorage.removeItem(k));
    sessionStorage.removeItem('return_to');
    sessionStorage.removeItem('autologin_retry');
    window.location.href = '/login';
  }

  function downloadMD() {
    if (!currentPath) {
      showStatus('Нет открытой страницы', false);
      return;
    }
    const md = vditor.getValue();
    const blob = new Blob([md], { type: 'text/markdown' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = currentPath.split('/').pop();
    a.click();
    URL.revokeObjectURL(a.href);
  }

  function uploadMD() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.md';
    input.onchange = () => {
      const file = input.files[0];
      if (!file) return;
      file.text().then((text) => {
        vditor.setValue(text);
        markDirty();
        showStatus('Загружено: ' + file.name + ' — сохраните страницу', true);
      });
    };
    input.click();
  }

  // --- Dirty tracking ---
  let dirty = false;
  function markDirty() {
    dirty = true;
    document.title = '● PageTree — Редактор страниц';
  }

  window.addEventListener('beforeunload', (e) => {
    if (dirty) {
      e.preventDefault();
      e.returnValue = 'Есть несохранённые изменения. Уйти?';
    }
  });

  // --- Init ---
  function init() {
    initEditor();
    loadTree();
    document.getElementById('user-nickname').textContent = displayNickname();
    // SEO input listeners
    ['seo-title', 'seo-desc'].forEach(id => {
      document.getElementById(id).addEventListener('input', () => {
        updateSeoPreview();
        updateCounts();
      });
    });
    ['meta-title'].forEach(id => {
      document.getElementById(id).addEventListener('input', updateSeoPreview);
    });
  }

  // Expose for HTML onclick
  window.app = {
    savePage, newPage, deletePage, renamePage, renameEntry, deleteEntry,
    saveSection, openMedia, closeMedia, uploadMedia, deleteMedia, filterMedia,
    openImageDialog, closeImageDialog, applyImageProps, preview,
    logout, downloadMD, uploadMD,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
