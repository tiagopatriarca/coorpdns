/**
 * CoorpDNS - Frontend Application Logic (Vanilla JS ES6+)
 */

const API_BASE = ''; // Relativo ao host atual

// Estado Global da Aplicação
const state = {
  currentTab: 'dashboard',
  currentReportSubtab: 'top-sites',
  reportDays: 30,
  domainStatusFilter: 'all',
  selectedClientForMatrix: '',
  selectedGroupForReport: 1,
  groups: [],
  clients: [],
  availableCategories: [],
  currentCategoryGroup: 1,
  groupCategoryRules: {},
  adForwarders: [],
  isBlocking: true,
  pauseTimeout: 0,
};

// ==========================================================
// Inicialização
// ==========================================================
document.addEventListener('DOMContentLoaded', () => {
  setupNavigation();
  setupReportsControls();
  setupLiveLogControls();
  setupClientModal();
  setupGroupControls();
  setupCategoryControls();
  setupActiveDirectoryControls();
  setupBlockPageSettingsControls();
  setupMasterToggle();
  setupRulesForm();

  // Carga inicial
  refreshStatus();
  refreshDashboard();
  loadGroups();
  loadClients();
  loadRules();
  loadLists();

  // Atualização periódica automática
  setInterval(refreshStatus, 3000);
  setInterval(refreshDashboard, 10000);
  setInterval(() => {
    if (state.currentTab === 'logs') {
      loadLiveLogs();
    }
  }, 2500);
});

// ==========================================================
// Navegação entre Abas Principais
// ==========================================================
function setupNavigation() {
  const navButtons = document.querySelectorAll('.nav-item');
  navButtons.forEach(btn => {
    btn.addEventListener('click', () => {
      const targetTab = btn.getAttribute('data-tab');
      switchTab(targetTab);
    });
  });

  // Links rápidos dentro do dashboard que levam a abas
  document.querySelectorAll('[data-go-tab]').forEach(el => {
    el.addEventListener('click', (e) => {
      e.preventDefault();
      switchTab(el.getAttribute('data-go-tab'));
    });
  });
}

function switchTab(tabId) {
  state.currentTab = tabId;

  document.querySelectorAll('.nav-item').forEach(btn => {
    btn.classList.toggle('active', btn.getAttribute('data-tab') === tabId);
  });

  document.querySelectorAll('.tab-pane').forEach(pane => {
    pane.classList.toggle('active', pane.id === `tab-${tabId}`);
  });

  const titles = {
    dashboard: ['Painel de Controle', 'Proteção de rede e bloqueio de anúncios ativo para todos os dispositivos Wi-Fi'],
    reports: ['Relatórios Históricos (180 Dias)', 'Auditoria completa de consumo por máquina, sites mais acessados e tendências'],
    logs: ['Log de Consultas DNS', 'Acompanhamento em tempo real das requisições DNS processadas na sua rede'],
    clients: ['Gerenciamento de Dispositivos', 'Nomeie e personalize os aparelhos conectados para facilitar o monitoramento'],
    rules: ['Regras & Listas de Bloqueio', 'Gerencie listas comunitárias e adicione regras de bloqueio ou exceção personalizadas'],
    guide: ['Guia de Configuração do Roteador', 'Como ativar a proteção em toda a sua residência com poucos cliques'],
  };

  if (titles[tabId]) {
    document.getElementById('pageTitle').textContent = titles[tabId][0];
    document.getElementById('pageSubtitle').textContent = titles[tabId][1];
  }

  // Dispara carregamento de dados da aba selecionada
  if (tabId === 'reports') {
    loadReportsData();
  } else if (tabId === 'logs') {
    loadLiveLogs();
  } else if (tabId === 'clients') {
    loadClients();
  }
}

// ==========================================================
// Status e Botão Master Toggle
// ==========================================================
async function refreshStatus() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/status`);
    if (!res.ok) return;
    const data = await res.json();

    state.isBlocking = data.blocking_active;
    state.pauseTimeout = data.pause_remaining;

    const ind = document.getElementById('globalStatusIndicator');
    const toggleBtn = document.getElementById('btnMasterToggle');
    const toggleText = document.getElementById('masterToggleText');
    const ruleCount = document.getElementById('sidebarRuleCount');
    const uptimeEl = document.getElementById('sidebarUptime');

    ruleCount.textContent = data.total_rules.toLocaleString();

    const hours = Math.floor(data.uptime_seconds / 3600);
    const mins = Math.floor((data.uptime_seconds % 3600) / 60);
    uptimeEl.textContent = `${hours}h ${mins}m`;

    if (data.pause_remaining > 0) {
      ind.className = 'badge-status offline';
      ind.textContent = `Pausado (${Math.ceil(data.pause_remaining / 60)}m)`;
      toggleBtn.className = 'btn btn-primary toggle-btn paused';
      toggleText.textContent = 'Pausado';
    } else if (data.blocking_active) {
      ind.className = 'badge-status online';
      ind.textContent = 'Ativo';
      toggleBtn.className = 'btn btn-primary toggle-btn';
      toggleText.textContent = 'Bloqueio Ativo';
    } else {
      ind.className = 'badge-status offline';
      ind.textContent = 'Desativado';
      toggleBtn.className = 'btn btn-primary toggle-btn paused';
      toggleText.textContent = 'Desativado';
    }
  } catch (err) {
    console.warn('Erro ao atualizar status:', err);
  }
}

function setupMasterToggle() {
  const btn = document.getElementById('btnMasterToggle');
  const btnPause = document.getElementById('btnPauseModal');
  const modal = document.getElementById('pauseModal');
  const btnClose = document.getElementById('btnClosePauseModal');

  btn.addEventListener('click', async () => {
    const newStatus = !state.isBlocking;
    await fetch(`${API_BASE}/api/v1/control/toggle`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: newStatus, duration_seconds: 0 }),
    });
    refreshStatus();
  });

  btnPause.addEventListener('click', () => {
    modal.classList.remove('hidden');
  });

  btnClose.addEventListener('click', () => {
    modal.classList.add('hidden');
  });

  document.querySelectorAll('.pause-opt').forEach(opt => {
    opt.addEventListener('click', async () => {
      const seconds = parseInt(opt.getAttribute('data-seconds'), 10);
      modal.classList.add('hidden');
      await fetch(`${API_BASE}/api/v1/control/toggle`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled: false, duration_seconds: seconds }),
      });
      refreshStatus();
    });
  });
}

// ==========================================================
// Dashboard (Métricas e Gráfico)
// ==========================================================
async function refreshDashboard() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/stats?days=1`);
    if (!res.ok) return;
    const stats = await res.json();

    document.getElementById('cardTotalQueries').textContent = stats.total_queries.toLocaleString();
    document.getElementById('cardBlockedQueries').textContent = stats.blocked_queries.toLocaleString();
    document.getElementById('cardBlockRate').textContent = `${stats.block_rate.toFixed(1)}%`;
    document.getElementById('cardActiveClients').textContent = stats.active_clients.toLocaleString();

    // Atualiza mini tabelas rápidas
    loadQuickDomains();
    loadQuickClients();
    renderTimelineChart();
  } catch (err) {
    console.error('Erro no dashboard:', err);
  }
}

