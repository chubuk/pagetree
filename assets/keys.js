/* PageTree Keys & Access — vanilla JS, no frameworks. XSS: user data → textContent/esc() only. */
(function () {
  'use strict';

  const API = '/api/admin/content';
  const PLACEHOLDER_KEY = '<ваш-ptk-ключ>';
  let clientTypes = [];
  let editKeyId = null;
  // Режим генератора: 'created' (raw известен) | 'example' (плейсхолдер)
  let cfg = { name: '', apiKey: PLACEHOLDER_KEY };

  function showStatus(msg, ok) {
    const el = document.getElementById('status');
    el.textContent = msg;
    el.className = 'status ' + (ok ? 'ok' : 'err');
    setTimeout(() => { el.className = 'status'; }, 3000);
  }

  function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  async function api(method, path, body) {
    const opts = { method, headers: {} };
    if (body) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    const token = localStorage.getItem('token');
    if (token) opts.headers['Authorization'] = 'Bearer ' + token;
    const resp = await fetch(API + path, opts);
    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(err.error || resp.statusText);
    }
    return resp.json();
  }

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

  function logout() {
    ['token', 'nickname', 'role', 'user_id', 'impersonating', 'admin_token', 'admin_user_id', 'admin_nickname', 'admin_role']
      .forEach((k) => localStorage.removeItem(k));
    sessionStorage.removeItem('return_to');
    sessionStorage.removeItem('autologin_retry');
    window.location.href = '/login';
  }

  // slugify — 1:1 с mcp-is aikeys.js (транслитерация + fallback), fallback = 'pagetree'
  function slugify(str, fallback = 'pagetree') {
    if (!str || !str.trim()) return fallback;
    const cyr = {'а':'a','б':'b','в':'v','г':'g','д':'d','е':'e','ё':'e','ж':'zh','з':'z','и':'i','й':'y','к':'k','л':'l','м':'m','н':'n','о':'o','п':'p','р':'r','с':'s','т':'t','у':'u','ф':'f','х':'kh','ц':'ts','ч':'ch','ш':'sh','щ':'shch','ъ':'','ы':'y','ь':'','э':'e','ю':'yu','я':'ya'};
    let s = str.toLowerCase();
    let result = '';
    for (const ch of s) { result += cyr[ch] !== undefined ? cyr[ch] : ch; }
    result = result.replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
    return result || fallback;
  }

  function fillTemplate(template, alias, url, apiKey) {
    return template
      .replace(/\{\{KEY_NAME\}\}/g, alias)
      .replace(/\{\{URL\}\}/g, url)
      .replace(/\{\{API_KEY\}\}/g, apiKey);
  }

  function downloadText(fileName, text) {
    const blob = new Blob([text], { type: 'text/plain' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = fileName;
    a.click();
    URL.revokeObjectURL(a.href);
  }

  function mcpURL() { return location.origin + '/api/agent/mcp'; }
  function restURL() { return location.origin + '/api/agent/v1/'; }

  function envText() {
    return 'PAGETREE_API_URL=' + restURL() + '\n' +
      'PAGETREE_MCP_URL=' + mcpURL() + '\n' +
      'PAGETREE_API_KEY=' + cfg.apiKey + '\n' +
      'PAGETREE_KEY_NAME=' + slugify(cfg.name) + '\n';
  }

  // --- Endpoints block ---
  function initEndpoints() {
    document.getElementById('ep-rest').value = restURL();
    document.getElementById('ep-mcp').value = mcpURL();
  }

  // --- Config generator ---
  async function loadClientTypes() {
    clientTypes = await api('GET', '/keys/client-types');
    const sel = document.getElementById('client-type');
    clientTypes.forEach((t, i) => {
      const opt = document.createElement('option');
      opt.value = String(i);
      opt.textContent = t.name;
      sel.appendChild(opt);
    });
    const saved = localStorage.getItem('mcp_client_type');
    if (saved != null) sel.value = saved;
  }

  function selectedType() {
    const v = document.getElementById('client-type').value;
    if (v === '') return null;
    return clientTypes[Number(v)] || null;
  }

  function renderConfig() {
    const t = selectedType();
    const pre = document.getElementById('config-pre');
    const code = document.getElementById('config-code');
    const hint = document.getElementById('config-file-hint');
    const dl = document.getElementById('config-download');
    if (!t) {
      pre.style.display = 'none';
      hint.textContent = '';
      dl.style.display = 'none';
      return;
    }
    localStorage.setItem('mcp_client_type', document.getElementById('client-type').value);
    code.textContent = fillTemplate(t.config_template, slugify(cfg.name), mcpURL(), cfg.apiKey);
    pre.style.display = '';
    hint.textContent = t.file_name
      ? 'Файл: ' + t.file_name + ' → ' + t.file_description
      : t.file_description;
    dl.style.display = t.file_name ? '' : 'none';
  }

  function copyConfig() {
    const code = document.getElementById('config-code');
    if (!code.textContent) { showStatus('Сначала выберите агента', false); return; }
    copyText(code.textContent);
  }

  function downloadConfig() {
    const t = selectedType();
    if (!t || !t.file_name) return;
    downloadText(t.file_name, document.getElementById('config-code').textContent);
  }

  function downloadEnv() {
    downloadText('.env', envText());
  }

  function copyText(text) {
    navigator.clipboard.writeText(text).then(
      () => showStatus('Скопировано', true),
      () => showStatus('Не удалось скопировать — выделите вручную', false)
    );
  }

  function copyField(id) {
    const el = document.getElementById(id);
    copyText(el.value !== undefined ? el.value : el.textContent);
  }

  // --- Keys list / CRUD ---
  function keyPolicyForm() {
    return {
      pages: document.getElementById('key-pages').value,
      news: document.getElementById('key-news').value,
      media: document.getElementById('key-media').value,
      paths: document.getElementById('key-paths').value.split(',').map(s => s.trim()).filter(Boolean),
    };
  }

  async function loadKeys() {
    const el = document.getElementById('keys-list');
    try {
      const keys = await api('GET', '/keys/');
      if (!keys.length) {
        el.textContent = 'Ключей пока нет';
        return;
      }
      el.innerHTML = keys.map(k => {
        const st = k.revoked_at ? '<span style="color:#c00">отозван</span>' : '<span style="color:#080">активен</span>';
        const last = k.last_used_at ? new Date(k.last_used_at).toLocaleString('ru') : '—';
        const paths = (k.policy.paths || []).join(', ') || 'все пути';
        return `<div class="key-item">
          <b>${esc(k.name)}</b> <code>${esc(k.prefix)}…</code>
          <span class="meta">стр:${esc(k.policy.pages)} нов:${esc(k.policy.news)} мед:${esc(k.policy.media)} · ${esc(paths)} · ${st} · был: ${esc(last)}</span>
          <span class="actions">
            <button data-act="examples" data-id="${esc(k.id)}" data-name="${esc(k.name)}">Примеры</button>
            <button data-act="edit" data-id="${esc(k.id)}">Изменить</button>
            ${k.revoked_at ? '' : `<button data-act="revoke" data-id="${esc(k.id)}">Отозвать</button>`}
            <button data-act="delete" data-id="${esc(k.id)}">Удалить</button>
          </span>
        </div>`;
      }).join('');
    } catch (e) {
      el.textContent = '';
      showStatus('Ошибка загрузки ключей: ' + e.message, false);
    }
  }

  async function createKey() {
    const name = document.getElementById('key-name').value.trim();
    if (!name) { showStatus('Укажите имя ключа', false); return; }
    try {
      if (editKeyId) {
        await api('PUT', '/keys/' + editKeyId, { name, policy: keyPolicyForm() });
        showStatus('Ключ «' + name + '» обновлён', true);
        editKeyId = null;
        document.getElementById('key-submit').textContent = 'Создать';
      } else {
        const res = await api('POST', '/keys/', { name, policy: keyPolicyForm() });
        document.getElementById('key-raw').value = res.raw_key;
        document.getElementById('key-created').style.display = '';
        cfg = { name, apiKey: res.raw_key };
        renderConfig();
        showStatus('Ключ создан — сохраните его', true);
      }
      await loadKeys();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  async function editKey(id) {
    try {
      const k = await api('GET', '/keys/' + id);
      editKeyId = id;
      document.getElementById('key-name').value = k.name;
      document.getElementById('key-pages').value = k.policy.pages;
      document.getElementById('key-news').value = k.policy.news;
      document.getElementById('key-media').value = k.policy.media;
      document.getElementById('key-paths').value = (k.policy.paths || []).join(', ');
      document.getElementById('key-created').style.display = 'none';
      document.getElementById('key-submit').textContent = 'Сохранить';
      showStatus('Редактирование ключа «' + k.name + '»', true);
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  async function revokeKey(id) {
    if (!confirm('Отозвать ключ? Выдачи по нему перестанут работать.')) return;
    try {
      await api('POST', '/keys/' + id + '/revoke');
      showStatus('Ключ отозван', true);
      await loadKeys();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  async function deleteKey(id) {
    if (!confirm('Удалить ключ безвозвратно (запись исчезнет из журнала)?')) return;
    try {
      await api('DELETE', '/keys/' + id);
      showStatus('Ключ удалён', true);
      await loadKeys();
    } catch (e) {
      showStatus('Ошибка: ' + e.message, false);
    }
  }

  // --- Init ---
  async function init() {
    document.getElementById('user-nickname').textContent = displayNickname();
    initEndpoints();
    try {
      await loadClientTypes();
    } catch (e) {
      showStatus('Ошибка загрузки списка агентов: ' + e.message, false);
    }
    await loadKeys();
    renderConfig();
    document.getElementById('keys-list').addEventListener('click', (e) => {
      const btn = e.target.closest('button[data-act]');
      if (!btn) return;
      const id = btn.dataset.id;
      switch (btn.dataset.act) {
        case 'examples':
          cfg = { name: btn.dataset.name || '', apiKey: PLACEHOLDER_KEY };
          document.getElementById('key-created').style.display = 'none';
          renderConfig();
          showStatus('Примеры для ключа «' + cfg.name + '» — вставьте свой ключ вместо ' + PLACEHOLDER_KEY, true);
          document.getElementById('config-card').scrollIntoView({ behavior: 'smooth' });
          break;
        case 'edit': editKey(id); break;
        case 'revoke': revokeKey(id); break;
        case 'delete': deleteKey(id); break;
      }
    });
  }

  window.app = {
    createKey, editKey, revokeKey, deleteKey,
    renderConfig, copyConfig, downloadConfig, downloadEnv, copyField,
    logout,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
