/* ==========================================================================
   HutaomuChat 管理控制台
   纯原生 JavaScript 实现，无构建步骤。
   ========================================================================== */

'use strict';

const State = {
  token: localStorage.getItem('hmc_admin_token') || '',
  adminName: localStorage.getItem('hmc_admin_name') || 'admin',
  page: 'dashboard',
  socket: null,
  stats: {},
  series: 'messages',
  users: { page: 1, pageSize: 20, total: 0, search: '' },
  chats: { page: 1, pageSize: 20, total: 0, search: '', type: '' },
  messages: { page: 1, pageSize: 50, total: 0, chatId: '' },
  moments: { page: 1, pageSize: 24, total: 0 },
  charts: { messages: [], registrations: [] },
  statsTimer: null,
};

const PAGE_META = {
  dashboard:    ['仪表盘', '服务端运行概览'],
  online:       ['在线用户', '实时查看当前连接的客户端'],
  users:        ['用户管理', '搜索、编辑、封禁与删除账号'],
  chats:        ['会话与群组', '查看所有单聊与群聊'],
  messages:     ['消息记录', '按会话浏览历史消息'],
  moments:      ['朋友圈动态', '审核用户发布的动态'],
  announcements:['群公告', '管理各群组的置顶公告'],
  joinreq:      ['入群申请', '待处理的入群申请'],
  logs:         ['运行日志', '系统日志与用户操作日志'],
  audit:        ['操作审计', '管理员在后台执行的动作记录'],
  system:       ['系统状态', '运行时信息与上传文件管理'],
  settings:     ['服务设置', '配置服务端参数与推送通道'],
};

/* ------------------------------------------------------------------ 工具 -- */

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

function escapeHTML(value) {
  if (value === null || value === undefined) return '';
  return String(value).replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));
}