async function loadQuickDomains() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/top-domains?days=1&limit=5`);
    if (!res.ok) return;
    const domains = await res.json();
    const tbody = document.querySelector('#tableQuickDomains tbody');
    if (!domains || domains.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">Nenhuma consulta hoje ainda.</td></tr>';
      return;
    }

    tbody.innerHTML = domains.map(d => `
      <tr>
        <td><strong>${escapeHtml(d.domain)}</strong></td>
        <td><span class="tag-badge ${d.blocked_queries > 0 ? 'blocked' : 'allowed'}">${d.blocked_queries > 0 ? 'Bloqueado' : 'Permitido'}</span></td>
        <td>${d.total_queries.toLocaleString()}</td>
      </tr>
    `).join('');
  } catch (err) {
    console.warn(err);
  }
}

async function loadQuickClients() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/top-clients?days=1&limit=5`);
    if (!res.ok) return;
    const clients = await res.json();
    const tbody = document.querySelector('#tableQuickClients tbody');
    if (!clients || clients.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" class="text-center text-muted">Nenhum cliente detectado hoje.</td></tr>';
      return;
    }

    tbody.innerHTML = clients.map(c => `
      <tr>
        <td><strong>${escapeHtml(c.client_name)}</strong> <small class="text-muted">(${c.client_ip})</small></td>
        <td>${c.total_queries.toLocaleString()}</td>
        <td class="text-red">${c.blocked_queries.toLocaleString()} (${c.block_rate.toFixed(0)}%)</td>
      </tr>
    `).join('');
  } catch (err) {
    console.warn(err);
  }
}

