const state = {
  token: sessionStorage.getItem('orbit_admin_token') || '',
  currentView: 'dashboard'
};

const $ = (id) => document.getElementById(id);
const api = async (path, options = {}) => {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const response = await fetch(path, { ...options, headers });
  const payload = await response.json().catch(() => ({ success: false, error: '响应不是 JSON' }));
  if (!response.ok || payload.success === false) {
    throw new Error(payload.error || `HTTP ${response.status}`);
  }
  return payload.data;
};

function toast(message) {
  const node = $('toast');
  node.textContent = message;
  node.classList.remove('hidden');
  setTimeout(() => node.classList.add('hidden'), 3800);
}

function setAuth(token) {
  state.token = token || '';
  if (state.token) sessionStorage.setItem('orbit_admin_token', state.token);
  else sessionStorage.removeItem('orbit_admin_token');
  $('statusPill').textContent = state.token ? '已登录' : '未登录';
  $('loginPanel').classList.toggle('hidden', Boolean(state.token));
}

function showView(view) {
  state.currentView = view;
  const titles = { dashboard: '仪表盘', users: '用户治理', policies: '系统策略', audit: '审计日志' };
  $('pageTitle').textContent = titles[view] || '仪表盘';
  document.querySelectorAll('.nav-item').forEach(btn => btn.classList.toggle('active', btn.dataset.view === view));
  ['dashboard', 'users', 'policies', 'audit'].forEach(name => {
    $(`${name}View`).classList.toggle('hidden', name !== view);
  });
  if (state.token) refresh(view).catch(err => toast(err.message));
}

async function checkBootstrap() {
  const status = await api('/api/v1/admin/bootstrap/status');
  $('bootstrapPanel').classList.toggle('hidden', !status.needs_setup);
}

async function login() {
  const data = await api('/api/v1/admin/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username: $('loginUsername').value.trim(), password: $('loginPassword').value })
  });
  setAuth(data.access_token || data.token);
  toast('登录成功');
  await refresh(state.currentView);
}

async function bootstrap() {
  await api('/api/v1/admin/bootstrap/super-admin', {
    method: 'POST',
    headers: { 'X-Orbit-Admin-Bootstrap-Token': $('bootstrapToken').value },
    body: JSON.stringify({ username: $('bootstrapUsername').value.trim(), password: $('bootstrapPassword').value })
  });
  toast('首个管理员已创建，请登录');
  await checkBootstrap();
}

async function refresh(view) {
  if (!state.token) return;
  if (view === 'dashboard') return loadDashboard();
  if (view === 'users') return loadUsers();
  if (view === 'policies') return loadPolicies();
  if (view === 'audit') return loadAudit();
}

async function loadDashboard() {
  const data = await api('/api/v1/admin/dashboard/overview');
  $('metricUsers').textContent = data.users?.total ?? '--';
  $('metricBanned').textContent = data.users?.banned ?? '--';
  $('metricConfigs').textContent = data.configs?.total ?? '--';
  $('metricBackup').textContent = data.backup?.ready ? 'OK' : `${data.backup?.warning_count ?? 0} 警告`;
  renderList($('recentAudits'), data.recent_audits || [], auditRow);
}

async function loadUsers() {
  const params = new URLSearchParams({ limit: '50', offset: '0' });
  if ($('userSearch').value.trim()) params.set('q', $('userSearch').value.trim());
  if ($('userStatus').value) params.set('status', $('userStatus').value);
  const data = await api(`/api/v1/admin/users?${params}`);
  renderList($('usersList'), data.items || [], userRow);
}

async function loadPolicies() {
  const [security, recovery] = await Promise.all([
    api('/api/v1/admin/system/security-policy'),
    api('/api/v1/admin/system/recovery-policy')
  ]);
  $('registrationEnabled').checked = Boolean(security.registration_enabled);
  $('minPasswordLength').value = security.min_password_length || 8;
  $('registrationReason').value = security.registration_disabled_reason || '';
  $('supportContact').value = recovery.support_contact || '';
  $('recoveryMessage').value = recovery.user_facing_message || '';
}

