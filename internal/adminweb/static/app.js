const state = {
  token: sessionStorage.getItem('orbit_admin_token') || '',
  currentView: 'dashboard',
  selectedUserID: null,
  selectedUserIDs: new Set(),
  auditOffset: 0,
  auditLimit: 50,
  auditTotal: 0
};

const $ = (id) => document.getElementById(id);

const api = async (path, options = {}) => {
  const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  const response = await fetch(path, { ...options, headers });
  const payload = await response.json().catch(() => ({ success: false, error: '响应不是 JSON' }));
  if (!response.ok || payload.success === false) {
    if (response.status === 401) setAuth('');
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
	else {
		sessionStorage.removeItem('orbit_admin_token');
		clearSensitiveViews();
	}
	$('statusPill').textContent = state.token ? '已登录' : '未登录';
	$('loginPanel').classList.toggle('hidden', Boolean(state.token));
}

function clearSensitiveViews() {
	state.selectedUserID = null;
	state.selectedUserIDs.clear();
	$('recentAudits').textContent = '登录后显示最近审计。';
	$('usersList').textContent = '登录后显示用户列表。';
	$('auditList').textContent = '登录后显示审计日志。';
	$('backupReport').textContent = '登录后可执行备份就绪检查。';
	$('metricRuntime').textContent = '--';
	$('runtimeHint').textContent = '等待检查';
	$('metricAutoUnban').textContent = '--';
	$('autoUnbanHint').textContent = '等待检查';
	$('metricTokenPolicy').textContent = '--';
	$('tokenPolicyHint').textContent = '等待检查';
	['recentAudits', 'usersList', 'auditList', 'backupReport'].forEach(id => $(id).classList.add('empty'));
	$('userDetail').classList.add('hidden');
	$('userDetail').innerHTML = '';
}

function showView(view) {
  state.currentView = view;
  const titles = { dashboard: '仪表盘', users: '用户治理', policies: '系统策略', backup: '备份自检', audit: '审计日志' };
  $('pageTitle').textContent = titles[view] || '仪表盘';
  document.querySelectorAll('.nav-item').forEach(btn => btn.classList.toggle('active', btn.dataset.view === view));
  ['dashboard', 'users', 'policies', 'backup', 'audit'].forEach(name => {
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
  if (view === 'backup') return loadBackup();
  if (view === 'audit') return loadAudit();
}

async function loadDashboard() {
  const [data, runtime] = await Promise.all([
    api('/api/v1/admin/dashboard/overview'),
    api('/api/v1/admin/system/runtime')
  ]);
  $('metricUsers').textContent = data.users?.total ?? '--';
  $('metricBanned').textContent = data.users?.banned ?? '--';
  $('metricConfigs').textContent = data.configs?.total ?? '--';
  $('metricBackup').textContent = data.backup?.ready ? 'OK' : `${data.backup?.warning_count ?? 0} 警告`;
  renderRuntimeStatus(runtime);
  renderList($('recentAudits'), data.recent_audits || [], auditRow);
}

function renderRuntimeStatus(runtime) {
  $('metricRuntime').textContent = runtime.status === 'ok' ? 'OK' : 'DEGRADED';
  $('runtimeHint').textContent = `DB ${runtime.database?.reachable ? '正常' : '异常'} · 已运行 ${formatDuration(runtime.uptime_seconds || 0)}`;
  $('metricAutoUnban').textContent = runtime.auto_unban?.enabled ? 'ON' : 'OFF';
  $('autoUnbanHint').textContent = runtime.auto_unban?.enabled
    ? `${runtime.auto_unban.effective_interval_minutes} 分钟/次 · 上限 ${runtime.auto_unban.effective_batch_limit}`
    : '未启用，到期封禁需手动扫描';
  $('metricTokenPolicy').textContent = runtime.jwt?.secret_strength_status === 'strong' ? 'SAFE' : 'WEAK';
  $('tokenPolicyHint').textContent = `Access ${runtime.jwt?.access_expire_minutes ?? '-'} 分钟 · Refresh ${runtime.jwt?.refresh_expire_days ?? '-'} 天`;
}

async function loadUsers() {
  const params = new URLSearchParams({ limit: '50', offset: '0' });
  if ($('userSearch').value.trim()) params.set('q', $('userSearch').value.trim());
  if ($('userStatus').value) params.set('status', $('userStatus').value);
  const data = await api(`/api/v1/admin/users?${params}`);
  renderList($('usersList'), data.items || [], userRow);
  syncSelectedUsers(data.items || []);
  if (state.selectedUserID) {
    const stillVisible = (data.items || []).some(user => Number(user.id) === Number(state.selectedUserID));
    if (stillVisible) loadUserDetail(state.selectedUserID).catch(err => toast(err.message));
    else hideUserDetail();
  }
}

async function loadUserDetail(userID) {
  state.selectedUserID = userID;
  const user = await api(`/api/v1/admin/users/${userID}`);
  renderUserDetail(user);
}

async function loadPolicies() {
  const [security, recovery, auditPolicy] = await Promise.all([
    api('/api/v1/admin/system/security-policy'),
    api('/api/v1/admin/system/recovery-policy'),
    api('/api/v1/admin/system/audit-policy')
  ]);
  $('registrationEnabled').checked = Boolean(security.registration_enabled);
  $('minPasswordLength').value = security.min_password_length || 8;
  $('registrationReason').value = security.registration_disabled_reason || '';
  $('supportContact').value = recovery.support_contact || '';
  $('recoveryMessage').value = recovery.user_facing_message || '';
  $('auditRetentionDays').value = auditPolicy.retention_days || 180;
  $('auditCleanupBatchLimit').value = auditPolicy.cleanup_batch_limit || 500;
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
  await api('/api/v1/admin/system/audit-policy', {
    method: 'PUT',
    body: JSON.stringify({
      retention_days: Number($('auditRetentionDays').value || 180),
      cleanup_batch_limit: Number($('auditCleanupBatchLimit').value || 500),
      reason: '管理端更新审计日志保留策略'
    })
  });
  toast('策略已保存');
}

async function loadBackup() {
  const data = await api('/api/v1/admin/system/backup-readiness');
  renderBackup(data);
}

async function loadAudit() {
  const params = new URLSearchParams({ limit: String(state.auditLimit), offset: String(state.auditOffset) });
  if ($('auditAction').value.trim()) params.set('action', $('auditAction').value.trim());
  if ($('auditTarget').value.trim()) params.set('target_user_id', $('auditTarget').value.trim());
  const data = await api(`/api/v1/admin/audit-logs?${params}`);
  state.auditTotal = data.total || 0;
  renderList($('auditList'), data.items || [], auditRow);
  renderAuditPager();
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
  row.className = `row user-row status-${escapeClass(user.status)}`;

  const checkbox = document.createElement('input');
  checkbox.type = 'checkbox';
  checkbox.className = 'select-user';
  checkbox.checked = state.selectedUserIDs.has(Number(user.id));
  checkbox.setAttribute('aria-label', `选择用户 ${user.username}`);
  checkbox.addEventListener('change', () => toggleUserSelection(user.id, checkbox.checked));

  const info = document.createElement('button');
  info.className = 'row-main';
  info.type = 'button';
  info.innerHTML = `
    <strong>${escapeHtml(user.username)}</strong>
    <small>ID ${user.id} · ${escapeHtml(user.role)} · ${escapeHtml(user.status)} · Token v${user.token_version ?? 0}</small>
    ${user.is_banned ? `<small class="warn">封禁至：${formatDate(user.ban_until) || '永久'} · ${escapeHtml(user.ban_reason || '')}</small>` : ''}
  `;
  info.addEventListener('click', () => loadUserDetail(user.id).catch(err => toast(err.message)));

  const actions = document.createElement('div');
  actions.className = 'actions';
  actions.append(
    actionButton('详情', 'ghost', () => loadUserDetail(user.id)),
    actionButton('封禁', 'danger', () => banUser(user.id)),
    actionButton('解封', 'ghost', () => unbanUser(user.id)),
    actionButton('下线', 'danger', () => highRisk(`/api/v1/admin/users/${user.id}/force-logout`, { reason: promptReason('强制下线原因') }))
  );
  row.append(checkbox, info, actions);
  return row;
}

function toggleUserSelection(userID, checked) {
  const id = Number(userID);
  if (checked) state.selectedUserIDs.add(id);
  else state.selectedUserIDs.delete(id);
  updateBatchBar();
}

function syncSelectedUsers(users) {
  const visibleIDs = new Set(users.map(user => Number(user.id)));
  for (const id of Array.from(state.selectedUserIDs)) {
    if (!visibleIDs.has(id)) state.selectedUserIDs.delete(id);
  }
  updateBatchBar();
}

function updateBatchBar() {
  const count = state.selectedUserIDs.size;
  $('batchBar').classList.toggle('hidden', count === 0);
  $('batchCount').textContent = `已选择 ${count} 个用户`;
}

function renderUserDetail(user) {
  const node = $('userDetail');
  node.classList.remove('hidden');
  node.innerHTML = `
    <div class="panel-head compact">
      <div>
        <h3>${escapeHtml(user.username)}</h3>
        <p>ID ${user.id} · ${escapeHtml(user.role)} · ${escapeHtml(user.status)}</p>
      </div>
      <button class="ghost small" id="closeUserDetail">关闭</button>
    </div>
    <div class="kv-grid">
      ${kv('注册时间', formatDate(user.created_at))}
      ${kv('最后登录', formatDate(user.last_login_at) || '暂无')}
      ${kv('最后登录 IP', user.last_login_ip || '暂无')}
      ${kv('Token 版本', user.token_version ?? 0)}
      ${kv('必须改密码', user.must_change_password ? '是' : '否')}
      ${kv('封禁状态', user.is_banned ? `已封禁至 ${formatDate(user.ban_until) || '永久'}` : '正常')}
      ${kv('封禁原因', user.ban_reason || '无')}
      ${kv('删除状态', user.is_deleted ? `已注销 ${formatDate(user.deleted_at)}` : '未删除')}
    </div>
    <div class="actions detail-actions">
      <button class="ghost" id="resetPasswordButton">重置登录密码</button>
      <button class="ghost" id="changeRoleButton">调整角色</button>
      <button class="danger" id="softDeleteButton">软删除/注销</button>
      <button class="ghost" id="restoreButton">恢复用户</button>
    </div>
  `;
  $('closeUserDetail').addEventListener('click', hideUserDetail);
  $('resetPasswordButton').addEventListener('click', () => resetPassword(user.id).catch(err => toast(err.message)));
  $('changeRoleButton').addEventListener('click', () => changeRole(user.id, user.role).catch(err => toast(err.message)));
  $('softDeleteButton').addEventListener('click', () => highRisk(`/api/v1/admin/users/${user.id}/soft-delete`, { reason: promptReason('软删除/注销原因') }).catch(err => toast(err.message)));
  $('restoreButton').addEventListener('click', () => restoreUser(user.id).catch(err => toast(err.message)));
}

function hideUserDetail() {
  state.selectedUserID = null;
  $('userDetail').classList.add('hidden');
  $('userDetail').innerHTML = '';
}

function renderBackup(report) {
  const node = $('backupReport');
  node.classList.remove('empty');
  const envRows = (report.environment || []).map(item => `
    <div class="check-row severity-${escapeClass(item.severity)}">
      <div><strong>${escapeHtml(item.key)}</strong><small>${escapeHtml(item.message)}</small></div>
      <span>${item.secure ? '安全' : item.configured ? '需处理' : '未配置'}</span>
      <code>${escapeHtml(item.masked_value || '-')}</code>
    </div>
  `).join('');
  const counts = Object.entries(report.database?.table_counts || {})
    .map(([key, value]) => `<span class="chip">${escapeHtml(key)}: ${value}</span>`).join('');
  const warnings = (report.warnings || []).map(item => `<li>${escapeHtml(item)}</li>`).join('') || '<li>暂无警告。</li>';
  const items = (report.recommended_items || []).map(item => `<li><strong>${escapeHtml(item.name)}</strong> ${item.required ? '<em>必备</em>' : '<em>可选</em>'}<br><small>${escapeHtml(item.description)}</small></li>`).join('');
  const guides = (report.operational_guides || []).map(item => `<li>${escapeHtml(item)}</li>`).join('');

  node.innerHTML = `
    <article class="backup-card ${report.ready ? 'ready' : 'warning'}">
      <span>整体状态</span>
      <strong>${report.ready ? 'READY' : 'NEEDS ATTENTION'}</strong>
      <small>生成时间：${formatDate(report.generated_at)}</small>
    </article>
    <article class="backup-card wide-card">
      <h3>数据库</h3>
      <p>${report.database?.reachable ? '数据库可读' : '数据库检查存在警告'} · ${escapeHtml(report.database?.dialect || '-')} · ${escapeHtml(report.database?.backup_method || '-')}</p>
      <div class="chips">${counts}</div>
      <small>${escapeHtml(report.database?.hint || '')}</small>
    </article>
    <article class="backup-card wide-card">
      <h3>环境变量脱敏检查</h3>
      <div class="checks">${envRows}</div>
    </article>
    <article class="backup-card"><h3>警告</h3><ul>${warnings}</ul></article>
    <article class="backup-card"><h3>建议备份项</h3><ul>${items}</ul></article>
    <article class="backup-card wide-card"><h3>恢复步骤提示</h3><ol>${guides}</ol></article>
  `;
}

function auditRow(log) {
  const row = document.createElement('div');
  row.className = 'row';
  row.innerHTML = `<div><strong>${escapeHtml(log.action || '-')}</strong><small>管理员 ${log.admin_user_id || '-'} · 目标 ${log.target_user_id || '-'} · ${formatDate(log.created_at)}</small></div>`;
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

async function banUser(userID) {
  const reason = promptReason('封禁原因');
  if (!reason) return;
  const minutes = Number($('banDuration').value || 0);
  const body = { reason };
  if (minutes > 0) body.duration_minutes = minutes;
  await highRisk(`/api/v1/admin/users/${userID}/ban`, body);
}

async function batchUserAction(action) {
  const ids = Array.from(state.selectedUserIDs);
  if (!ids.length) return;
  const reason = promptReason(`批量${actionLabel(action)}原因`);
  if (!reason) return;
  if (!requireTypedConfirmation(`批量${actionLabel(action)} ${ids.length} 个用户`)) return;

  const failures = [];
  for (const id of ids) {
    try {
      await runUserAction(id, action, reason);
    } catch (err) {
      failures.push(`用户 ${id}: ${err.message}`);
    }
  }
  state.selectedUserIDs.clear();
  await refresh('users');
  toast(failures.length ? `批量操作完成，失败 ${failures.length} 个：${failures.join('；')}` : '批量操作完成');
}

async function forceLogoutRegularUsers() {
  const reason = promptReason('普通用户全部下线原因');
  if (!reason) return;
  if (!requireTypedConfirmation('普通用户全部下线')) return;
  const result = await api('/api/v1/admin/users/force-logout-regular', {
    method: 'POST',
    body: JSON.stringify({ reason, confirmation: 'CONFIRM' })
  });
  toast(`普通用户旧 Token 已失效，影响 ${result.affected_count || 0} 个账号`);
  await refresh('users');
}

async function runUserAction(userID, action, reason) {
  if (action === 'ban') {
    const minutes = Number($('banDuration').value || 0);
    const body = { reason, confirmation: 'CONFIRM' };
    if (minutes > 0) body.duration_minutes = minutes;
    await api(`/api/v1/admin/users/${userID}/ban`, { method: 'POST', body: JSON.stringify(body) });
    return;
  }
  if (action === 'unban') {
    await api(`/api/v1/admin/users/${userID}/unban`, { method: 'POST', body: JSON.stringify({ reason }) });
    return;
  }
  if (action === 'forceLogout') {
    await api(`/api/v1/admin/users/${userID}/force-logout`, { method: 'POST', body: JSON.stringify({ reason, confirmation: 'CONFIRM' }) });
  }
}

function actionLabel(action) {
  return { ban: '封禁', unban: '解封', forceLogout: '下线' }[action] || '操作';
}

async function unbanUser(userID) {
  const reason = promptReason('解封原因');
  if (!reason) return;
  await api(`/api/v1/admin/users/${userID}/unban`, { method: 'POST', body: JSON.stringify({ reason }) });
  await refresh('users');
}

async function resetPassword(userID) {
  const newPassword = window.prompt('请输入新的登录密码（不会影响主密码，用户云端资产仍需要原主密码解密）');
  if (!newPassword) return;
  const reason = promptReason('重置登录密码原因');
  if (!reason) return;
  await highRisk(`/api/v1/admin/users/${userID}/reset-password`, { new_password: newPassword, reason });
}

async function restoreUser(userID) {
  const reason = promptReason('恢复用户原因');
  if (!reason) return;
  await api(`/api/v1/admin/users/${userID}/restore`, { method: 'POST', body: JSON.stringify({ reason }) });
  await refresh('users');
}

async function createManagedUser() {
  const username = $('managedUsername').value.trim();
  const password = $('managedPassword').value;
  const role = $('managedRole').value;
  const reason = promptReason('创建受管账号原因');
  if (!username || !password || !role || !reason) return;
  if (!requireTypedConfirmation(`创建 ${role} 账号 ${username}`)) return;
  await api('/api/v1/admin/users/managed', {
    method: 'POST',
    body: JSON.stringify({ username, password, role, reason, confirmation: 'CONFIRM' })
  });
  $('managedUsername').value = '';
  $('managedPassword').value = '';
  toast('账号已创建');
  await refresh('users');
}

async function changeRole(userID, currentRole) {
  const role = window.prompt(`请输入新角色：super_admin / admin / support / user\n当前角色：${currentRole}`);
  if (!role) return;
  const reason = promptReason('调整角色原因');
  if (!reason) return;
  if (!requireTypedConfirmation(`将用户 ${userID} 角色调整为 ${role}`)) return;
  await api(`/api/v1/admin/users/${userID}/role`, {
    method: 'POST',
    body: JSON.stringify({ role, reason, confirmation: 'CONFIRM' })
  });
  toast('角色已调整，目标用户旧 Token 已失效');
  await refresh('users');
}

async function highRisk(path, body) {
  if (!body.reason) return;
  if (!requireTypedConfirmation('高危操作')) return;
  await api(path, { method: 'POST', body: JSON.stringify({ ...body, confirmation: 'CONFIRM' }) });
  await refresh('users');
}

function requireTypedConfirmation(title) {
  return window.prompt(`${title} 将写入审计日志。请输入 CONFIRM 继续。`) === 'CONFIRM';
}

async function exportAudit() {
  if (!state.token) return;
  const params = new URLSearchParams({ limit: '200', offset: '0' });
  if ($('auditAction').value.trim()) params.set('action', $('auditAction').value.trim());
  if ($('auditTarget').value.trim()) params.set('target_user_id', $('auditTarget').value.trim());
  const data = await api(`/api/v1/admin/audit-logs?${params}`);
  downloadJSON(data, `orbitterm-audit-${new Date().toISOString().slice(0, 10)}.json`);
}

async function cleanupAuditLogs() {
  const reason = promptReason('清理过期审计日志原因');
  if (!reason) return;
  if (!requireTypedConfirmation('清理过期审计日志')) return;
  const result = await api('/api/v1/admin/audit-logs/cleanup', {
    method: 'POST',
    body: JSON.stringify({ reason, confirmation: 'CONFIRM' })
  });
  toast(`审计清理完成，删除 ${result.deleted_count || 0} 条，截止 ${formatDate(result.cutoff)}`);
  await loadAudit();
}

async function exportDiagnostics() {
  if (!state.token) return;
  const data = await api('/api/v1/admin/system/diagnostics');
  downloadJSON(data, `orbitterm-diagnostics-${new Date().toISOString().slice(0, 10)}.json`);
  toast('诊断包已导出，敏感密钥已脱敏');
}

function downloadJSON(data, filename) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function renderAuditPager() {
  const page = Math.floor(state.auditOffset / state.auditLimit) + 1;
  const pages = Math.max(1, Math.ceil(state.auditTotal / state.auditLimit));
  $('auditPageInfo').textContent = `第 ${page} / ${pages} 页，共 ${state.auditTotal} 条`;
  $('auditPrev').disabled = state.auditOffset <= 0;
  $('auditNext').disabled = state.auditOffset + state.auditLimit >= state.auditTotal;
}

function moveAuditPage(direction) {
  state.auditOffset = Math.max(0, state.auditOffset + direction * state.auditLimit);
  loadAudit().catch(err => toast(err.message));
}

function promptReason(title) {
  const reason = window.prompt(`${title}（必填，至少 2 个字符）`);
  return (reason || '').trim();
}

function kv(key, value) {
  return `<div><span>${escapeHtml(key)}</span><strong>${escapeHtml(value ?? '-')}</strong></div>`;
}

function formatDate(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString('zh-CN', { hour12: false });
}

function formatDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days > 0) return `${days}天${hours}小时`;
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  return `${minutes}分钟`;
}

function escapeClass(value) {
  return String(value || 'unknown').replace(/[^a-zA-Z0-9_-]/g, '-');
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>'"]/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[char]));
}

$('loginButton').addEventListener('click', () => login().catch(err => toast(err.message)));
$('bootstrapButton').addEventListener('click', () => bootstrap().catch(err => toast(err.message)));
$('logoutButton').addEventListener('click', () => { setAuth(''); hideUserDetail(); toast('已退出'); });
$('savePolicies').addEventListener('click', () => savePolicies().catch(err => toast(err.message)));
$('refreshBackup').addEventListener('click', () => loadBackup().catch(err => toast(err.message)));
$('exportDiagnostics').addEventListener('click', () => exportDiagnostics().catch(err => toast(err.message)));
$('scanExpiredBans').addEventListener('click', () => highRisk('/api/v1/admin/users/expired-bans/scan', { limit: 100, reason: promptReason('扫描原因') }));
$('forceLogoutRegularUsers').addEventListener('click', () => forceLogoutRegularUsers().catch(err => toast(err.message)));
$('batchBan').addEventListener('click', () => batchUserAction('ban').catch(err => toast(err.message)));
$('batchUnban').addEventListener('click', () => batchUserAction('unban').catch(err => toast(err.message)));
$('batchForceLogout').addEventListener('click', () => batchUserAction('forceLogout').catch(err => toast(err.message)));
$('batchClear').addEventListener('click', () => { state.selectedUserIDs.clear(); refresh('users').catch(err => toast(err.message)); });
$('exportAudit').addEventListener('click', () => exportAudit().catch(err => toast(err.message)));
$('cleanupAudit').addEventListener('click', () => cleanupAuditLogs().catch(err => toast(err.message)));
$('createManagedUser').addEventListener('click', () => createManagedUser().catch(err => toast(err.message)));
$('auditPrev').addEventListener('click', () => moveAuditPage(-1));
$('auditNext').addEventListener('click', () => moveAuditPage(1));
document.querySelectorAll('.nav-item').forEach(btn => btn.addEventListener('click', () => showView(btn.dataset.view)));
document.querySelectorAll('[data-refresh]').forEach(btn => btn.addEventListener('click', () => refresh(btn.dataset.refresh).catch(err => toast(err.message))));
['userSearch', 'userStatus'].forEach(id => $(id).addEventListener('change', () => refresh(state.currentView).catch(err => toast(err.message))));
['auditAction', 'auditTarget'].forEach(id => $(id).addEventListener('change', () => { state.auditOffset = 0; refresh(state.currentView).catch(err => toast(err.message)); }));
['loginUsername', 'loginPassword'].forEach(id => $(id).addEventListener('keydown', event => {
  if (event.key === 'Enter') login().catch(err => toast(err.message));
}));

setAuth(state.token);
checkBootstrap().catch(err => toast(err.message));
showView('dashboard');