async function renderTimelineChart() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/timeline?days=30`);
    if (!res.ok) return;
    const points = await res.json();
    const svg = document.getElementById('timelineSvg');
    if (!points || points.length === 0) {
      svg.innerHTML = '<text x="450" y="120" text-anchor="middle" fill="#64748b" font-size="14">Aguardando dados de tráfego histórico...</text>';
      return;
    }

    const width = 900;
    const height = 240;
    const padding = 30;

    const maxVal = Math.max(...points.map(p => p.total_queries), 10);
    const stepX = (width - padding * 2) / Math.max(points.length - 1, 1);

    const totalPoints = [];
    const blockedPoints = [];

    points.forEach((p, idx) => {
      const x = padding + idx * stepX;
      const yTotal = height - padding - (p.total_queries / maxVal) * (height - padding * 2);
      const yBlocked = height - padding - (p.blocked_queries / maxVal) * (height - padding * 2);
      totalPoints.push(`${x},${yTotal}`);
      blockedPoints.push(`${x},${yBlocked}`);
    });

    const totalPath = `M ${totalPoints.join(' L ')}`;
    const blockedPath = `M ${blockedPoints.join(' L ')}`;

    // Monta SVG polilinha com gradientes suaves
    svg.innerHTML = `
      <defs>
        <linearGradient id="gradEmerald" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#10b981" stop-opacity="0.4"/>
          <stop offset="100%" stop-color="#10b981" stop-opacity="0.0"/>
        </linearGradient>
      </defs>
      <!-- Linhas de grade sutis -->
      <line x1="${padding}" y1="${padding}" x2="${width - padding}" y2="${padding}" stroke="rgba(255,255,255,0.05)" />
      <line x1="${padding}" y1="${height/2}" x2="${width - padding}" y2="${height/2}" stroke="rgba(255,255,255,0.05)" />
      <line x1="${padding}" y1="${height - padding}" x2="${width - padding}" y2="${height - padding}" stroke="rgba(255,255,255,0.1)" />

      <!-- Linhas de dados -->
      <path d="${totalPath}" fill="none" stroke="#10b981" stroke-width="3" stroke-linecap="round"/>
      <path d="${blockedPath}" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-dasharray="4 2"/>
    `;
  } catch (err) {
    console.warn(err);
  }
}

// ==========================================================
// ABA DE RELATÓRIOS (180 DIAS)
// ==========================================================
function setupReportsControls() {
  // Botões de período (Hoje, 7d, 30d, 90d, 180d)
  const periodBtns = document.querySelectorAll('#reportPeriodBtns button');
  periodBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      periodBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      state.reportDays = parseInt(btn.getAttribute('data-days'), 10);
      loadReportsData();
    });
  });

  // Sub-abas de relatórios (1. Top Sites, 2. Top Máquinas, 3. Máquina x Sites)
  const subtabs = document.querySelectorAll('.report-subtab');
  subtabs.forEach(tab => {
    tab.addEventListener('click', () => {
      subtabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      state.currentReportSubtab = tab.getAttribute('data-report');

      document.querySelectorAll('.subreport-content').forEach(c => c.classList.remove('active'));
      document.getElementById(`subreport-${state.currentReportSubtab}`).classList.add('active');

      loadReportsData();
    });
  });

  // Filtro de status de domínio
  const filterSelect = document.getElementById('selectDomainStatusFilter');
  filterSelect.addEventListener('change', () => {
    state.domainStatusFilter = filterSelect.value;
    loadReportTopSites();
  });

  // Seletor de máquina na matriz
  const matrixClientSelect = document.getElementById('selectMatrixClient');
  matrixClientSelect.addEventListener('change', () => {
    state.selectedClientForMatrix = matrixClientSelect.value;
    loadReportMatrix();
  });

  // Seletor de grupo no relatório por grupo
  const groupSelect = document.getElementById('selectReportGroup');
  if (groupSelect) {
    groupSelect.addEventListener('change', () => {
      state.selectedGroupForReport = parseInt(groupSelect.value, 10);
      loadGroupReport();
    });
  }

  // Botão Exportar CSV
  document.getElementById('btnExportCSV').addEventListener('click', () => {
    let type = 'domains';
    if (state.currentReportSubtab === 'top-machines') type = 'clients';
    if (state.currentReportSubtab === 'machine-matrix') type = 'matrix';
    if (state.currentReportSubtab === 'group-report') type = 'group';
    
    let url = `${API_BASE}/api/v1/reports/export-csv?type=${type}&days=${state.reportDays}`;
    if (type === 'matrix' && state.selectedClientForMatrix) {
      url += `&client_ip=${encodeURIComponent(state.selectedClientForMatrix)}`;
    }
    if (type === 'group') {
      url += `&group_id=${state.selectedGroupForReport}`;
    }
    window.location.href = url;
  });
}

function loadReportsData() {
  document.getElementById('subreport1Desc').textContent = `Analisando histórico agregado dos últimos ${state.reportDays} dias`;
  document.getElementById('subreport2Desc').textContent = `Consumo detalhado dos últimos ${state.reportDays} dias por dispositivo`;

  if (state.currentReportSubtab === 'top-sites') {
    loadReportTopSites();
  } else if (state.currentReportSubtab === 'top-machines') {
    loadReportTopMachines();
  } else if (state.currentReportSubtab === 'machine-matrix') {
    loadReportMatrixClients();
  } else if (state.currentReportSubtab === 'group-report') {
    loadGroupReport();
  }
}

// 4. Relatório por Grupo de Dispositivos (Lista dispositivos do grupo e sites acessados por cada um)
async function loadGroupReport() {
  const container = document.getElementById('groupDevicesContainer');
  const summaryStrip = document.getElementById('groupSummaryStrip');
  container.innerHTML = '<div class="text-center text-muted" style="padding: 2rem;">Carregando relatório do grupo...</div>';

  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/group?group_id=${state.selectedGroupForReport}&days=${state.reportDays}`);
    if (!res.ok) throw new Error();
    const data = await res.json();

    summaryStrip.innerHTML = `
      <span>Grupo: <strong style="color: ${data.group_color || 'inherit'};">● ${escapeHtml(data.group_name)}</strong></span>
      <span>Dispositivos no Grupo: <strong>${data.devices_count}</strong></span>
      <span>Consultas no Período: <strong>${data.total_queries.toLocaleString()}</strong></span>
      <span>Bloqueadas: <strong class="text-red">${data.blocked_queries.toLocaleString()} (${data.block_rate.toFixed(1)}%)</strong></span>
    `;

    if (!data.devices || data.devices.length === 0) {
      container.innerHTML = `
        <div class="card" style="text-align: center; padding: 2rem; color: var(--text-muted);">
          Nenhum dispositivo cadastrado neste grupo ainda. Você pode associar dispositivos a este grupo na aba <strong>Dispositivos</strong>!
        </div>
      `;
      return;
    }

    container.innerHTML = data.devices.map((dev, idx) => {
      const typeBadge = dev.is_remote
        ? '<span class="tag-badge allowed">Nuvem DoH</span>'
        : '<span class="tag-badge" style="background:rgba(56,189,248,0.15); color:var(--accent-blue)">Wi-Fi Local</span>';

      const domainsHtml = (!dev.domains || dev.domains.length === 0)
        ? '<p class="text-muted" style="font-size: 0.85rem; padding: 0.5rem 0;">Nenhum site acessado por este aparelho no período selecionado.</p>'
        : `
          <div class="table-container compact" style="margin-top: 0.5rem;">
            <table class="data-table">
              <thead>
                <tr>
                  <th>Domínio Solicitado</th>
                  <th>Consultas</th>
                  <th>Bloqueadas</th>
                  <th>Ação Rápida</th>
                </tr>
              </thead>
              <tbody>
                ${dev.domains.map(d => `
                  <tr>
                    <td><strong>${escapeHtml(d.domain)}</strong></td>
                    <td>${d.total_queries.toLocaleString()}</td>
                    <td class="${d.blocked_queries > 0 ? 'text-red' : 'text-muted'}">
                      ${d.blocked_queries.toLocaleString()}
                    </td>
                    <td>
                      ${d.blocked_queries > 0 ? `
                        <button class="btn btn-secondary btn-sm" onclick="quickAllowDomain('${d.domain}')">Liberar</button>
                      ` : `
                        <button class="btn btn-danger btn-sm" onclick="quickBlockDomain('${d.domain}')">Bloquear</button>
                      `}
                    </td>
                  </tr>
                `).join('')}
              </tbody>
            </table>
          </div>
        `;

      return `
        <div class="group-device-card" style="border-left: 4px solid ${data.group_color || '#10b981'};">
          <div class="group-device-header" onclick="toggleGroupDeviceCard('group-dev-body-${idx}')">
            <div class="group-device-meta">
              <span style="font-weight: 700; font-size: 1rem;">${escapeHtml(dev.client_name)}</span>
              ${typeBadge}
              <code style="font-size: 0.8rem; color: var(--text-dim);">${dev.client_ip}</code>
            </div>
            <div class="group-device-stats">
              <span><strong>${dev.total_queries.toLocaleString()}</strong> consultas</span>
              <span class="${dev.blocked_queries > 0 ? 'text-red' : 'text-muted'}"><strong>${dev.blocked_queries.toLocaleString()}</strong> bloqueadas (${dev.block_rate.toFixed(0)}%)</span>
              <button class="btn btn-secondary btn-sm" style="pointer-events: none;">
                Sites (${dev.domains_count}) ▼
              </button>
            </div>
          </div>
          <div id="group-dev-body-${idx}" class="group-device-domains-container">
            ${domainsHtml}
          </div>
        </div>
      `;
    }).join('');

  } catch (err) {
    container.innerHTML = '<div class="text-center text-red" style="padding: 2rem;">Erro ao carregar dados do grupo.</div>';
  }
}

function toggleGroupDeviceCard(id) {
  const el = document.getElementById(id);
  if (el) {
    el.classList.toggle('hidden');
  }
}