async function savePolicies() {
  await api('/api/v1/admin/system/security-policy', {
    method: 'PUT',
    body: JSON.stringify({
      registration_enabled: $('registrationEnabled').checked,
      registration_disabled_reason: $('registrationReason').value.trim(),
      min_password_length: Number($('minPasswordLength').value || 8),
      reason: '管理端更新安全策略'
    })
  });
  await api('/api/v1/admin/system/recovery-policy', {
    method: 'PUT',
    body: JSON.stringify({
      support_contact: $('supportContact').value.trim(),
      user_facing_message: $('recoveryMessage').value.trim(),
      reason: '管理端更新恢复策略文案'
    })
  });
  toast('策略已保存');
}

async function loadAudit() {
  const params = new URLSearchParams({ limit: '50', offset: '0' });
  if ($('auditAction').value.trim()) params.set('action', $('auditAction').value.trim());
  if ($('auditTarget').value.trim()) params.set('target_user_id', $('auditTarget').value.trim());
  const data = await api(`/api/v1/admin/audit-logs?${params}`);
  renderList($('auditList'), data.items || [], auditRow);
}

function renderList(container, items, renderer) {
  container.innerHTML = '';
  if (!items.length) {
    container.textContent = '暂无数据。';
    container.classList.add('empty');
    return;
  }
  container.classList.remove('empty');
  items.forEach(item => container.appendChild(renderer(item)));
}

function userRow(user) {
  const row = document.createElement('div');
  row.className = 'row';
  row.innerHTML = `<div><strong>${escapeHtml(user.username)}</strong><small>ID ${user.id} · ${user.role} · ${user.status}</small></div>`;
  const actions = document.createElement('div');
  actions.className = 'actions';
  actions.append(
    actionButton('封禁', 'danger', () => highRisk(`/api/v1/admin/users/${user.id}/ban`, { duration_minutes: 1440, reason: promptReason('封禁原因') })),
    actionButton('解封', 'ghost', () => api(`/api/v1/admin/users/${user.id}/unban`, { method: 'POST', body: JSON.stringify({ reason: promptReason('解封原因') }) }).then(() => refresh('users'))),
    actionButton('下线', 'danger', () => highRisk(`/api/v1/admin/users/${user.id}/force-logout`, { reason: promptReason('强制下线原因') }))
  );
  row.appendChild(actions);
  return row;
}

function auditRow(log) {
  const row = document.createElement('div');
  row.className = 'row';
  row.innerHTML = `<div><strong>${escapeHtml(log.action || '-')}</strong><small>管理员 ${log.admin_user_id || '-'} · 目标 ${log.target_user_id || '-'} · ${escapeHtml(log.created_at || '')}</small></div>`;
  return row;
}

function actionButton(text, cls, handler) {
  const btn = document.createElement('button');
  btn.className = cls || 'ghost';
  btn.textContent = text;
  btn.addEventListener('click', async () => {
    try { await handler(); toast(`${text} 操作完成`); }
    catch (err) { toast(err.message); }
  });
  return btn;
}

async function highRisk(path, body) {
  if (!body.reason) return;
  if (!confirm('这是高危操作，确认继续？')) return;
  await api(path, { method: 'POST', body: JSON.stringify({ ...body, confirmation: 'CONFIRM' }) });
  await refresh('users');
}

function promptReason(title) {
  const reason = window.prompt(`${title}（必填）`);
  return (reason || '').trim();
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

$('loginButton').addEventListener('click', () => login().catch(err => toast(err.message)));
$('bootstrapButton').addEventListener('click', () => bootstrap().catch(err => toast(err.message)));
$('logoutButton').addEventListener('click', () => { setAuth(''); toast('已退出'); });
$('savePolicies').addEventListener('click', () => savePolicies().catch(err => toast(err.message)));
$('scanExpiredBans').addEventListener('click', () => highRisk('/api/v1/admin/users/expired-bans/scan', { limit: 100, reason: promptReason('扫描原因') }));
document.querySelectorAll('.nav-item').forEach(btn => btn.addEventListener('click', () => showView(btn.dataset.view)));
document.querySelectorAll('[data-refresh]').forEach(btn => btn.addEventListener('click', () => refresh(btn.dataset.refresh).catch(err => toast(err.message))));
['userSearch', 'userStatus', 'auditAction', 'auditTarget'].forEach(id => $(id).addEventListener('change', () => refresh(state.currentView).catch(err => toast(err.message))));

setAuth(state.token);
checkBootstrap().catch(err => toast(err.message));
showView('dashboard');