function formatTime(millis) {
  if (!millis) return '—';
  const date = new Date(Number(millis));
  const now = new Date();
  const sameDay = date.toDateString() === now.toDateString();
  const pad = (n) => String(n).padStart(2, '0');
  if (sameDay) return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function formatDuration(seconds) {
  if (!seconds || seconds < 0) return '0 分钟';
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`;
  return `${minutes} 分钟`;
}

function formatSize(bytes) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index++; }
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function initials(name) {
  if (!name) return '?';
  const trimmed = String(name).trim();
  return trimmed.slice(0, 1).toUpperCase();
}

/* ---------------------------------------------------------------- 提示条 -- */

function toast(message, type = 'info', timeout = 3200) {
  const host = $('#toasts');
  const icons = { info: 'ℹ️', success: '✅', error: '⛔', warning: '⚠️' };
  const node = document.createElement('div');
  node.className = `toast ${type}`;
  node.innerHTML = `<span class="icon">${icons[type] || icons.info}</span><span>${escapeHTML(message)}</span>`;
  host.appendChild(node);
  setTimeout(() => {
    node.classList.add('hide');
    setTimeout(() => node.remove(), 240);
  }, timeout);
}

/* ------------------------------------------------------------------ 接口 -- */

async function api(path, options = {}) {
  const config = {
    method: options.method || 'GET',
    headers: { 'Authorization': `Bearer ${State.token}` },
  };
  if (options.body !== undefined) {
    config.headers['Content-Type'] = 'application/json';
    config.body = JSON.stringify(options.body);
  }
  const response = await fetch(`/api/admin${path}`, config);
  if (response.status === 401) {
    logout();
    throw new Error('登录已失效');
  }
  const text = await response.text();
  let payload = {};
  if (text) {
    try { payload = JSON.parse(text); } catch { payload = {}; }
  }
  if (!response.ok) {
    throw new Error(payload.error || `请求失败 (${response.status})`);
  }
  return payload;
}

/* ------------------------------------------------------------------ 登录 -- */

async function login(event) {
  event.preventDefault();
  const username = $('#loginUser').value.trim();
  const password = $('#loginPass').value;
  const errorBox = $('#loginError');
  const button = $('#loginBtn');
  errorBox.style.display = 'none';
  button.disabled = true;
  button.textContent = '登录中…';
  try {
    const response = await fetch('/api/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(payload.error || '登录失败');
    State.token = payload.token;
    State.adminName = payload.user || username;
    localStorage.setItem('hmc_admin_token', State.token);
    localStorage.setItem('hmc_admin_name', State.adminName);
    enterConsole();
    toast('欢迎回来，' + State.adminName, 'success');
  } catch (error) {
    errorBox.textContent = error.message;
    errorBox.style.display = 'block';
  } finally {
    button.disabled = false;
    button.textContent = '登录控制台';
  }
}

function logout() {
  State.token = '';
  localStorage.removeItem('hmc_admin_token');
  localStorage.removeItem('hmc_admin_name');
  if (State.socket) { State.socket.close(); State.socket = null; }
  $('#app').classList.add('hidden');
  $('#loginScreen').style.display = 'grid';
  $('#loginPass').value = '';
}

function enterConsole() {
  $('#loginScreen').style.display = 'none';
  $('#app').classList.remove('hidden');
  $('#serverName').textContent = State.stats.serverName || 'HutaomuChat';
  navigate(State.page);
  connectSocket();
  if (State.statsTimer) clearInterval(State.statsTimer);
  State.statsTimer = setInterval(() => { if (State.token) refreshStats(); }, 20000);
}

/* ---------------------------------------------------------------- 路由 -- */

function navigate(page) {
  State.page = page;
  $$('.nav-item').forEach((item) => item.classList.toggle('active', item.dataset.page === page));
  $$('.page').forEach((section) => section.classList.toggle('active', section.id === `page-${page}`));
  const [title, desc] = PAGE_META[page] || ['', ''];
  $('#pageTitle').textContent = title;
  $('#pageDesc').textContent = desc;
  $('#sidebar').classList.remove('open');
  loadPage(page);
}

function loadPage(page) {
  switch (page) {
    case 'dashboard': return loadDashboard();
    case 'online': return loadOnline();
    case 'users': return loadUsers();
    case 'chats': return loadChats();
    case 'messages': return loadMessages();
    case 'moments': return loadMoments();
    case 'announcements': return loadAnnouncements();
    case 'joinreq': return loadJoinRequests();
    case 'logs': return loadLogs();
    case 'audit': return loadAudit();
    case 'system': return loadSystem();
    case 'settings': return loadSettings();
    default: return undefined;
  }
}

/* -------------------------------------------------------------- 仪表盘 -- */

async function loadDashboard() {
  const stats = await refreshStats();
  renderStatCards(stats);
  renderAreaChart(State.charts[State.series] || []);
  renderDonut(stats.msgTypes || {});
  await Promise.all([renderRecentUsers(), renderRuntimeInfo()]);
}

async function refreshStats() {
  try {
    const stats = await api('/stats');
    State.stats = stats;
    $('#serverName').textContent = stats.serverName || 'HutaomuChat';
    State.charts.messages = stats.dailyMessages || [];
    State.charts.registrations = stats.dailyRegistrations || [];
    const badge = $('#onlineBadge');
    badge.textContent = stats.onlineUsers || 0;
    badge.classList.toggle('hidden', !stats.onlineUsers);
    const pending = stats.pendingJoinRequests || 0;
    const joinBadge = $('#joinBadge');
    joinBadge.textContent = pending;
    joinBadge.classList.toggle('hidden', !pending);
    return stats;
  } catch (error) {
    console.error(error);
    return State.stats;
  }
}

function renderStatCards(stats) {
  const cards = [
    { label: '在线用户', value: stats.onlineUsers ?? 0, foot: `共 ${stats.totalConnections ?? 0} 个连接`, tone: 'accent', icon: '🟢' },
    { label: '注册用户', value: stats.totalUsers ?? 0, foot: `今日新增 ${stats.todayRegistrations ?? 0}`, tone: 'success', icon: '👥' },
    { label: '消息总数', value: stats.totalMessages ?? 0, foot: `今日 ${stats.todayMessages ?? 0} 条`, tone: 'info', icon: '💬' },
    { label: '会话 / 群组', value: stats.totalChats ?? 0, foot: `其中群聊 ${stats.totalGroups ?? 0} 个`, tone: 'purple', icon: '📁' },
    { label: '动态总数', value: stats.totalMoments ?? 0, foot: `今日 ${stats.todayMoments ?? 0} 条`, tone: 'accent', icon: '🌈' },
    { label: '好友关系', value: stats.totalFriendships ?? 0, foot: `待处理申请 ${stats.pendingJoinRequests ?? 0}`, tone: 'success', icon: '🤝' },
    { label: '离线队列', value: stats.offlineQueueSize ?? 0, foot: '待拉取的消息', tone: 'warning', icon: '📥' },
    { label: '运行时长', value: formatDuration(stats.uptime ?? 0), foot: '服务持续在线', tone: 'info', icon: '⏱️' },
  ];
  $('#statCards').innerHTML = cards.map((card) => `
    <div class="stat ${card.tone}">
      <div class="stat-label"><span>${card.icon}</span>${card.label}</div>
      <div class="stat-value">${escapeHTML(card.value)}</div>
      <div class="stat-foot">${escapeHTML(card.foot)}</div>
    </div>`).join('');
}

/* 面积图（SVG） */
function renderAreaChart(points) {
  const host = $('#chartArea');
  if (!points.length) { host.innerHTML = '<div class="empty">暂无数据</div>'; return; }
  const width = 780;
  const height = 240;
  const padding = { top: 16, right: 16, bottom: 28, left: 38 };
  const values = points.map((p) => Number(p.count) || 0);
  const max = Math.max(...values, 1);
  const innerW = width - padding.left - padding.right;
  const innerH = height - padding.top - padding.bottom;
  const step = points.length > 1 ? innerW / (points.length - 1) : innerW;

  const coords = values.map((value, index) => {
    const x = padding.left + (points.length > 1 ? index * step : innerW / 2);
    const y = padding.top + innerH - (value / max) * innerH;
    return [x, y];
  });

  const line = coords.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ');
  const area = `${line} L${coords[coords.length - 1][0].toFixed(1)},${(padding.top + innerH).toFixed(1)} L${coords[0][0].toFixed(1)},${(padding.top + innerH).toFixed(1)} Z`;

  const gridLines = [0, 0.25, 0.5, 0.75, 1].map((ratio) => {
    const y = padding.top + innerH - ratio * innerH;
    const label = Math.round(max * ratio);
    return `<line x1="${padding.left}" y1="${y}" x2="${width - padding.right}" y2="${y}" stroke="var(--border)" stroke-width="1"/>
            <text x="${padding.left - 8}" y="${y + 4}" text-anchor="end" font-size="10" fill="var(--text-dim)">${label}</text>`;
  }).join('');

  const dots = coords.map(([x, y], i) => `
    <circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="3" fill="var(--accent)">
      <title>${escapeHTML(points[i].label || points[i].date)}: ${values[i]}</title>
    </circle>`).join('');

  const labels = points.map((point, i) => {
    if (points.length > 8 && i % 2 !== 0) return '';
    const x = padding.left + (points.length > 1 ? i * step : innerW / 2);
    return `<text x="${x.toFixed(1)}" y="${height - 8}" text-anchor="middle" font-size="10" fill="var(--text-dim)">${escapeHTML(point.label || point.date)}</text>`;
  }).join('');

  host.innerHTML = `
    <svg class="chart" viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" style="height:240px">
      <defs>
        <linearGradient id="areaFill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="var(--accent)" stop-opacity="0.34"/>
          <stop offset="100%" stop-color="var(--accent)" stop-opacity="0.02"/>
        </linearGradient>
      </defs>
      ${gridLines}
      <path d="${area}" fill="url(#areaFill)"/>
      <path d="${line}" fill="none" stroke="var(--accent)" stroke-width="2.2" stroke-linejoin="round" stroke-linecap="round"/>
      ${dots}
      ${labels}
    </svg>`;
}

/* 环形图（SVG） */
function renderDonut(types) {
  const host = $('#chartDonut');
  const entries = Object.entries(types).filter(([, value]) => value > 0);
  if (!entries.length) { host.innerHTML = '<div class="empty">暂无数据</div>'; return; }
  const palette = ['#6366f1', '#10b981', '#f59e0b', '#38bdf8', '#a855f7', '#ef4444', '#14b8a6', '#f472b6'];
  const total = entries.reduce((sum, [, value]) => sum + value, 0);
  const radius = 62;
  const circumference = 2 * Math.PI * radius;
  let offset = 0;

  const arcs = entries.map(([label, value], index) => {
    const portion = value / total;
    const length = portion * circumference;
    const arc = `<circle cx="80" cy="80" r="${radius}" fill="none"
      stroke="${palette[index % palette.length]}" stroke-width="20"
      stroke-dasharray="${length.toFixed(2)} ${(circumference - length).toFixed(2)}"
      stroke-dashoffset="${(-offset).toFixed(2)}"
      transform="rotate(-90 80 80)"><title>${escapeHTML(label)}: ${value}</title></circle>`;
    offset += length;
    return arc;
  }).join('');

  const legend = entries.map(([label, value], index) => `
    <div class="donut-row">
      <span class="legend-swatch" style="background:${palette[index % palette.length]}"></span>
      <span class="label">${escapeHTML(label)}</span>
      <span class="value">${value}</span>
    </div>`).join('');

  host.innerHTML = `
    <div class="donut-wrap">
      <svg width="160" height="160" viewBox="0 0 160 160">
        <circle cx="80" cy="80" r="${radius}" fill="none" stroke="var(--border)" stroke-width="20"/>
        ${arcs}
        <text x="80" y="76" text-anchor="middle" font-size="22" font-weight="700" fill="var(--text)">${total}</text>
        <text x="80" y="96" text-anchor="middle" font-size="11" fill="var(--text-dim)">条消息</text>
      </svg>
      <div class="donut-legend">${legend}</div>
    </div>`;
}

async function renderRecentUsers() {
  const payload = await api('/users?page=1&pageSize=8');
  const host = $('#recentUsers');
  if (!payload.users || !payload.users.length) {
    host.innerHTML = '<div class="empty">还没有用户</div>';
    return;
  }
  host.innerHTML = payload.users.map((user) => `
    <div class="list-row" style="padding:9px 0;border-bottom:1px solid var(--border)">
      <div class="avatar sm">${escapeHTML(initials(user.nickname))}</div>
      <div class="grow">
        <div class="title">${escapeHTML(user.nickname)}</div>
        <div class="meta">${escapeHTML(user.uid)} · ${formatTime(user.created_at)}</div>
      </div>
      <span class="tag ${user.online ? 'success' : ''}">${user.online ? '在线' : '离线'}</span>
    </div>`).join('');
}

async function renderRuntimeInfo() {
  const stats = State.stats;
  const info = [
    ['服务器名称', stats.serverName || '—'],
    ['在线用户', `${stats.onlineUsers ?? 0} 人`],
    ['消息总数', `${stats.totalMessages ?? 0} 条`],
    ['今日消息', `${stats.todayMessages ?? 0} 条`],
    ['会话数量', `${stats.totalChats ?? 0} 个`],
    ['运行时长', formatDuration(stats.uptime ?? 0)],
  ];
  $('#runtimeInfo').innerHTML = info.map(([key, value]) => `<dt>${key}</dt><dd>${escapeHTML(value)}</dd>`).join('');
}

/* ------------------------------------------------------------ 在线用户 -- */

async function loadOnline() {
  const payload = await api('/online');
  const host = $('#onlineGrid');
  $('#onlineSummary').textContent = `${payload.total || 0} 人在线`;
  if (!payload.online || !payload.online.length) {
    host.innerHTML = '<div class="empty" style="grid-column:1/-1"><div class="icon">🕊️</div>当前没有在线用户</div>';
    return;
  }
  host.innerHTML = payload.online.map((user) => `
    <div class="online-card">
      <div class="avatar">${escapeHTML(initials(user.nickname))}</div>
      <div class="grow" style="min-width:0">
        <div class="name" style="font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">${escapeHTML(user.nickname)}</div>
        <div class="meta dim" style="font-size:11.5px">${escapeHTML(user.uid)} · ${user.connections} 个连接</div>
      </div>
      <span class="dot live"></span>
    </div>`).join('');
}

/* ------------------------------------------------------------ 用户管理 -- */

async function loadUsers() {
  const { page, pageSize, search } = State.users;
  const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
  if (search) params.set('search', search);
  const payload = await api(`/users?${params}`);
  State.users.total = payload.total || 0;
  $('#userCount').textContent = `共 ${payload.total || 0} 位用户`;

  const rows = payload.users || [];
  $('#userRows').innerHTML = rows.length ? rows.map((user) => `
    <tr>
      <td>
        <div class="cell-user">
          <div class="avatar sm">${escapeHTML(initials(user.nickname))}</div>
          <div class="meta">
            <div class="name">${escapeHTML(user.nickname)}</div>
            <div class="sub">${escapeHTML(user.signature || '—')}</div>
          </div>
        </div>
      </td>
      <td class="mono dim">${escapeHTML(user.uid)}</td>
      <td class="mono dim">${escapeHTML(user.phone || '—')}</td>
      <td>
        <span class="tag ${user.banned ? 'danger' : (user.online ? 'success' : '')}">${user.banned ? '已封禁' : (user.online ? '在线' : '离线')}</span>
      </td>
      <td class="dim">${user.friend_count} / ${user.chat_count}</td>
      <td class="dim nowrap">${formatTime(user.created_at)}</td>
      <td class="actions">
        <button class="btn btn-sm" data-action="user-detail" data-id="${user.id}">详情</button>
        <button class="btn btn-sm" data-action="user-ban" data-id="${user.id}">${user.banned ? '解封' : '封禁'}</button>
        <button class="btn btn-sm btn-danger" data-action="user-delete" data-id="${user.id}">删除</button>
      </td>
    </tr>`).join('') : '<tr><td colspan="7"><div class="empty"><div class="icon">👤</div>没有匹配的用户</div></td></tr>';

  renderPager('#userPager', State.users, () => loadUsers());
}

async function showUserDetail(id) {
  const payload = await api(`/users/${id}`);
  const user = payload.user;
  openDrawer('用户详情', `
    <div class="row-gap mb">
      <div class="avatar lg">${escapeHTML(initials(user.nickname))}</div>
      <div>
        <div style="font-weight:700;font-size:15px">${escapeHTML(user.nickname)}</div>
        <div class="dim" style="font-size:12px">${escapeHTML(user.uid)} · ${escapeHTML(user.phone || '未绑定手机号')}</div>
      </div>
    </div>
    <dl class="kv">
      <dt>个性签名</dt><dd>${escapeHTML(user.signature || '—')}</dd>
      <dt>性别 / 地区</dt><dd>${escapeHTML(user.gender)} · ${escapeHTML(user.region || '—')}</dd>
      <dt>账号状态</dt><dd>${user.banned ? '<span class="tag danger">已封禁</span>' : '<span class="tag success">正常</span>'}</dd>
      <dt>在线状态</dt><dd>${user.online ? '<span class="tag success">在线</span>' : '<span class="tag">离线</span>'}</dd>
      <dt>推送令牌</dt><dd>${user.has_push_token ? '已注册' : '未注册'}</dd>
      <dt>好友 / 会话</dt><dd>${user.friend_count} 位好友 · ${user.chat_count} 个会话</dd>
      <dt>注册时间</dt><dd>${formatTime(user.created_at)}</dd>
      <dt>最后在线</dt><dd>${formatTime(user.last_seen_at)}</dd>
    </dl>
    <h4 style="margin:22px 0 10px;font-size:13px">好友列表</h4>
    ${(payload.friends || []).length ? payload.friends.map((f) => `
      <div class="list-row" style="padding:8px 0"><div class="avatar sm">${escapeHTML(initials(f.nickname))}</div>
      <div class="grow"><div class="title">${escapeHTML(f.nickname)}</div><div class="meta">${escapeHTML(f.uid)}</div></div></div>`).join('')
      : '<p class="dim">暂无好友</p>'}
    <h4 style="margin:22px 0 10px;font-size:13px">会话列表</h4>
    ${(payload.chats || []).length ? payload.chats.map((c) => `
      <div class="list-row" style="padding:8px 0">
        <div class="grow"><div class="title">${escapeHTML(c.name || c.id)}</div>
        <div class="meta">${c.type === 'GROUP' ? '群聊' : '单聊'} · ${c.members} 人</div></div>
        <span class="tag">${escapeHTML(c.role)}</span>
      </div>`).join('') : '<p class="dim">暂无会话</p>'}
  `, [
    { label: '重置密码', class: 'btn', action: 'user-reset-password' },
    { label: '封禁 / 解封', class: 'btn', action: 'user-ban' },
    { label: '删除用户', class: 'btn btn-danger', action: 'user-delete' },
  ]);
}

/* ------------------------------------------------------------ 会话管理 -- */

async function loadChats() {
  const { page, pageSize, search, type } = State.chats;
  const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
  if (search) params.set('search', search);
  if (type) params.set('type', type);
  const payload = await api(`/chats?${params}`);
  State.chats.total = payload.total || 0;
  $('#chatCount').textContent = `共 ${payload.total || 0} 个会话`;

  const rows = payload.chats || [];
  $('#chatRows').innerHTML = rows.length ? rows.map((chat) => `
    <tr>
      <td>
        <div class="cell-user">
          <div class="avatar sm">${chat.type === 'GROUP' ? '👥' : '💬'}</div>
          <div class="meta"><div class="name">${escapeHTML(chat.name || '未命名会话')}</div>
          <div class="sub mono">${escapeHTML(chat.id.slice(0, 8))}…</div></div>
        </div>
      </td>
      <td class="mono dim">${escapeHTML(chat.group_uid || '—')}</td>
      <td class="dim">${escapeHTML(chat.owner_name)}</td>
      <td class="dim">${chat.member_count}</td>
      <td class="dim">${chat.message_count}</td>
      <td class="dim nowrap">${formatTime(chat.updated_at)}</td>
      <td class="actions">
        <button class="btn btn-sm" data-action="chat-messages" data-id="${chat.id}" data-name="${escapeHTML(chat.name || chat.id)}">消息</button>
        <button class="btn btn-sm btn-danger" data-action="chat-delete" data-id="${chat.id}">删除</button>
      </td>
    </tr>`).join('') : '<tr><td colspan="7"><div class="empty"><div class="icon">💬</div>没有匹配的会话</div></td></tr>';

  renderPager('#chatPager', State.chats, () => loadChats());
  const selector = $('#messageChat');
  const current = selector.value;
  selector.innerHTML = '<option value="">选择会话查看消息…</option>' + rows.map((chat) =>
    `<option value="${chat.id}">${escapeHTML(chat.name || chat.id)}</option>`).join('');
  selector.value = current;
}

/* ------------------------------------------------------------ 消息记录 -- */

async function loadMessages() {
  const chatId = $('#messageChat').value || State.messages.chatId;
  if (!chatId) {
    $('#messageRows').innerHTML = '<tr><td colspan="4"><div class="empty"><div class="icon">✉️</div>请先在上方选择一个会话</div></td></tr>';
    $('#messageCount').textContent = '';
    $('#messagePager').innerHTML = '';
    return;
  }
  State.messages.chatId = chatId;
  const { page, pageSize } = State.messages;
  const payload = await api(`/chats/${chatId}/messages?page=${page}&pageSize=${pageSize}`);
  State.messages.total = payload.total || 0;
  $('#messageCount').textContent = `共 ${payload.total || 0} 条消息`;

  const rows = payload.messages || [];
  $('#messageRows').innerHTML = rows.length ? rows.map((message) => `
    <tr>
      <td class="dim nowrap mono">${formatTime(message.timestamp)}</td>
      <td>${escapeHTML(message.sender_name)}</td>
      <td><span class="tag">${escapeHTML(message.type)}</span></td>
      <td>${escapeHTML(message.content).slice(0, 400)}</td>
    </tr>`).join('') : '<tr><td colspan="4"><div class="empty">该会话暂无消息</div></td></tr>';

  renderPager('#messagePager', State.messages, () => loadMessages());
}

/* ---------------------------------------------------------------- 动态 -- */

async function loadMoments() {
  const { page, pageSize } = State.moments;
  const payload = await api(`/moments?page=${page}&pageSize=${pageSize}`);
  State.moments.total = payload.total || 0;
  $('#momentCount').textContent = `共 ${payload.total || 0} 条动态`;

  const moments = payload.moments || [];
  $('#momentGrid').innerHTML = moments.length ? moments.map((moment) => `
    <div class="moment-card">
      <div class="cell-user">
        <div class="avatar sm">${escapeHTML(initials(moment.author_name))}</div>
        <div class="meta">
          <div class="name">${escapeHTML(moment.author_name)}</div>
          <div class="sub">${formatTime(moment.created_at)} · ${moment.scope === 'friends' ? '好友可见' : '公开'}</div>
        </div>
      </div>
      <div class="moment-text">${escapeHTML(moment.content)}</div>
      ${(moment.images || []).length ? `<div class="moment-images">${moment.images.map((src) =>
        `<img src="${escapeHTML(src)}" alt="" loading="lazy" onerror="this.style.display='none'">`).join('')}</div>` : ''}
      <div class="row-gap dim" style="font-size:12px">
        <span>👍 ${moment.like_count}</span><span>💬 ${moment.comment_count}</span>
        <button class="btn btn-sm btn-danger" style="margin-left:auto" data-action="moment-delete" data-id="${moment.id}">删除</button>
      </div>
    </div>`).join('') : '<div class="empty" style="grid-column:1/-1"><div class="icon">🌈</div>还没有动态</div>';

  renderPager('#momentPager', State.moments, () => loadMoments());
}

/* ---------------------------------------------------------------- 公告 -- */

async function loadAnnouncements() {
  const payload = await api('/announcements');
  const rows = payload.announcements || [];
  $('#announcementRows').innerHTML = rows.length ? rows.map((item) => `
    <tr>
      <td>${escapeHTML(item.content).slice(0, 160)}</td>
      <td class="dim">${escapeHTML(item.chat_name)}</td>
      <td class="dim">${escapeHTML(item.creator_name)}</td>
      <td>${item.at_all ? '<span class="tag warning">@全体</span>' : '<span class="dim">—</span>'}</td>
      <td class="dim nowrap">${formatTime(item.created_at)}</td>
      <td class="actions">
        <button class="btn btn-sm btn-danger" data-action="announcement-delete" data-id="${item.id}">删除</button>
      </td>
    </tr>`).join('') : '<tr><td colspan="6"><div class="empty"><div class="icon">📢</div>暂无公告</div></td></tr>';
}

/* ------------------------------------------------------------ 入群申请 -- */

async function loadJoinRequests() {
  const payload = await api('/join-requests');
  const rows = payload.joinRequests || [];
  $('#joinRows').innerHTML = rows.length ? rows.map((item) => `
    <tr>
      <td>
        <div class="cell-user">
          <div class="avatar sm">${escapeHTML(initials(item.user_name))}</div>
          <div class="meta"><div class="name">${escapeHTML(item.user_name)}</div><div class="sub">${escapeHTML(item.user_uid)}</div></div>
        </div>
      </td>
      <td class="dim">${escapeHTML(item.chat_name)}</td>
      <td class="dim">${escapeHTML(item.message || '—')}</td>
      <td><span class="tag ${item.status === 'PENDING' ? 'warning' : (item.status === 'ACCEPTED' ? 'success' : '')}">${escapeHTML(statusLabel(item.status))}</span></td>
      <td class="dim nowrap">${formatTime(item.created_at)}</td>
    </tr>`).join('') : '<tr><td colspan="5"><div class="empty"><div class="icon">🚪</div>暂无入群申请</div></td></tr>';
}

function statusLabel(status) {
  return { PENDING: '待处理', ACCEPTED: '已同意', REJECTED: '已拒绝' }[status] || status;
}

/* ---------------------------------------------------------------- 日志 -- */

async function loadLogs() {
  const stats = await api('/logs/stats');
  const levels = Object.entries(stats.systemLevels || {});
  const actions = Object.entries(stats.userActions || {}).sort((a, b) => b[1] - a[1]).slice(0, 8);
  const top = stats.topUsers || [];

  $('#logStats').innerHTML = `
    <dl class="kv mb">
      <dt>系统日志</dt><dd>${stats.systemLogCount || 0} 条</dd>
      <dt>用户操作</dt><dd>${stats.userLogCount || 0} 条</dd>
    </dl>
    <h4 style="margin:0 0 8px;font-size:12.5px">级别分布</h4>
    ${levels.length ? levels.map(([level, count]) => `
      <div class="donut-row"><span class="label">${escapeHTML(level)}</span><span class="value">${count}</span></div>`).join('')
      : '<p class="dim">暂无数据</p>'}
    <h4 style="margin:18px 0 8px;font-size:12.5px">操作类型</h4>
    ${actions.length ? actions.map(([action, count]) => `
      <div class="donut-row"><span class="label">${escapeHTML(action)}</span><span class="value">${count}</span></div>`).join('')
      : '<p class="dim">暂无数据</p>'}
    <h4 style="margin:18px 0 8px;font-size:12.5px">活跃用户</h4>
    ${top.length ? top.map((user) => `
      <div class="donut-row"><span class="label">${escapeHTML(user.nickname)}</span><span class="value">${user.count}</span></div>`).join('')
      : '<p class="dim">暂无数据</p>'}`;

  const [systemLogs, userLogs] = await Promise.all([api('/logs/system?limit=200'), api('/logs/user?limit=200')]);
  const lines = [
    ...systemLogs.logs.map((log) => ({ time: log.time, level: log.level, message: log.message, data: log.data })),
    ...userLogs.logs.map((log) => ({ time: log.time, level: 'USER', message: `[${log.userId}] ${log.action}`, data: log.detail })),
  ].sort((a, b) => String(b.time).localeCompare(String(a.time)));

  $('#logMemory').innerHTML = lines.length ? lines.map((log) => `
    <div class="log-line">
      <span class="ts">${escapeHTML(String(log.time).slice(11, 19))}</span>
      <span class="lvl ${escapeHTML(log.level)}">${escapeHTML(log.level)}</span>
      <span>${escapeHTML(log.message)}${log.data && Object.keys(log.data).length ? ' ' + escapeHTML(JSON.stringify(log.data)) : ''}</span>
    </div>`).join('') : '<span class="dim">暂无日志</span>';
}

async function loadLogFile() {
  const stream = $('#logStream').value;
  const date = $('#logDate').value;
  const params = new URLSearchParams({ lines: '400' });
  if (date) params.set('date', date);
  const payload = await api(`/logs/file/${stream}?${params}`);
  $('#logView').textContent = payload.content || '（空）';
}

/* ---------------------------------------------------------------- 审计 -- */

async function loadAudit() {
  const payload = await api('/audit?limit=200');
  const rows = payload.audit || [];
  $('#auditRows').innerHTML = rows.length ? rows.map((item) => `
    <tr>
      <td class="dim nowrap mono">${formatTime(item.created_at)}</td>
      <td>${escapeHTML(item.admin_user || '—')}</td>
      <td><span class="tag accent">${escapeHTML(item.action)}</span></td>
      <td class="mono dim">${escapeHTML(item.target || '—')}</td>
      <td class="dim">${escapeHTML(item.detail || '')}</td>
      <td class="mono dim">${escapeHTML(item.ip || '—')}</td>
    </tr>`).join('') : '<tr><td colspan="6"><div class="empty"><div class="icon">🛡️</div>暂无操作记录</div></td></tr>';
}

/* ------------------------------------------------------------ 系统状态 -- */

async function loadSystem() {
  const payload = await api('/system');
  const memory = payload.memory || {};
  $('#systemInfo').innerHTML = [
    ['运行时长', formatDuration(payload.uptime)],
    ['启动时间', formatTime(payload.started_at)],
    ['Go 版本', payload.go_version || '—'],
    ['运行平台', payload.platform || '—'],
    ['CPU 核心', payload.num_cpu || '—'],
    ['协程数量', payload.goroutines || 0],
    ['GC 次数', memory.num_gc || 0],
    ['堆内存', formatSize(memory.heap_alloc || 0)],
    ['系统内存', formatSize(memory.sys || 0)],
  ].map(([key, value]) => `<dt>${key}</dt><dd>${escapeHTML(String(value))}</dd>`).join('');

  $('#storageInfo').innerHTML = [
    ['数据目录', payload.data_dir || '—'],
    ['上传文件数', payload.upload_count || 0],
    ['上传文件大小', formatSize(payload.upload_size || 0)],
    ['WebSocket 连接', payload.ws_connections || 0],
    ['在线用户', payload.online_users || 0],
    ['推送通道', payload.push_status === 'active' ? '<span class="tag success">已配置</span>' : '<span class="tag">未配置</span>'],
  ].map(([key, value]) => `<dt>${key}</dt><dd>${value}</dd>`).join('');

  const files = payload.upload_files || [];
  $('#uploadSummary').textContent = `共 ${files.length} 个文件`;
  $('#uploadRows').innerHTML = files.length ? files.map((file) => `
    <tr>
      <td class="mono">${escapeHTML(file.name)}</td>
      <td class="dim">${formatSize(file.size)}</td>
      <td class="dim nowrap">${formatTime(file.created)}</td>
      <td class="actions">
        <a class="btn btn-sm" href="/uploads/${encodeURIComponent(file.name)}" target="_blank">查看</a>
        <button class="btn btn-sm btn-danger" data-action="file-delete" data-id="${escapeHTML(file.name)}">删除</button>
      </td>
    </tr>`).join('') : '<tr><td colspan="4"><div class="empty">暂无上传文件</div></td></tr>';
}

/* ---------------------------------------------------------------- 设置 -- */

async function loadSettings() {
  const payload = await api('/settings');
  $('#setServerName').value = payload.serverName || '';
  $('#setServerPort').value = payload.serverPort || 3000;
  $('#setAdminUser').value = payload.adminUser || 'admin';
  $('#setJwtSecret').value = payload.jwtSecret || '';
}

async function saveSettings() {
  const body = {
    serverName: $('#setServerName').value.trim(),
    serverPort: Number($('#setServerPort').value) || 3000,
    adminUser: $('#setAdminUser').value.trim(),
  };
  const password = $('#setAdminPass').value;
  if (password) body.adminPass = password;
  const secret = $('#setJwtSecret').value;
  if (secret && secret !== '********') body.jwtSecret = secret;
  try {
    const payload = await api('/settings', { method: 'PUT', body });
    toast(payload.message || '设置已保存', 'success');
    $('#setAdminPass').value = '';
  } catch (error) {
    toast(error.message, 'error');
  }
}

/* -------------------------------------------------------------- 分页器 -- */

function renderPager(selector, state, reload) {
  const host = $(selector);
  const pages = Math.max(1, Math.ceil((state.total || 0) / state.pageSize));
  const current = Math.min(state.page, pages);
  const buttons = [];
  const push = (label, page, disabled, active) => {
    buttons.push(`<button class="page-btn ${active ? 'active' : ''}" data-page="${page}" ${disabled ? 'disabled' : ''}>${label}</button>`);
  };
  push('‹', current - 1, current <= 1, false);
  const start = Math.max(1, current - 2);
  const end = Math.min(pages, start + 4);
  for (let i = start; i <= end; i++) push(String(i), i, false, i === current);
  push('›', current + 1, current >= pages, false);

  host.innerHTML = `
    <span class="info">第 ${current} / ${pages} 页 · 共 ${state.total || 0} 条</span>
    <div class="pages">${buttons.join('')}</div>`;
  host.querySelectorAll('.page-btn').forEach((button) => {
    button.addEventListener('click', () => {
      const page = Number(button.dataset.page);
      if (page >= 1 && page <= pages) {
        state.page = page;
        reload();
      }
    });
  });
}

/* ---------------------------------------------------------------- 抽屉 -- */

function openDrawer(title, bodyHTML, actions = []) {
  $('#drawerTitle').textContent = title;
  $('#drawerBody').innerHTML = bodyHTML;
  const foot = $('#drawerFoot');
  foot.innerHTML = '';
  actions.forEach((action) => {
    const button = document.createElement('button');
    button.className = action.class || 'btn';
    button.textContent = action.label;
    button.addEventListener('click', action.onClick || action.action);
    foot.appendChild(button);
  });
  const close = document.createElement('button');
  close.className = 'btn';
  close.textContent = '关闭';
  close.addEventListener('click', closeDrawer);
  foot.appendChild(close);
  $('#drawer').classList.add('show');
  $('#overlay').classList.add('show');
}

function closeDrawer() {
  $('#drawer').classList.remove('show');
  $('#overlay').classList.remove('show');
}

/* ------------------------------------------------------------ 实时推送 -- */

function connectSocket() {
  if (State.socket) { try { State.socket.close(); } catch { /* ignore */ } }
  const protocol = location.protocol === 'https:' ? 'wss' : 'ws';
  const socket = new WebSocket(`${protocol}://${location.host}/ws/admin`);
  State.socket = socket;

  socket.onopen = () => {
    socket.send(JSON.stringify({ type: 'authenticate', token: State.token }));
    setLive(true);
  };
  socket.onmessage = (event) => {
    let payload;
    try { payload = JSON.parse(event.data); } catch { return; }
    if (payload.type === 'auth_error') { toast('实时连接认证失败', 'error'); setLive(false); return; }
    if (payload.type === 'stats') {
      State.stats = { ...State.stats, ...payload.stats };
      if (State.page === 'dashboard') {
        renderStatCards(State.stats);
        renderAreaChart(State.charts[State.series] || []);
      }
      const badge = $('#onlineBadge');
      badge.textContent = payload.stats.onlineUsers || 0;
      badge.classList.toggle('hidden', !payload.stats.onlineUsers);
      return;
    }
    if (payload.type === 'event') {
      toast(`服务端事件：${payload.event}`, 'info');
      if (State.page === 'online') loadOnline();
      if (State.page === 'users') loadUsers();
      return;
    }
  };
  socket.onclose = () => {
    setLive(false);
    if (State.token) setTimeout(connectSocket, 4000);
  };
  socket.onerror = () => setLive(false);
}

function setLive(alive) {
  const dot = $('#liveDot');
  const text = $('#liveText');
  dot.className = `dot ${alive ? 'live' : 'down'}`;
  text.textContent = alive ? '实时连接正常' : '实时连接断开';
}

/* ------------------------------------------------------------ 事件绑定 -- */

function bindEvents() {
  $('#loginForm').addEventListener('submit', login);
  $('#logoutBtn').addEventListener('click', () => { logout(); toast('已退出登录', 'info'); });
  $('#menuToggle').addEventListener('click', () => $('#sidebar').classList.toggle('open'));
  $('#refreshBtn').addEventListener('click', () => loadPage(State.page));
  $('#overlay').addEventListener('click', closeDrawer);
  $('#drawerClose').addEventListener('click', closeDrawer);

  $$('.nav-item').forEach((item) => {
    item.addEventListener('click', () => navigate(item.dataset.page));
  });

  $('#themeBtn').addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    localStorage.setItem('hmc_theme', next);
    $('#themeBtn').textContent = next === 'dark' ? '🌙' : '☀️';
    if (State.page === 'dashboard') renderAreaChart(State.charts[State.series] || []);
  });

  $$('[data-series]').forEach((button) => {
    button.addEventListener('click', () => {
      State.series = button.dataset.series;
      renderAreaChart(State.charts[State.series] || []);
    });
  });

  let userTimer;
  $('#userSearch').addEventListener('input', (event) => {
    clearTimeout(userTimer);
    userTimer = setTimeout(() => {
      State.users.search = event.target.value.trim();
      State.users.page = 1;
      loadUsers();
    }, 320);
  });

  let chatTimer;
  $('#chatSearch').addEventListener('input', (event) => {
    clearTimeout(chatTimer);
    chatTimer = setTimeout(() => {
      State.chats.search = event.target.value.trim();
      State.chats.page = 1;
      loadChats();
    }, 320);
  });

  $('#chatType').addEventListener('change', (event) => {
    State.chats.type = event.target.value;
    State.chats.page = 1;
    loadChats();
  });

  $('#messageChat').addEventListener('change', (event) => {
    State.messages.chatId = event.target.value;
    State.messages.page = 1;
    loadMessages();
  });

  $('#logReload').addEventListener('click', () => loadLogFile().catch((error) => toast(error.message, 'error')));
  $('#uploadReload').addEventListener('click', () => loadSystem());
  $('#saveSettings').addEventListener('click', saveSettings);

  $('#saveGetui').addEventListener('click', async () => {
    try {
      const payload = await api('/settings/getui', {
        method: 'PUT',
        body: {
          appId: $('#setGetuiAppId').value.trim(),
          appKey: $('#setGetuiAppKey').value.trim(),
          masterSecret: $('#setGetuiSecret').value.trim(),
        },
      });
      toast(payload.message || '推送配置已保存', 'success');
    } catch (error) { toast(error.message, 'error'); }
  });

  $('#sendBroadcast').addEventListener('click', async () => {
    try {
      const payload = await api('/broadcast', {
        method: 'POST',
        body: { title: $('#broadcastTitle').value.trim(), content: $('#broadcastContent').value.trim() },
      });
      toast(`广播已发送给 ${payload.reached} 个连接`, 'success');
      $('#broadcastContent').value = '';
    } catch (error) { toast(error.message, 'error'); }
  });

  $('#restartServer').addEventListener('click', async () => {
    if (!confirm('确定要重启服务端吗？重启期间客户端会短暂断线。')) return;
    try {
      const payload = await api('/settings/restart', { method: 'POST' });
      toast(payload.message || '重启信号已发送', 'success');
    } catch (error) { toast(error.message, 'error'); }
  });

  $('#resetServer').addEventListener('click', async () => {
    const code = $('#resetConfirm').value.trim();
    if (code !== 'RESET_ALL_DATA') { toast('请输入正确的确认码 RESET_ALL_DATA', 'error'); return; }
    if (!confirm('此操作会清空全部数据且不可撤销，确定继续吗？')) return;
    try {
      const payload = await api('/settings/reset', { method: 'POST', body: { confirm: code } });
      toast(payload.message || '服务端已格式化', 'success');
      $('#resetConfirm').value = '';
    } catch (error) { toast(error.message, 'error'); }
  });

  // 表格内的操作按钮
  document.addEventListener('click', async (event) => {
    const button = event.target.closest('[data-action]');
    if (!button) return;
    const { action, id, name } = button.dataset;
    try {
      switch (action) {
        case 'user-detail':
          await showUserDetail(id);
          break;
        case 'user-ban': {
          const payload = await api(`/users/${id}`);
          const banned = payload.user.banned;
          await api(`/users/${id}/${banned ? 'unban' : 'ban'}`, { method: 'POST' });
          toast(banned ? '已解除封禁' : '已封禁该用户', 'success');
          loadUsers();
          break;
        }
        case 'user-delete': {
          if (!confirm('删除用户将同时清除其好友、会话与消息，确定继续吗？')) return;
          await api(`/users/${id}`, { method: 'DELETE' });
          toast('用户已删除', 'success');
          loadUsers();
          break;
        }
        case 'user-reset-password': {
          const password = prompt('请输入新的密码（至少 6 位）');
          if (!password) return;
          await api(`/users/${id}/password`, { method: 'POST', body: { password } });
          toast('密码已重置', 'success');
          closeDrawer();
          break;
        }
        case 'chat-messages':
          State.messages.chatId = id;
          $('#messageChat').value = id;
          navigate('messages');
          break;
        case 'chat-delete': {
          if (!confirm(`确定删除会话「${name || id}」吗？该操作会清除全部消息。`)) return;
          await api(`/chats/${id}`, { method: 'DELETE' });
          toast('会话已删除', 'success');
          loadChats();
          break;
        }
        case 'moment-delete': {
          if (!confirm('确定删除这条动态吗？')) return;
          await api(`/moments/${id}`, { method: 'DELETE' });
          toast('动态已删除', 'success');
          loadMoments();
          break;
        }
        case 'announcement-delete': {
          if (!confirm('确定删除这条公告吗？')) return;
          await api(`/announcements/${id}`, { method: 'DELETE' });
          toast('公告已删除', 'success');
          loadAnnouncements();
          break;
        }
        case 'file-delete': {
          if (!confirm('确定删除这个文件吗？')) return;
          await api(`/system/files/${encodeURIComponent(id)}`, { method: 'DELETE' });
          toast('文件已删除', 'success');
          loadSystem();
          break;
        }
        default: break;
      }
    } catch (error) {
      toast(error.message, 'error');
    }
  });

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') closeDrawer();
  });
}

/* ---------------------------------------------------------------- 启动 -- */

function bootstrap() {
  const theme = localStorage.getItem('hmc_theme') || 'dark';
  document.documentElement.dataset.theme = theme;
  $('#themeBtn').textContent = theme === 'dark' ? '🌙' : '☀️';
  $('#logDate').value = new Date().toISOString().slice(0, 10);
  bindEvents();
  if (State.token) {
    enterConsole();
  } else {
    $('#loginScreen').style.display = 'grid';
  }
}

bootstrap();