// 1. Relatório: Top Sites Mais Acessados
async function loadReportTopSites() {
  const tbody = document.getElementById('tbodyReportTopDomains');
  tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Carregando dados históricos...</td></tr>';

  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/top-domains?days=${state.reportDays}&limit=50&status=${state.domainStatusFilter}`);
    if (!res.ok) throw new Error();
    const domains = await res.json();

    if (!domains || domains.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Nenhum dado encontrado para este período.</td></tr>';
      return;
    }

    const maxQueries = domains[0].total_queries || 1;

    tbody.innerHTML = domains.map((d, index) => {
      const pct = ((d.total_queries / maxQueries) * 100).toFixed(0);
      const isBlocked = d.blocked_queries > 0;
      return `
        <tr>
          <td><strong class="text-dim">#${index + 1}</strong></td>
          <td>
            <strong>${escapeHtml(d.domain)}</strong>
          </td>
          <td>
            <span class="tag-badge ${isBlocked ? 'blocked' : 'allowed'}">
              ${isBlocked ? `Bloqueado (${d.block_rate.toFixed(0)}%)` : 'Permitido'}
            </span>
          </td>
          <td><strong>${d.total_queries.toLocaleString()}</strong></td>
          <td>
            <div class="proportion-bar-container">
              <div class="proportion-bar-bg">
                <div class="proportion-bar-fill ${isBlocked ? 'blocked' : ''}" style="width: ${pct}%"></div>
              </div>
              <small class="text-dim">${pct}%</small>
            </div>
          </td>
          <td>
            ${isBlocked ? `
              <button class="btn btn-secondary btn-sm" onclick="quickAllowDomain('${d.domain}')">Liberar</button>
            ` : `
              <button class="btn btn-danger btn-sm" onclick="quickBlockDomain('${d.domain}')">Bloquear</button>
            `}
          </td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    tbody.innerHTML = '<tr><td colspan="6" class="text-center text-red">Erro ao carregar relatório.</td></tr>';
  }
}

// 2. Relatório: Top Máquinas que Mais Consomem
async function loadReportTopMachines() {
  const tbody = document.getElementById('tbodyReportTopClients');
  tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Carregando máquinas...</td></tr>';

  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/top-clients?days=${state.reportDays}&limit=20`);
    if (!res.ok) throw new Error();
    const clients = await res.json();

    if (!clients || clients.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Nenhuma máquina encontrada.</td></tr>';
      return;
    }

    tbody.innerHTML = clients.map((c, idx) => `
      <tr>
        <td><strong class="text-dim">#${idx + 1}</strong></td>
        <td>
          <strong>${escapeHtml(c.client_name)}</strong><br>
          <code class="text-dim">${c.client_ip}</code>
        </td>
        <td><strong>${c.total_queries.toLocaleString()}</strong></td>
        <td class="text-red">${c.blocked_queries.toLocaleString()}</td>
        <td>
          <span class="tag-badge ${c.block_rate > 20 ? 'blocked' : 'allowed'}">
            ${c.block_rate.toFixed(1)}% do tráfego barrado
          </span>
        </td>
        <td>
          <button class="btn btn-secondary btn-sm" onclick="inspectClientMatrix('${c.client_ip}')">
            Ver Sites &rarr;
          </button>
        </td>
      </tr>
    `).join('');
  } catch (err) {
    tbody.innerHTML = '<tr><td colspan="6" class="text-center text-red">Erro ao carregar máquinas.</td></tr>';
  }
}

// 3. Relatório: Máquina x Sites Acessados por Período
async function loadReportMatrixClients() {
  const select = document.getElementById('selectMatrixClient');
  try {
    const res = await fetch(`${API_BASE}/api/v1/clients`);
    const clients = await res.json();
    state.clients = clients || [];

    select.innerHTML = '<option value="all">Todas as Máquinas Combinadas</option>' +
      state.clients.map(c => `
        <option value="${c.ip}" ${c.ip === state.selectedClientForMatrix ? 'selected' : ''}>
          ${escapeHtml(c.name)} (${c.ip})
        </option>
      `).join('');

    if (!state.selectedClientForMatrix && state.clients.length > 0) {
      state.selectedClientForMatrix = state.clients[0].ip;
      select.value = state.selectedClientForMatrix;
    }

    loadReportMatrix();
  } catch (err) {
    console.warn(err);
  }
}

async function loadReportMatrix() {
  const tbody = document.getElementById('tbodyReportMatrix');
  const summaryStrip = document.getElementById('matrixClientSummary');
  tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">Consultando histórico do dispositivo...</td></tr>';

  try {
    const res = await fetch(`${API_BASE}/api/v1/reports/client-activity?client_ip=${encodeURIComponent(state.selectedClientForMatrix)}&days=${state.reportDays}&limit=100`);
    if (!res.ok) throw new Error();
    const items = await res.json();

    const clientObj = state.clients.find(c => c.ip === state.selectedClientForMatrix);
    const clientName = clientObj ? clientObj.name : state.selectedClientForMatrix;

    summaryStrip.innerHTML = `
      <span>Dispositivo: <strong>${escapeHtml(clientName)}</strong></span>
      <span>Período: <strong>Últimos ${state.reportDays} dias</strong></span>
      <span>Total de domínios distintos acessados: <strong>${items ? items.length : 0}</strong></span>
    `;

    if (!items || items.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">Nenhum acesso registrado para esta máquina no período selecionado.</td></tr>';
      return;
    }

    tbody.innerHTML = items.map(item => `
      <tr>
        <td><strong>${escapeHtml(item.domain)}</strong></td>
        <td>${item.total_queries.toLocaleString()}</td>
        <td class="${item.blocked_queries > 0 ? 'text-red' : 'text-muted'}">${item.blocked_queries.toLocaleString()}</td>
        <td>
          <button class="btn btn-secondary btn-sm" onclick="quickBlockDomain('${item.domain}')">Bloquear</button>
        </td>
      </tr>
    `).join('');
  } catch (err) {
    tbody.innerHTML = '<tr><td colspan="4" class="text-center text-red">Erro ao carregar dados do dispositivo.</td></tr>';
  }
}

function inspectClientMatrix(clientIP) {
  state.selectedClientForMatrix = clientIP;
  document.querySelector('.report-subtab[data-report="machine-matrix"]').click();
}

// ==========================================================
// Log em Tempo Real
// ==========================================================
function setupLiveLogControls() {
  document.getElementById('btnRefreshLogs').addEventListener('click', loadLiveLogs);
  document.getElementById('selectLogStatus').addEventListener('change', loadLiveLogs);
  document.getElementById('inputSearchDomain').addEventListener('input', debounce(loadLiveLogs, 300));
}

async function loadLiveLogs() {
  const tbody = document.getElementById('tbodyLiveLogs');
  const search = document.getElementById('inputSearchDomain').value.trim();
  const status = document.getElementById('selectLogStatus').value;

  try {
    const res = await fetch(`${API_BASE}/api/v1/logs?limit=50&domain=${encodeURIComponent(search)}&status=${status}`);
    if (!res.ok) return;
    const logs = await res.json();

    if (!logs || logs.length === 0) {
      tbody.innerHTML = '<tr><td colspan="8" class="text-center text-muted">Nenhuma consulta no filtro atual.</td></tr>';
      return;
    }

    tbody.innerHTML = logs.map(l => {
      const d = new Date(l.timestamp);
      const timeStr = `${d.getHours().toString().padStart(2,'0')}:${d.getMinutes().toString().padStart(2,'0')}:${d.getSeconds().toString().padStart(2,'0')}`;
      const isBlocked = l.status === 'blocked';
      return `
        <tr>
          <td><span class="text-dim">${timeStr}</span></td>
          <td><code>${l.client_ip}</code></td>
          <td><span class="text-dim">${l.qtype}</span></td>
          <td><strong>${escapeHtml(l.domain)}</strong></td>
          <td><span class="tag-badge ${isBlocked ? 'blocked' : 'allowed'}">${isBlocked ? 'Bloqueado' : 'Permitido'}</span></td>
          <td><small class="text-dim">${l.rule_matched || l.upstream}</small></td>
          <td><small class="text-dim">${l.latency_ms.toFixed(1)}ms</small></td>
          <td>
            ${isBlocked ? `
              <button class="btn btn-secondary btn-sm" onclick="quickAllowDomain('${l.domain}')">Liberar</button>
            ` : `
              <button class="btn btn-danger btn-sm" onclick="quickBlockDomain('${l.domain}')">Bloquear</button>
            `}
          </td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    console.warn(err);
  }
}

// ==========================================================
// Gerenciamento de Clientes (Dispositivos)
// ==========================================================
function setupClientModal() {
  const modal = document.getElementById('clientModal');
  document.getElementById('btnCloseClientModal').addEventListener('click', () => modal.classList.add('hidden'));

  document.getElementById('btnSaveClient').addEventListener('click', async () => {
    const ip = document.getElementById('modalClientIP').value;
    const name = document.getElementById('modalClientName').value.trim();
    const comment = document.getElementById('modalClientComment').value.trim();
    const groupId = parseInt(document.getElementById('modalClientGroup').value, 10) || 1;

    await fetch(`${API_BASE}/api/v1/clients`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ip, name, comment, block_enabled: true, group_id: groupId }),
    });

    modal.classList.add('hidden');
    loadClients();
    loadGroups();
  });

  // Modal de Novo Cliente Remoto (Token / Nuvem)
  const remoteModal = document.getElementById('newRemoteClientModal');
  const btnOpenRemote = document.getElementById('btnOpenNewRemoteClientModal');
  const btnCloseRemote = document.getElementById('btnCloseNewRemoteModal');
  const btnDoneRemote = document.getElementById('btnDoneRemoteModal');
  const btnSubmitRemote = document.getElementById('btnSubmitNewRemote');

  if (btnOpenRemote) {
    btnOpenRemote.addEventListener('click', () => {
      document.getElementById('remoteCreateForm').classList.remove('hidden');
      document.getElementById('remoteResultSection').classList.add('hidden');
      document.getElementById('inputRemoteClientName').value = '';
      document.getElementById('inputRemoteClientComment').value = '';
      remoteModal.classList.remove('hidden');
    });
  }

  if (btnCloseRemote) btnCloseRemote.addEventListener('click', () => remoteModal.classList.add('hidden'));
  if (btnDoneRemote) btnDoneRemote.addEventListener('click', () => remoteModal.classList.add('hidden'));

  if (btnSubmitRemote) {
    btnSubmitRemote.addEventListener('click', async () => {
      const name = document.getElementById('inputRemoteClientName').value.trim();
      const comment = document.getElementById('inputRemoteClientComment').value.trim();
      const groupId = parseInt(document.getElementById('inputRemoteClientGroup').value, 10) || 1;
      if (!name) {
        alert('Informe o nome do aparelho');
        return;
      }

      try {
        const res = await fetch(`${API_BASE}/api/v1/clients/remote`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, comment, group_id: groupId }),
        });
        const client = await res.json();

        const origin = window.location.origin;
        const dohUrl = `${origin}/dns-query/${client.auth_token}`;
        const agentCmd = `coorpdns-agent.exe -server ${origin} -token ${client.auth_token}`;
        const psInstallCmd = `irm ${origin}/install.ps1?token=${client.auth_token} | iex`;
        const msiInstallCmd = `msiexec /i "${origin}/downloads/coorpdns-agent.msi" TOKEN="${client.auth_token}" /qn`;

        document.getElementById('displayRemoteToken').value = client.auth_token;
        document.getElementById('displayRemoteDoHUrl').value = dohUrl;
        document.getElementById('displayAgentCommand').value = agentCmd;

        const elPs = document.getElementById('displayPsInstallCommand');
        if (elPs) elPs.value = psInstallCmd;

        const elMsi = document.getElementById('displayMsiInstallCommand');
        if (elMsi) elMsi.value = msiInstallCmd;

        const btnMsi = document.getElementById('btnDownloadMsi');
        if (btnMsi) btnMsi.href = `${origin}/downloads/coorpdns-agent.msi`;

        const btnExe = document.getElementById('btnDownloadExe');
        if (btnExe) btnExe.href = `${origin}/downloads/coorpdns-agent.exe`;

        document.getElementById('remoteCreateForm').classList.add('hidden');
        document.getElementById('remoteResultSection').classList.remove('hidden');

        loadClients();
        loadGroups();
      } catch (err) {
        alert('Erro ao gerar cliente remoto: ' + err.message);
      }
    });
  }

  // Ações de Cópia dos Comandos de Instalação
  const btnCopyPs = document.getElementById('btnCopyPsInstall');
  if (btnCopyPs) {
    btnCopyPs.addEventListener('click', () => {
      const input = document.getElementById('displayPsInstallCommand');
      if (input && input.value) {
        navigator.clipboard.writeText(input.value).then(() => {
          btnCopyPs.textContent = '✅ Copiado!';
          setTimeout(() => { btnCopyPs.textContent = '📋 Copiar'; }, 2000);
        });
      }
    });
  }

  const btnCopyMsi = document.getElementById('btnCopyMsiInstall');
  if (btnCopyMsi) {
    btnCopyMsi.addEventListener('click', () => {
      const input = document.getElementById('displayMsiInstallCommand');
      if (input && input.value) {
        navigator.clipboard.writeText(input.value).then(() => {
          btnCopyMsi.textContent = '✅ Copiado!';
          setTimeout(() => { btnCopyMsi.textContent = '📋 Copiar'; }, 2000);
        });
      }
    });
  }
}

// Gerenciamento e Carregamento de Grupos
function setupGroupControls() {
  const modal = document.getElementById('groupManagerModal');
  const btnOpen = document.getElementById('btnOpenGroupManagerModal');
  const btnClose = document.getElementById('btnCloseGroupManagerModal');
  const btnCreate = document.getElementById('btnCreateGroup');

  if (btnOpen) btnOpen.addEventListener('click', () => modal.classList.remove('hidden'));
  if (btnClose) btnClose.addEventListener('click', () => modal.classList.add('hidden'));

  if (btnCreate) {
    btnCreate.addEventListener('click', async () => {
      const name = document.getElementById('inputNewGroupName').value.trim();
      const color = document.getElementById('inputNewGroupColor').value;
      const description = document.getElementById('inputNewGroupDesc').value.trim();

      if (!name) {
        alert('Digite o nome do grupo');
        return;
      }

      try {
        await fetch(`${API_BASE}/api/v1/groups`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, description, color }),
        });
        document.getElementById('inputNewGroupName').value = '';
        document.getElementById('inputNewGroupDesc').value = '';
        loadGroups();
      } catch (err) {
        alert('Erro ao criar grupo: ' + err.message);
      }
    });
  }
}

async function loadGroups() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/groups`);
    if (!res.ok) return;
    const groups = await res.json();
    state.groups = groups || [];

    // 1. Popula seletor do Relatório por Grupo
    const selectReport = document.getElementById('selectReportGroup');
    if (selectReport) {
      selectReport.innerHTML = state.groups.map(g => `
        <option value="${g.id}" ${g.id === state.selectedGroupForReport ? 'selected' : ''}>
          ${escapeHtml(g.name)} (${g.client_count} aparelhos)
        </option>
      `).join('');

      if (!state.selectedGroupForReport && state.groups.length > 0) {
        state.selectedGroupForReport = state.groups[0].id;
      }
    }

    // 2. Popula seletores dos modais de edição e criação
    const modalClientGroup = document.getElementById('modalClientGroup');
    if (modalClientGroup) {
      modalClientGroup.innerHTML = state.groups.map(g => `
        <option value="${g.id}">${escapeHtml(g.name)}</option>
      `).join('');
    }

    const inputRemoteGroup = document.getElementById('inputRemoteClientGroup');
    if (inputRemoteGroup) {
      inputRemoteGroup.innerHTML = state.groups.map(g => `
        <option value="${g.id}">${escapeHtml(g.name)}</option>
      `).join('');
    }

    // 3. Popula lista de grupos existentes no modal de gerenciamento
    const listContainer = document.getElementById('groupsListContainer');
    if (listContainer) {
      listContainer.innerHTML = state.groups.map(g => `
        <div class="group-item-row">
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span class="group-dot" style="background: ${g.color};"></span>
            <div>
              <strong>${escapeHtml(g.name)}</strong>
              ${g.description ? `<br><small class="text-dim">${escapeHtml(g.description)}</small>` : ''}
            </div>
          </div>
          <span class="tag-badge" style="background: rgba(255,255,255,0.06);">${g.client_count} dispositivos</span>
        </div>
      `).join('');
    }

    // Se estiver na aba de relatório por grupo, atualiza a visão
    if (state.currentTab === 'reports' && state.currentReportSubtab === 'group-report') {
      loadGroupReport();
    }
  } catch (err) {
    console.warn(err);
  }
}

async function loadClients() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/clients`);
    if (!res.ok) return;
    const clients = await res.json();
    state.clients = clients || [];

    const tbody = document.getElementById('tbodyClientsList');
    if (!clients || clients.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="text-center text-muted">Nenhum dispositivo cadastrado ainda.</td></tr>';
      return;
    }

    tbody.innerHTML = clients.map(c => {
      const isRemote = c.is_remote;
      const isRevoked = c.status === 'revoked';
      const typeBadge = isRemote 
        ? `<span class="tag-badge allowed">Nuvem (DoH)</span>` 
        : `<span class="tag-badge" style="background:rgba(56,189,248,0.15); color:var(--accent-blue)">Wi-Fi Local</span>`;
      
      const groupBadge = `
        <span class="group-badge">
          <span class="group-dot" style="background: ${c.group_color || '#64748b'};"></span>
          ${escapeHtml(c.group_name || 'Geral / Padrão')}
        </span>
      `;

      const tokenDisplay = isRemote 
        ? (isRevoked 
            ? `<span class="tag-badge blocked">Revogado</span>` 
            : `<code>${c.auth_token ? c.auth_token.slice(0, 8) + '...' : 'Ativo'}</code>`) 
        : `<span class="text-dim">Rede Local</span>`;

      return `
        <tr>
          <td>${typeBadge}</td>
          <td>${groupBadge}</td>
          <td><strong>${escapeHtml(c.name)}</strong></td>
          <td><code>${c.ip}</code></td>
          <td>${tokenDisplay}</td>
          <td>${escapeHtml(c.comment || '-')}</td>
          <td>
            <div style="display: flex; gap: 0.35rem;">
              <button class="btn btn-secondary btn-sm" onclick="openEditClient('${c.ip}', '${escapeHtml(c.name)}', '${escapeHtml(c.comment)}', ${c.group_id || 1})">
                Editar
              </button>
              ${isRemote && !isRevoked ? `
                <button class="btn btn-danger btn-sm" onclick="revokeRemoteClient('${c.auth_token}')">
                  Revogar
                </button>
              ` : ''}
            </div>
          </td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    console.warn(err);
  }
}

async function revokeRemoteClient(token) {
  if (!confirm('Deseja realmente revogar o acesso deste dispositivo remoto? Ele não conseguirá mais resolver nomes.')) return;
  try {
    await fetch(`${API_BASE}/api/v1/clients/revoke`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    });
    loadClients();
  } catch (err) {
    alert('Erro ao revogar: ' + err.message);
  }
}

function openEditClient(ip, name, comment, groupId) {
  document.getElementById('modalClientIP').value = ip;
  document.getElementById('modalClientName').value = name;
  document.getElementById('modalClientComment').value = comment;
  const groupSelect = document.getElementById('modalClientGroup');
  if (groupSelect) {
    groupSelect.value = groupId || 1;
  }
  document.getElementById('clientModal').classList.remove('hidden');
}

// ==========================================================
// Regras e Listas
// ==========================================================
function setupRulesForm() {
  document.getElementById('btnAddBlockRule').addEventListener('click', async () => {
    const rule = document.getElementById('inputNewRule').value.trim();
    if (!rule) return;
    await fetch(`${API_BASE}/api/v1/rules`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'add_block', rule }),
    });
    document.getElementById('inputNewRule').value = '';
    loadRules();
    refreshStatus();
  });

  document.getElementById('btnAddAllowRule').addEventListener('click', async () => {
    const rule = document.getElementById('inputNewRule').value.trim();
    if (!rule) return;
    await fetch(`${API_BASE}/api/v1/rules`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action: 'add_allow', rule }),
    });
    document.getElementById('inputNewRule').value = '';
    loadRules();
    refreshStatus();
  });
}

async function loadRules() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/rules`);
    if (!res.ok) return;
    const data = await res.json();

    const ulBlock = document.getElementById('ulBlockRules');
    const ulAllow = document.getElementById('ulAllowRules');

    if (!data.custom_rules || data.custom_rules.length === 0) {
      ulBlock.innerHTML = '<li class="text-muted">Nenhuma regra de bloqueio customizada.</li>';
    } else {
      ulBlock.innerHTML = data.custom_rules.map(r => `<li><code>${escapeHtml(r)}</code></li>`).join('');
    }

    if (!data.allow_rules || data.allow_rules.length === 0) {
      ulAllow.innerHTML = '<li class="text-muted">Nenhuma exceção configurada.</li>';
    } else {
      ulAllow.innerHTML = data.allow_rules.map(r => `<li><code>${escapeHtml(r)}</code></li>`).join('');
    }
  } catch (err) {
    console.warn(err);
  }
}

async function loadLists() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/lists`);
    if (!res.ok) return;
    const lists = await res.json();
    const container = document.getElementById('listsContainer');

    container.innerHTML = lists.map(l => `
      <div class="step-card" style="margin-bottom: 0.75rem;">
        <div style="display: flex; justify-content: space-between; align-items: center;">
          <div>
            <h4 style="margin: 0; font-size: 0.95rem;">${escapeHtml(l.name)}</h4>
            <small class="text-dim">${escapeHtml(l.url)}</small>
          </div>
          <span class="badge-status ${l.enabled ? 'online' : 'offline'}">${l.enabled ? 'Ativa' : 'Pausada'}</span>
        </div>
      </div>
    `).join('');
  } catch (err) {
    console.warn(err);
  }
}

async function quickBlockDomain(domain) {
  await fetch(`${API_BASE}/api/v1/rules`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'add_block', rule: `||${domain}^` }),
  });
  alert(`Domínio "${domain}" adicionado à lista de bloqueio!`);
  loadRules();
  refreshStatus();
  if (state.currentTab === 'reports') loadReportsData();
}

async function quickAllowDomain(domain) {
  await fetch(`${API_BASE}/api/v1/rules`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'add_allow', rule: `@@||${domain}^` }),
  });
  alert(`Domínio "${domain}" adicionado à lista de exceções permitidas!`);
  loadRules();
  refreshStatus();
  if (state.currentTab === 'reports') loadReportsData();
}

// ==========================================================
// Utilitários
// ==========================================================
function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'
  }[m]));
}

function debounce(fn, delay) {
  let timer;
  return function(...args) {
    clearTimeout(timer);
    timer = setTimeout(() => fn.apply(this, args), delay);
  };
}

// ==========================================================
// Controles de Políticas de Categorias por Grupo
// ==========================================================
function setupCategoryControls() {
  const modal = document.getElementById('groupCategoriesModal');
  const btnOpen = document.getElementById('btnOpenGroupCategoriesModal');
  const btnClose = document.getElementById('btnCloseGroupCategoriesModal');
  const selectGroup = document.getElementById('selectCategoryModalGroup');
  const btnSave = document.getElementById('btnSaveCategories');
  const btnBlockAll = document.getElementById('btnCategoryBlockAll');
  const btnAllowAll = document.getElementById('btnCategoryAllowAll');

  if (btnOpen) {
    btnOpen.addEventListener('click', async () => {
      populateCategoryGroupSelect();
      await loadCategories();
      await loadGroupCategoryRules(state.currentCategoryGroup);
      modal.classList.remove('hidden');
    });
  }

  if (btnClose) {
    btnClose.addEventListener('click', () => modal.classList.add('hidden'));
  }

  if (selectGroup) {
    selectGroup.addEventListener('change', async (e) => {
      state.currentCategoryGroup = parseInt(e.target.value, 10);
      await loadGroupCategoryRules(state.currentCategoryGroup);
    });
  }

  if (btnBlockAll) {
    btnBlockAll.addEventListener('click', () => {
      state.availableCategories.forEach(cat => {
        state.groupCategoryRules[cat.id] = 'block';
      });
      renderCategoryCards();
    });
  }

  if (btnAllowAll) {
    btnAllowAll.addEventListener('click', () => {
      state.availableCategories.forEach(cat => {
        state.groupCategoryRules[cat.id] = 'allow';
      });
      renderCategoryCards();
    });
  }

  if (btnSave) {
    btnSave.addEventListener('click', saveGroupCategoryRules);
  }
}

function populateCategoryGroupSelect() {
  const select = document.getElementById('selectCategoryModalGroup');
  if (!select) return;
  select.innerHTML = state.groups.map(g => `
    <option value="${g.id}" ${g.id === state.currentCategoryGroup ? 'selected' : ''}>
      ${escapeHtml(g.name)} (${g.client_count || 0} dispositivos)
    </option>
  `).join('');
}

async function loadCategories() {
  if (state.availableCategories && state.availableCategories.length > 0) return;
  try {
    const res = await fetch(`${API_BASE}/api/v1/categories`);
    state.availableCategories = await res.json();
  } catch (err) {
    console.error('Erro ao carregar categorias:', err);
  }
}

async function loadGroupCategoryRules(groupId) {
  try {
    const res = await fetch(`${API_BASE}/api/v1/groups/categories?group_id=${groupId}`);
    const data = await res.json();
    state.groupCategoryRules = data.rules || {};
    renderCategoryCards();
  } catch (err) {
    console.error('Erro ao carregar regras de categorias do grupo:', err);
  }
}

function renderCategoryCards() {
  const container = document.getElementById('containerCategoryCards');
  if (!container) return;

  container.innerHTML = state.availableCategories.map(cat => {
    const action = state.groupCategoryRules[cat.id] || 'allow';
    const isBlocked = action === 'block';

    return `
      <div class="category-card ${isBlocked ? 'blocked' : 'allowed'}">
        <div class="category-info">
          <div class="category-icon-box" style="background: ${cat.color}20; color: ${cat.color};">
            <i class="${cat.icon || 'fas fa-shield-alt'}"></i>
          </div>
          <div>
            <span class="category-text-title">${escapeHtml(cat.name)}</span>
            <span class="category-text-desc">${escapeHtml(cat.description)}</span>
          </div>
        </div>
        <div>
          <button class="category-toggle-btn ${isBlocked ? 'btn-blocked' : 'btn-allowed'}" 
                  onclick="toggleCategoryAction('${cat.id}')">
            ${isBlocked ? '🚫 Bloqueado' : '✅ Permitido'}
          </button>
        </div>
      </div>
    `;
  }).join('');
}

function toggleCategoryAction(catId) {
  const current = state.groupCategoryRules[catId] || 'allow';
  state.groupCategoryRules[catId] = current === 'block' ? 'allow' : 'block';
  renderCategoryCards();
}

async function saveGroupCategoryRules() {
  const statusSpan = document.getElementById('categorySaveStatus');
  if (statusSpan) statusSpan.textContent = 'Salvando...';

  try {
    const res = await fetch(`${API_BASE}/api/v1/groups/categories`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        group_id: state.currentCategoryGroup,
        rules: state.groupCategoryRules
      })
    });
    if (res.ok) {
      if (statusSpan) {
        statusSpan.textContent = '✅ Políticas salvas com sucesso para o grupo!';
        setTimeout(() => { if (statusSpan) statusSpan.textContent = ''; }, 3500);
      }
    }
  } catch (err) {
    if (statusSpan) statusSpan.textContent = '❌ Erro ao salvar políticas';
    console.error(err);
  }
}

// ==========================================================
// Controles de Integração Active Directory (AD) & Encaminhamento Condicional
// ==========================================================
function setupActiveDirectoryControls() {
  const modal = document.getElementById('activeDirectoryModal');
  const btnOpen = document.getElementById('btnOpenActiveDirectoryModal');
  const btnClose = document.getElementById('btnCloseActiveDirectoryModal');
  const btnAdd = document.getElementById('btnAddAdForwarder');

  if (btnOpen) {
    btnOpen.addEventListener('click', async () => {
      await loadAdForwarders();
      modal.classList.remove('hidden');
    });
  }

  if (btnClose) {
    btnClose.addEventListener('click', () => modal.classList.add('hidden'));
  }

  if (btnAdd) {
    btnAdd.addEventListener('click', addAdForwarder);
  }
}

async function loadAdForwarders() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/forwarders`);
    state.adForwarders = await res.json();
    renderAdForwarders();
  } catch (err) {
    console.error('Erro ao carregar encaminhadores AD:', err);
  }
}

function renderAdForwarders() {
  const tbody = document.getElementById('tbodyAdForwarders');
  if (!tbody) return;

  if (!state.adForwarders || state.adForwarders.length === 0) {
    tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted" style="padding: 1.5rem;">Nenhuma zona de AD ou domínio local configurado. Adicione acima para encaminhar ao seu Domain Controller.</td></tr>';
    return;
  }

  tbody.innerHTML = state.adForwarders.map(f => `
    <tr>
      <td><strong>${escapeHtml(f.domain)}</strong></td>
      <td><span class="mono">${escapeHtml(f.upstream_ip)}</span></td>
      <td><span class="text-muted">${escapeHtml(f.comment || 'Controlador de Domínio')}</span></td>
      <td><span class="badge-status online">Encaminhando</span></td>
      <td>
        <button class="btn btn-danger btn-xs" onclick="deleteAdForwarder(${f.id})">Remover</button>
      </td>
    </tr>
  `).join('');
}

async function addAdForwarder() {
  const domain = document.getElementById('inputAdDomain').value.trim();
  const ip = document.getElementById('inputAdIp').value.trim();
  const comment = document.getElementById('inputAdComment').value.trim();

  if (!domain || !ip) {
    alert('Informe o domínio da zona interna (ex: empresa.local) e o IP do Domain Controller!');
    return;
  }

  try {
    const res = await fetch(`${API_BASE}/api/v1/forwarders`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ domain, upstream_ip: ip, comment })
    });
    if (res.ok) {
      document.getElementById('inputAdDomain').value = '';
      document.getElementById('inputAdIp').value = '';
      document.getElementById('inputAdComment').value = '';
      await loadAdForwarders();
    } else {
      alert('Erro ao salvar encaminhador condicional.');
    }
  } catch (err) {
    console.error(err);
  }
}

async function deleteAdForwarder(id) {
  if (!confirm('Deseja realmente remover este encaminhador condicional? As máquinas podem perder a resolução do AD se este for o único DC!')) return;
  try {
    await fetch(`${API_BASE}/api/v1/forwarders/${id}`, { method: 'DELETE' });
    await loadAdForwarders();
  } catch (err) {
    console.error(err);
  }
}

// ==========================================================
// Controles de Configuração da Página de Bloqueio & Suporte
// ==========================================================
function setupBlockPageSettingsControls() {
  const modal = document.getElementById('blockPageSettingsModal');
  const btnOpen = document.getElementById('btnOpenBlockPageSettingsModal');
  const btnClose = document.getElementById('btnCloseBlockPageSettingsModal');
  const btnSave = document.getElementById('btnSaveSupportConfig');

  if (btnOpen) {
    btnOpen.addEventListener('click', async () => {
      await loadSupportConfig();
      modal.classList.remove('hidden');
    });
  }

  if (btnClose) {
    btnClose.addEventListener('click', () => modal.classList.add('hidden'));
  }

  if (btnSave) {
    btnSave.addEventListener('click', saveSupportConfig);
  }
}

async function loadSupportConfig() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/config/support`);
    const cfg = await res.json();
    if (cfg.block_mode) document.getElementById('inputConfigBlockMode').value = cfg.block_mode;
    if (cfg.server_ip) document.getElementById('inputConfigServerIp').value = cfg.server_ip;
    if (cfg.org_name) document.getElementById('inputConfigOrgName').value = cfg.org_name;
    if (cfg.support_email) document.getElementById('inputConfigSupportEmail').value = cfg.support_email;
    if (cfg.support_phone) document.getElementById('inputConfigSupportPhone').value = cfg.support_phone;
  } catch (err) {
    console.error('Erro ao carregar configurações de suporte:', err);
  }
}

async function saveSupportConfig() {
  const blockMode = document.getElementById('inputConfigBlockMode').value;
  const serverIp = document.getElementById('inputConfigServerIp').value.trim();
  const orgName = document.getElementById('inputConfigOrgName').value.trim();
  const supportEmail = document.getElementById('inputConfigSupportEmail').value.trim();
  const supportPhone = document.getElementById('inputConfigSupportPhone').value.trim();

  try {
    const res = await fetch(`${API_BASE}/api/v1/config/support`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        block_mode: blockMode,
        server_ip: serverIp,
        org_name: orgName,
        support_email: supportEmail,
        support_phone: supportPhone
      })
    });
    if (res.ok) {
      alert('Configurações da Página de Bloqueio e Suporte salvas com sucesso!');
      document.getElementById('blockPageSettingsModal').classList.add('hidden');
    } else {
      alert('Erro ao salvar configurações.');
    }
  } catch (err) {
    console.error(err);
  }
}


