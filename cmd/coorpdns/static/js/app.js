/**
 * CoorpDNS - Frontend Application Logic (Vanilla JS ES6+)
 */

const API_BASE = ''; // Relativo ao host atual

// Estado Global da Aplicação
const state = {
  currentTab: 'dashboard',
  currentReportSubtab: 'top-sites',
  currentPolicyTab: 'applications',
  currentPolicyGroup: 1,
  catalogApplications: [],
  groupAppRules: {},
  groupSchedule: { enabled: true, mode: 'partial_block', slots: [] },
  filterOnlyBlockedApps: false,
  searchAppQuery: '',
  reportDays: 30,
  domainStatusFilter: 'all',
  selectedClientForMatrix: '',
  selectedGroupForReport: 1,
  groups: [],
  clients: [],
  users: [],
  currentUser: null,
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
  setupAuthControls();
  setupUserControls();
  setupReportsControls();
  setupLiveLogControls();
  setupClientModal();
  setupGroupControls();
  setupCategoryControls();
  setupActiveDirectoryControls();
  setupBlockPageSettingsControls();
  setupMasterToggle();
  setupRulesForm();
  setupPoliciesControls();
  setupSidebarSearch();

  // Validação de autenticação inicial
  checkAuth();

  // Atualização periódica automática
  setInterval(() => {
    if (state.currentUser) refreshStatus();
  }, 3000);
  setInterval(() => {
    if (state.currentUser && state.currentTab === 'dashboard') refreshDashboard();
  }, 10000);
  setInterval(() => {
    if (state.currentUser && state.currentTab === 'logs') {
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
    policies: ['Políticas & Grupos', 'Gerenciamento de aplicações, regras de categorias e agendamento semanal de funcionamento'],
    reports: ['Relatórios Históricos (180 Dias)', 'Auditoria completa de consumo por máquina, sites mais acessados e tendências'],
    logs: ['Log de Consultas DNS', 'Acompanhamento em tempo real das requisições DNS processadas na sua rede'],
    clients: ['Gerenciamento de Dispositivos', 'Nomeie e personalize os aparelhos conectados para facilitar o monitoramento'],
    rules: ['Regras & Listas de Bloqueio', 'Gerencie listas comunitárias e adicione regras de bloqueio ou exceção personalizadas'],
    users: ['Gerenciamento de Usuários & Acessos', 'Cadastre operadores e defina permissões por grupos de dispositivos'],
    guide: ['Guia de Configuração do Roteador', 'Como ativar a proteção em toda a sua residência com poucos cliques'],
  };

  if (titles[tabId]) {
    document.getElementById('pageTitle').textContent = titles[tabId][0];
    document.getElementById('pageSubtitle').textContent = titles[tabId][1];
  }

  // Dispara carregamento de dados da aba selecionada
  if (tabId === 'policies') {
    loadPoliciesSection();
  } else if (tabId === 'reports') {
    loadReportsData();
  } else if (tabId === 'logs') {
    loadLiveLogs();
  } else if (tabId === 'clients') {
    loadClients();
  } else if (tabId === 'users') {
    if (!state.groups || state.groups.length === 0) loadGroups();
    loadUsers();
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
// Dashboard (Métricas e Gráficos Modernos)
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

    // Atualiza novos componentes visuais avançados
    renderHeroMonitoringChart();
    renderDeviceDonut();
    loadDashboardTopWidgets();
    loadDashboardGroupsSummary();

    // Atualiza mini tabelas rápidas
    loadQuickDomains();
    loadQuickClients();
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
    const res = await fetch(`${API_BASE}/api/v1/reports/timeline?hours=24`);
    if (!res.ok) return;
    let points = await res.json();
    const svg = document.getElementById('timelineSvg');
    if (!svg) return;

    if (!points || points.length === 0) {
      svg.innerHTML = '<text x="450" y="120" text-anchor="middle" fill="#64748b" font-size="13">Aguardando dados de consultas DNS...</text>';
      return;
    }

    const width = 900;
    const height = 240;
    const paddingLeft = 45;
    const paddingRight = 20;
    const paddingTop = 25;
    const paddingBottom = 35;
    const plotWidth = width - paddingLeft - paddingRight;
    const plotHeight = height - paddingTop - paddingBottom;

    const maxQueries = Math.max(...points.map(p => p.total_queries), 5);
    // Margem superior de 15% para as curvas não baterem no topo
    const maxVal = Math.ceil(maxQueries * 1.15);
    const stepX = plotWidth / Math.max(points.length - 1, 1);

    const totalCoords = [];
    const blockedCoords = [];
    let circleNodes = '';

    points.forEach((p, idx) => {
      const x = paddingLeft + idx * stepX;
      const yTotal = paddingTop + plotHeight - (p.total_queries / maxVal) * plotHeight;
      const yBlocked = paddingTop + plotHeight - (p.blocked_queries / maxVal) * plotHeight;
      totalCoords.push({ x, y: yTotal, q: p.total_queries, date: p.date });
      blockedCoords.push({ x, y: yBlocked, q: p.blocked_queries, date: p.date });

      if (p.total_queries > 0) {
        circleNodes += `
          <circle cx="${x}" cy="${yTotal}" r="4" fill="#10b981" stroke="#090d16" stroke-width="2">
            <title>${p.date} - Total: ${p.total_queries} consultas</title>
          </circle>
        `;
      }
      if (p.blocked_queries > 0) {
        circleNodes += `
          <circle cx="${x}" cy="${yBlocked}" r="3.5" fill="#ef4444" stroke="#090d16" stroke-width="2">
            <title>${p.date} - Bloqueadas: ${p.blocked_queries}</title>
          </circle>
        `;
      }
    });

    const baselineY = paddingTop + plotHeight;
    const totalLine = totalCoords.map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(' ');
    const totalArea = `${totalLine} L ${totalCoords[totalCoords.length - 1].x.toFixed(1)},${baselineY} L ${totalCoords[0].x.toFixed(1)},${baselineY} Z`;

    const blockedLine = blockedCoords.map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x.toFixed(1)},${c.y.toFixed(1)}`).join(' ');
    const blockedArea = `${blockedLine} L ${blockedCoords[blockedCoords.length - 1].x.toFixed(1)},${baselineY} L ${blockedCoords[0].x.toFixed(1)},${baselineY} Z`;

    // Rótulos do Eixo X (a cada 3 ou 4 horas)
    let xLabels = '';
    const labelStep = Math.max(Math.floor(points.length / 7), 1);
    points.forEach((p, idx) => {
      if (idx % labelStep === 0 || idx === points.length - 1) {
        const x = paddingLeft + idx * stepX;
        xLabels += `<text x="${x}" y="${height - 10}" text-anchor="middle" fill="#64748b" font-size="11" font-family="sans-serif">${p.date}</text>`;
      }
    });

    // Rótulos do Eixo Y (Máximo, Médio, Zero)
    const midVal = Math.round(maxVal / 2);
    const midY = paddingTop + plotHeight / 2;

    svg.innerHTML = `
      <defs>
        <linearGradient id="gradEmerald" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#10b981" stop-opacity="0.35"/>
          <stop offset="100%" stop-color="#10b981" stop-opacity="0.0"/>
        </linearGradient>
        <linearGradient id="gradRed" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#ef4444" stop-opacity="0.25"/>
          <stop offset="100%" stop-color="#ef4444" stop-opacity="0.0"/>
        </linearGradient>
      </defs>

      <!-- Linhas de Grade Horizontais -->
      <line x1="${paddingLeft}" y1="${paddingTop}" x2="${width - paddingRight}" y2="${paddingTop}" stroke="rgba(255,255,255,0.06)" stroke-dasharray="4 4" />
      <text x="${paddingLeft - 8}" y="${paddingTop + 4}" text-anchor="end" fill="#64748b" font-size="10">${maxVal}</text>

      <line x1="${paddingLeft}" y1="${midY}" x2="${width - paddingRight}" y2="${midY}" stroke="rgba(255,255,255,0.06)" stroke-dasharray="4 4" />
      <text x="${paddingLeft - 8}" y="${midY + 4}" text-anchor="end" fill="#64748b" font-size="10">${midVal}</text>

      <line x1="${paddingLeft}" y1="${baselineY}" x2="${width - paddingRight}" y2="${baselineY}" stroke="rgba(255,255,255,0.12)" />
      <text x="${paddingLeft - 8}" y="${baselineY + 4}" text-anchor="end" fill="#64748b" font-size="10">0</text>

      <!-- Áreas Preenchidas -->
      <path d="${totalArea}" fill="url(#gradEmerald)"/>
      <path d="${blockedArea}" fill="url(#gradRed)"/>

      <!-- Linhas das Curvas -->
      <path d="${totalLine}" fill="none" stroke="#10b981" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/>
      <path d="${blockedLine}" fill="none" stroke="#ef4444" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>

      <!-- Pontos Interativos com Tooltip -->
      ${circleNodes}

      <!-- Rótulos de Horário (Eixo X) -->
      ${xLabels}
    `;
  } catch (err) {
    console.warn('Erro ao renderizar gráfico:', err);
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
      const isLinux = dev.os_type === 'linux';
      const osBadge = isLinux
        ? `<span class="badge" style="background: rgba(234, 179, 8, 0.12); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3); font-size: 0.72rem; padding: 0.2rem 0.5rem;">🐧 Linux</span>`
        : `<span class="badge" style="background: rgba(59, 130, 246, 0.12); color: #60a5fa; border: 1px solid rgba(59, 130, 246, 0.3); font-size: 0.72rem; padding: 0.2rem 0.5rem;">🪟 Windows</span>`;

      const typeBadge = dev.is_remote
        ? '<span class="tag-badge allowed">Nuvem DoH</span>'
        : '<span class="tag-badge" style="background:rgba(56,189,248,0.15); color:var(--accent-blue)">Wi-Fi Local</span>';

      const categoriesHtml = (!dev.categories || dev.categories.length === 0)
        ? '<p class="text-muted" style="font-size: 0.85rem; padding: 0.75rem 0; text-align: center;">Nenhuma atividade registrada para este aparelho no período selecionado.</p>'
        : `
          <div class="device-categories-list">
            ${dev.categories.map((cat, catIdx) => {
              const catBlockId = `dev-${idx}-cat-${catIdx}`;
              const isBlocked = cat.blocked_queries > 0;
              return `
                <div class="device-category-block" style="border-left: 3.5px solid ${cat.color || '#3b82f6'};">
                  <div class="device-category-header" onclick="toggleDeviceCategory('${catBlockId}')">
                    <div class="device-category-title">
                      <span style="font-size: 1.15rem;">${cat.icon || '📁'}</span>
                      <span style="color: ${cat.color || 'var(--text-main)'}; font-weight: 600;">${escapeHtml(cat.category_name)}</span>
                      <span class="badge" style="background: rgba(255, 255, 255, 0.08); font-size: 0.72rem; padding: 0.15rem 0.45rem; border-radius: 4px; color: var(--text-muted);">${cat.domains_count} ${cat.domains_count === 1 ? 'site' : 'sites'}</span>
                    </div>
                    <div class="device-category-stats">
                      <span style="color: var(--text-dim);">${cat.total_queries.toLocaleString()} acessos</span>
                      ${isBlocked ? `
                        <span class="text-red" style="font-weight: 600; background: rgba(239, 68, 68, 0.12); padding: 0.15rem 0.5rem; border-radius: 4px; font-size: 0.78rem;">
                          🛡️ ${cat.blocked_queries.toLocaleString()} bloqueados
                        </span>
                      ` : ''}
                      <span id="chevron-${catBlockId}" style="font-size: 0.75rem; color: var(--text-dim); transition: transform 0.2s;">▼</span>
                    </div>
                  </div>
                  <div id="${catBlockId}" class="device-category-domains">
                    <div class="table-container compact" style="margin: 0;">
                      <table class="data-table">
                        <thead>
                          <tr>
                            <th>Domínio Solicitado</th>
                            <th>Consultas</th>
                            <th>Bloqueadas</th>
                            <th style="text-align: right;">Ação Rápida</th>
                          </tr>
                        </thead>
                        <tbody>
                          ${(cat.domains || []).map(d => `
                            <tr>
                              <td><strong>${escapeHtml(d.domain)}</strong></td>
                              <td>${d.total_queries.toLocaleString()}</td>
                              <td class="${d.blocked_queries > 0 ? 'text-red font-semibold' : 'text-muted'}">
                                ${d.blocked_queries > 0 ? `🛡️ ${d.blocked_queries.toLocaleString()}` : '0'}
                              </td>
                              <td style="text-align: right;">
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
                  </div>
                </div>
              `;
            }).join('')}
          </div>
        `;

      return `
        <div class="group-device-card" style="border-left: 4px solid ${data.group_color || '#10b981'};">
          <div class="group-device-header" onclick="toggleGroupDeviceCard('group-dev-body-${idx}')">
            <div class="group-device-meta">
              <span style="font-weight: 700; font-size: 1.05rem;">${escapeHtml(dev.client_name)}</span>
              ${typeBadge}
              ${dev.is_remote ? osBadge : ''}
              <code style="font-size: 0.8rem; color: var(--text-dim);">${dev.client_ip}</code>
            </div>
            <div class="group-device-stats">
              <span><strong>${dev.total_queries.toLocaleString()}</strong> consultas</span>
              <span class="${dev.blocked_queries > 0 ? 'text-red' : 'text-muted'}"><strong>${dev.blocked_queries.toLocaleString()}</strong> bloqueadas (${dev.block_rate.toFixed(0)}%)</span>
              <button class="btn btn-secondary btn-sm" style="pointer-events: none;">
                Categorias (${dev.categories ? dev.categories.length : 0}) ▼
              </button>
            </div>
          </div>
          <div id="group-dev-body-${idx}" class="group-device-domains-container">
            ${categoriesHtml}
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

window.toggleDeviceCategory = function(id) {
  const el = document.getElementById(id);
  const chevron = document.getElementById(`chevron-${id}`);
  if (el) {
    el.classList.toggle('hidden');
    if (chevron) {
      chevron.style.transform = el.classList.contains('hidden') ? 'rotate(-90deg)' : 'rotate(0deg)';
    }
  }
};

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

  // Interação de Seleção de SO (Windows vs Linux)
  const radioWin = document.getElementById('radioOsWindows');
  const radioLin = document.getElementById('radioOsLinux');
  const labelWin = document.getElementById('labelOsWindows');
  const labelLin = document.getElementById('labelOsLinux');

  function updateOsOptionCards() {
    if (radioLin && radioLin.checked) {
      if (labelLin) {
        labelLin.style.borderColor = '#eab308';
        labelLin.style.background = 'rgba(234, 179, 8, 0.12)';
      }
      if (labelWin) {
        labelWin.style.borderColor = 'var(--border-subtle)';
        labelWin.style.background = 'var(--bg-card)';
      }
    } else {
      if (labelWin) {
        labelWin.style.borderColor = '#3b82f6';
        labelWin.style.background = 'rgba(59, 130, 246, 0.1)';
      }
      if (labelLin) {
        labelLin.style.borderColor = 'var(--border-subtle)';
        labelLin.style.background = 'var(--bg-card)';
      }
    }
  }

  if (radioWin) radioWin.addEventListener('change', updateOsOptionCards);
  if (radioLin) radioLin.addEventListener('change', updateOsOptionCards);

  // Tabs do resultado (Windows vs Linux)
  const tabWin = document.getElementById('tabResultWindows');
  const tabLin = document.getElementById('tabResultLinux');
  const blockWin = document.getElementById('blockHeroWindows');
  const blockLin = document.getElementById('blockHeroLinux');

  function switchResultTab(os) {
    if (os === 'linux') {
      if (tabLin) {
        tabLin.style.background = '#ca8a04';
        tabLin.style.color = '#fff';
        tabLin.style.border = 'none';
      }
      if (tabWin) {
        tabWin.style.background = 'rgba(255,255,255,0.05)';
        tabWin.style.color = 'var(--text-muted)';
        tabWin.style.border = '1px solid var(--border-subtle)';
      }
      if (blockLin) blockLin.classList.remove('hidden');
      if (blockWin) blockWin.classList.add('hidden');
    } else {
      if (tabWin) {
        tabWin.style.background = '#2563eb';
        tabWin.style.color = '#fff';
        tabWin.style.border = 'none';
      }
      if (tabLin) {
        tabLin.style.background = 'rgba(255,255,255,0.05)';
        tabLin.style.color = 'var(--text-muted)';
        tabLin.style.border = '1px solid var(--border-subtle)';
      }
      if (blockWin) blockWin.classList.remove('hidden');
      if (blockLin) blockLin.classList.add('hidden');
    }
  }

  if (tabWin) tabWin.addEventListener('click', () => switchResultTab('windows'));
  if (tabLin) tabLin.addEventListener('click', () => switchResultTab('linux'));

  if (btnOpenRemote) {
    btnOpenRemote.addEventListener('click', () => {
      document.getElementById('remoteCreateForm').classList.remove('hidden');
      document.getElementById('remoteResultSection').classList.add('hidden');
      document.getElementById('inputRemoteClientName').value = '';
      document.getElementById('inputRemoteClientComment').value = '';
      if (radioWin) radioWin.checked = true;
      updateOsOptionCards();
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
      const osType = (radioLin && radioLin.checked) ? 'linux' : 'windows';

      if (!name) {
        alert('Informe o nome do aparelho');
        return;
      }

      try {
        const res = await fetch(`${API_BASE}/api/v1/clients/remote`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, comment, group_id: groupId, os_type: osType }),
        });
        const client = await res.json();

        const origin = window.location.origin;
        const dohUrl = `${origin}/dns-query/${client.auth_token}`;
        const agentCmd = osType === 'linux'
          ? `coorpdns-agent-linux -server ${origin} -token ${client.auth_token}`
          : `coorpdns-agent.exe -server ${origin} -token ${client.auth_token}`;
        const psInstallCmd = `irm ${origin}/install.ps1?token=${client.auth_token} | iex`;
        const linuxInstallCmd = `curl -fsSL ${origin}/install.sh?token=${client.auth_token} | sudo bash`;
        const msiInstallCmd = `msiexec /i "${origin}/downloads/coorpdns-agent.msi" TOKEN="${client.auth_token}" /qn`;

        document.getElementById('displayRemoteToken').value = client.auth_token;
        document.getElementById('displayRemoteDoHUrl').value = dohUrl;
        document.getElementById('displayAgentCommand').value = agentCmd;

        const elPs = document.getElementById('displayPsInstallCommand');
        if (elPs) elPs.value = psInstallCmd;

        const elLinux = document.getElementById('displayLinuxInstallCommand');
        if (elLinux) elLinux.value = linuxInstallCmd;

        const elMsi = document.getElementById('displayMsiInstallCommand');
        if (elMsi) elMsi.value = msiInstallCmd;

        const btnMsi = document.getElementById('btnDownloadMsi');
        if (btnMsi) btnMsi.href = `${origin}/downloads/coorpdns-agent.msi`;

        const btnExe = document.getElementById('btnDownloadExe');
        if (btnExe) btnExe.href = `${origin}/downloads/coorpdns-agent.exe`;

        const btnLinux = document.getElementById('btnDownloadLinux');
        if (btnLinux) btnLinux.href = `${origin}/downloads/coorpdns-agent-linux`;

        switchResultTab(osType);

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

  const btnCopyLinux = document.getElementById('btnCopyLinuxInstall');
  if (btnCopyLinux) {
    btnCopyLinux.addEventListener('click', () => {
      const input = document.getElementById('displayLinuxInstallCommand');
      if (input && input.value) {
        navigator.clipboard.writeText(input.value).then(() => {
          btnCopyLinux.textContent = '✅ Copiado!';
          setTimeout(() => { btnCopyLinux.textContent = '📋 Copiar'; }, 2000);
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
          <td>
            <strong>${escapeHtml(c.name)}</strong>
            ${isRemote ? (c.os_type === 'linux' 
              ? `<span class="badge" style="background:rgba(234,179,8,0.12); color:#facc15; border:1px solid rgba(234,179,8,0.3); font-size:0.72rem; padding:0.15rem 0.45rem; margin-left:0.35rem;">🐧 Linux</span>`
              : `<span class="badge" style="background:rgba(59,130,246,0.12); color:#60a5fa; border:1px solid rgba(59,130,246,0.3); font-size:0.72rem; padding:0.15rem 0.45rem; margin-left:0.35rem;">🪟 Windows</span>`) : ''}
          </td>
          <td><code>${c.ip}</code></td>
          <td>${tokenDisplay}</td>
          <td>${escapeHtml(c.comment || '-')}</td>
          <td>
            <div style="display: flex; gap: 0.35rem;">
              ${isRemote && !isRevoked ? (c.os_type === 'linux' ? `
                <button class="btn btn-sm" style="background: rgba(234, 179, 8, 0.15); color: #facc15; border: 1px solid rgba(234, 179, 8, 0.3); font-weight: 600;" onclick="copyLinuxInstallCommand('${c.auth_token}')" title="Copiar comando Linux Bash para instalar neste PC">
                  🐧 Instalar
                </button>
              ` : `
                <button class="btn btn-sm" style="background: rgba(59, 130, 246, 0.15); color: #60a5fa; border: 1px solid rgba(59, 130, 246, 0.3); font-weight: 600;" onclick="copyPsInstallCommand('${c.auth_token}')" title="Copiar comando PowerShell Admin para instalar neste PC">
                  ⚡ Instalar
                </button>
              `) : ''}
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

window.copyPsInstallCommand = function(token) {
  if (!token) return;
  const origin = window.location.origin;
  const cmd = `irm ${origin}/install.ps1?token=${token} | iex`;
  navigator.clipboard.writeText(cmd).then(() => {
    alert('Comando PowerShell copiado com sucesso!\n\nAbra o PowerShell como Administrador e execute:\n\n' + cmd);
  }).catch(() => {
    prompt('Copie o comando de instalação para executar no PowerShell como Administrador:', cmd);
  });
};

window.copyLinuxInstallCommand = function(token) {
  if (!token) return;
  const origin = window.location.origin;
  const cmd = `curl -fsSL ${origin}/install.sh?token=${token} | sudo bash`;
  navigator.clipboard.writeText(cmd).then(() => {
    alert('Comando Linux copiado com sucesso!\n\nAbra o terminal como root ou use sudo e execute:\n\n' + cmd);
  }).catch(() => {
    prompt('Copie o comando de instalação para executar no terminal Linux (sudo):', cmd);
  });
};


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

// ==========================================================
// Autenticação & Sessão (Login, Logout, RBAC)
// ==========================================================
async function checkAuth() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/auth/me`);
    if (res.ok) {
      const user = await res.json();
      onAuthenticated(user);
      return true;
    } else {
      showLoginScreen();
      return false;
    }
  } catch (err) {
    showLoginScreen();
    return false;
  }
}

function showLoginScreen() {
  state.currentUser = null;
  const loginOverlay = document.getElementById('loginOverlay');
  if (loginOverlay) {
    loginOverlay.classList.remove('hidden');
    const alertBox = document.getElementById('loginAlert');
    if (alertBox) alertBox.classList.add('hidden');
    const passInput = document.getElementById('loginPassword');
    if (passInput) passInput.value = '';
    setTimeout(() => {
      const userInput = document.getElementById('loginUsername');
      if (userInput) userInput.focus();
    }, 100);
  }
}

function onAuthenticated(user) {
  state.currentUser = user;
  const loginOverlay = document.getElementById('loginOverlay');
  if (loginOverlay) loginOverlay.classList.add('hidden');

  // Atualiza barra de sessão do usuário na sidebar
  const userDisplayName = document.getElementById('userDisplayName');
  const userDisplayRole = document.getElementById('userDisplayRole');
  const userAvatar = document.getElementById('userAvatar');
  if (userDisplayName) userDisplayName.textContent = user.full_name || user.username;
  if (userDisplayRole) {
    userDisplayRole.textContent = user.role === 'admin' ? 'Admin' : 'Operador';
    userDisplayRole.className = `user-role-badge ${user.role === 'admin' ? 'role-admin' : 'role-operator'}`;
  }
  if (userAvatar) {
    const initial = (user.full_name || user.username || 'U').charAt(0).toUpperCase();
    userAvatar.textContent = initial;
  }

  // Visibilidade de menus e controles de acordo com o papel (RBAC)
  const navItemUsers = document.getElementById('navItemUsers');
  const navItemRules = document.getElementById('navItemRules');
  const toggleGroup = document.querySelector('.toggle-control-group');

  if (user.role === 'admin') {
    if (navItemUsers) navItemUsers.style.display = 'flex';
    if (navItemRules) navItemRules.style.display = 'flex';
    if (toggleGroup) toggleGroup.style.display = 'flex';
  } else {
    if (navItemUsers) navItemUsers.style.display = 'none';
    if (navItemRules) navItemRules.style.display = 'none';
    if (toggleGroup) toggleGroup.style.display = 'none';
    if (state.currentTab === 'users' || state.currentTab === 'rules') {
      switchTab('dashboard');
    }
  }

  // Carrega dados iniciais
  refreshStatus();
  refreshDashboard();
  loadGroups();
  loadClients();
  if (user.role === 'admin') {
    loadRules();
    loadLists();
    loadUsers();
  }
}

function setupAuthControls() {
  const loginForm = document.getElementById('loginForm');
  const loginAlert = document.getElementById('loginAlert');
  const btnLoginSubmit = document.getElementById('btnLoginSubmit');
  const btnLogout = document.getElementById('btnLogout');

  if (loginForm) {
    loginForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      if (loginAlert) loginAlert.classList.add('hidden');
      if (btnLoginSubmit) {
        btnLoginSubmit.disabled = true;
        btnLoginSubmit.innerHTML = '<span>Entrando...</span>';
      }

      const username = document.getElementById('loginUsername').value.trim();
      const password = document.getElementById('loginPassword').value;

      try {
        const res = await fetch(`${API_BASE}/api/v1/auth/login`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username, password })
        });
        if (res.ok) {
          const data = await res.json();
          onAuthenticated(data.user);
        } else {
          const errText = await res.text();
          if (loginAlert) {
            loginAlert.textContent = errText || 'Credenciais inválidas. Verifique usuário e senha.';
            loginAlert.classList.remove('hidden');
          }
        }
      } catch (err) {
        if (loginAlert) {
          loginAlert.textContent = 'Erro de conexão com o servidor.';
          loginAlert.classList.remove('hidden');
        }
      } finally {
        if (btnLoginSubmit) {
          btnLoginSubmit.disabled = false;
          btnLoginSubmit.innerHTML = `<span>Entrar no Sistema</span><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 18px; height: 18px; flex-shrink: 0;"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>`;
        }
      }
    });
  }

  if (btnLogout) {
    btnLogout.addEventListener('click', async () => {
      if (!confirm('Deseja realmente encerrar sua sessão?')) return;
      try {
        await fetch(`${API_BASE}/api/v1/auth/logout`, { method: 'POST' });
      } catch (_) {}
      showLoginScreen();
    });
  }
}

// ==========================================================
// Gerenciamento de Usuários (RBAC)
// ==========================================================
function setupUserControls() {
  const modal = document.getElementById('modalUser');
  const btnOpen = document.getElementById('btnOpenNewUserModal');
  const btnClose = document.getElementById('btnCloseUserModal');
  const btnCancel = document.getElementById('btnCancelUserModal');
  const form = document.getElementById('formUser');
  const roleSelect = document.getElementById('userRole');

  if (btnOpen) {
    btnOpen.addEventListener('click', async () => {
      await openUserModal();
    });
  }

  if (btnClose) btnClose.addEventListener('click', () => modal.classList.add('hidden'));
  if (btnCancel) btnCancel.addEventListener('click', () => modal.classList.add('hidden'));

  if (roleSelect) {
    roleSelect.addEventListener('change', () => {
      const container = document.getElementById('groupSelectionContainer');
      const countEl = document.getElementById('groupSelectionCount');
      if (roleSelect.value === 'admin') {
        container.style.opacity = '0.5';
        container.style.pointerEvents = 'none';
        if (countEl) countEl.textContent = 'Todos os grupos (Admin)';
      } else {
        container.style.opacity = '1';
        container.style.pointerEvents = 'auto';
        updateGroupSelectionCount();
      }
    });
  }

  if (form) {
    form.addEventListener('submit', async (e) => {
      e.preventDefault();
      const id = document.getElementById('userFormId').value;
      const fullName = document.getElementById('userFullName').value.trim();
      const username = document.getElementById('userUsername').value.trim();
      const password = document.getElementById('userPassword').value;
      const role = document.getElementById('userRole').value;

      if (!fullName || !username) {
        alert('Nome completo e login são obrigatórios.');
        return;
      }
      if (!id && !password) {
        alert('A senha é obrigatória para novos usuários.');
        return;
      }

      // Coleta grupos marcados
      const checkedGroups = [];
      document.querySelectorAll('#userGroupsList input[type="checkbox"]:checked').forEach(cb => {
        checkedGroups.push(parseInt(cb.value, 10));
      });

      if (role === 'operator' && checkedGroups.length === 0) {
        if (!confirm('Este operador não terá acesso a nenhum grupo. Deseja continuar mesmo assim?')) {
          return;
        }
      }

      const payload = {
        full_name: fullName,
        username: username,
        password: password,
        role: role,
        group_ids: checkedGroups
      };

      try {
        let res;
        if (id) {
          res = await fetch(`${API_BASE}/api/v1/users/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
        } else {
          res = await fetch(`${API_BASE}/api/v1/users`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
          });
        }

        if (res.ok) {
          modal.classList.add('hidden');
          loadUsers();
        } else {
          const err = await res.text();
          alert('Erro ao salvar usuário: ' + err);
        }
      } catch (err) {
        alert('Erro de conexão ao salvar usuário.');
      }
    });
  }
}

function updateGroupSelectionCount() {
  const count = document.querySelectorAll('#userGroupsList input[type="checkbox"]:checked').length;
  const countEl = document.getElementById('groupSelectionCount');
  if (countEl) {
    countEl.textContent = `${count} selecionado${count === 1 ? '' : 's'}`;
  }
}

async function openUserModal(user = null) {
  const modal = document.getElementById('modalUser');
  if (!modal) return;
  const title = document.getElementById('modalUserTitle');
  const formId = document.getElementById('userFormId');
  const fullName = document.getElementById('userFullName');
  const username = document.getElementById('userUsername');
  const password = document.getElementById('userPassword');
  const passwordHint = document.getElementById('userPasswordHint');
  const roleSelect = document.getElementById('userRole');
  const groupsList = document.getElementById('userGroupsList');
  const container = document.getElementById('groupSelectionContainer');

  // Garante que grupos estejam carregados
  if (!state.groups || state.groups.length === 0) {
    await loadGroups();
  }

  // Renderiza checklist de grupos disponíveis no sistema
  if (groupsList) {
    groupsList.innerHTML = (state.groups || []).map(g => {
      const isChecked = user && user.allowed_groups && user.allowed_groups.some(gid => gid == g.id);
      return `
        <label class="group-checkbox-item">
          <input type="checkbox" value="${g.id}" ${isChecked ? 'checked' : ''} onchange="updateGroupSelectionCount()">
          <div class="group-checkbox-content">
            <span class="group-checkbox-name">
              <span class="group-checkbox-color-dot" style="background: ${g.color};"></span>
              ${escapeHtml(g.name)}
            </span>
            <span class="tag-badge" style="font-size: 0.7rem; background: rgba(255,255,255,0.06);">${g.client_count || 0} dispositivos</span>
          </div>
        </label>
      `;
    }).join('');
  }

  if (user) {
    title.textContent = 'Editar Usuário: ' + (user.full_name || user.username);
    formId.value = user.id;
    fullName.value = user.full_name || '';
    username.value = user.username || '';
    password.value = '';
    password.required = false;
    passwordHint.style.display = 'block';
    roleSelect.value = user.role || 'operator';
  } else {
    title.textContent = 'Cadastrar Novo Usuário';
    formId.value = '';
    fullName.value = '';
    username.value = '';
    password.value = '';
    password.required = true;
    passwordHint.style.display = 'none';
    roleSelect.value = 'operator';
  }

  if (roleSelect.value === 'admin') {
    container.style.opacity = '0.5';
    container.style.pointerEvents = 'none';
    const countEl = document.getElementById('groupSelectionCount');
    if (countEl) countEl.textContent = 'Todos os grupos (Admin)';
  } else {
    container.style.opacity = '1';
    container.style.pointerEvents = 'auto';
    updateGroupSelectionCount();
  }

  modal.classList.remove('hidden');
}

async function loadUsers() {
  if (!state.currentUser || state.currentUser.role !== 'admin') return;
  try {
    const res = await fetch(`${API_BASE}/api/v1/users`);
    if (!res.ok) return;
    const users = await res.json();
    state.users = users || [];

    const tbody = document.getElementById('tbodyUsers');
    if (!tbody) return;

    if (!users || users.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Nenhum usuário cadastrado.</td></tr>';
      return;
    }

    tbody.innerHTML = users.map(u => {
      const isAdmin = u.role === 'admin';
      const roleBadge = isAdmin
        ? `<span class="user-role-badge role-admin">Administrador</span>`
        : `<span class="user-role-badge role-operator">Operador</span>`;

      let groupsHtml = '';
      if (isAdmin) {
        groupsHtml = `<span class="tag-badge" style="background: rgba(16, 185, 129, 0.15); color: var(--accent-emerald);">Todos os Grupos (Acesso Total)</span>`;
      } else if (!u.allowed_groups || u.allowed_groups.length === 0) {
        groupsHtml = `<span class="text-dim" style="font-size: 0.82rem;">Nenhum grupo atribuído</span>`;
      } else {
        groupsHtml = u.allowed_groups.map(gid => {
          const g = state.groups.find(group => group.id === gid);
          const name = g ? g.name : `Grupo #${gid}`;
          const color = g ? g.color : '#64748b';
          return `<span class="group-pill"><span class="group-checkbox-color-dot" style="background: ${color};"></span>${escapeHtml(name)}</span>`;
        }).join('');
      }

      const createdDate = u.created_at ? new Date(u.created_at).toLocaleDateString('pt-BR') : '-';
      const canDelete = u.id !== 1 && u.id !== state.currentUser.id;

      return `
        <tr>
          <td><strong>${escapeHtml(u.username)}</strong></td>
          <td>${escapeHtml(u.full_name)}</td>
          <td>${roleBadge}</td>
          <td>${groupsHtml}</td>
          <td><span class="text-dim">${createdDate}</span></td>
          <td style="text-align: right;">
            <div style="display: flex; gap: 0.35rem; justify-content: flex-end;">
              <button class="btn btn-secondary btn-sm" onclick="editUserById(${u.id})">Editar</button>
              ${canDelete ? `
                <button class="btn btn-danger btn-sm" onclick="deleteUserById(${u.id}, '${escapeHtml(u.username)}')">Excluir</button>
              ` : ''}
            </div>
          </td>
        </tr>
      `;
    }).join('');
  } catch (err) {
    console.warn('Erro ao carregar usuários:', err);
  }
}

window.editUserById = async function(id) {
  const user = (state.users || []).find(u => u.id == id);
  if (user) {
    await openUserModal(user);
  }
};

window.openUserModal = openUserModal;
window.updateGroupSelectionCount = updateGroupSelectionCount;

window.deleteUserById = async function(id, username) {
  if (!confirm(`Deseja realmente excluir o usuário "${username}"?`)) return;
  try {
    const res = await fetch(`${API_BASE}/api/v1/users/${id}`, { method: 'DELETE' });
    if (res.ok) {
      loadUsers();
    } else {
      const err = await res.text();
      alert('Erro ao excluir usuário: ' + err);
    }
  } catch (err) {
    alert('Erro de conexão ao excluir usuário.');
  }
};

// ==========================================================
// SVGs & Ícones Autênticos de Aplicações (Emil Kowalski Design)
// ==========================================================
function getAppIconSvg(appId, name, color = '#3b82f6', domains = []) {
  const icons = {
    // Messaging & Collaboration
    whatsapp: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M12.04 2c-5.46 0-9.91 4.45-9.91 9.91 0 1.75.46 3.45 1.32 4.95L2.05 22l5.25-1.38c1.45.79 3.08 1.21 4.74 1.21 5.46 0 9.91-4.45 9.91-9.91 0-2.65-1.03-5.14-2.9-7.01A9.82 9.82 0 0 0 12.04 2m.01 1.67c2.2 0 4.26.86 5.82 2.42a8.23 8.23 0 0 1 2.41 5.83c0 4.54-3.7 8.24-8.24 8.24-1.48 0-2.93-.4-4.2-1.15l-.3-.18-3.12.82.83-3.04-.2-.31a8.19 8.19 0 0 1-1.26-4.38c0-4.54 3.7-8.24 8.24-8.24m4.52 11.66c-.25-.13-1.47-.72-1.7-.81-.23-.08-.39-.13-.56.13-.17.25-.64.81-.79.97-.14.17-.29.19-.54.06-.25-.13-1.06-.39-2.02-1.24-.75-.67-1.26-1.49-1.41-1.74-.14-.25-.02-.39.11-.51.11-.11.25-.29.37-.44.13-.14.17-.25.25-.42.08-.17.04-.31-.02-.44-.06-.13-.56-1.35-.77-1.85-.2-.49-.41-.42-.56-.43h-.48c-.17 0-.44.06-.67.31-.23.25-.88.86-.88 2.1 0 1.24.9 2.44 1.03 2.61.13.17 1.78 2.72 4.31 3.81.6.26 1.07.42 1.44.53.61.19 1.16.17 1.6.1.49-.07 1.47-.6 1.68-1.18.21-.58.21-1.08.14-1.18-.06-.1-.22-.16-.47-.28"/></svg>`,
    telegram: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="m20.665 3.717-17.73 6.837c-1.21.486-1.203 1.161-.222 1.462l4.552 1.42 10.532-6.645c.498-.303.953-.14.579.192l-8.533 7.701h-.002l-.313 4.67c.458 0 .66-.21.916-.457l2.199-2.138 4.574 3.379c.843.464 1.45.224 1.66-.782l3.003-14.156c.307-1.23-.468-1.787-1.215-1.483z"/></svg>`,
    discord: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M20.317 4.37a19.791 19.791 0 0 0-4.885-1.515.074.074 0 0 0-.079.037c-.21.375-.444.864-.608 1.25a18.27 18.27 0 0 0-5.487 0 12.64 12.64 0 0 0-.617-1.25.077.077 0 0 0-.079-.037A19.736 19.736 0 0 0 3.677 4.37a.07.07 0 0 0-.032.027C.533 9.046-.32 13.58.099 18.057a.082.082 0 0 0 .031.057 19.9 19.9 0 0 0 5.993 3.03.078.078 0 0 0 .084-.028c.462-.63.874-1.295 1.226-1.994.021-.041.001-.09-.041-.106a13.107 13.107 0 0 1-1.872-.892.077.077 0 0 1-.008-.128 10.2 10.2 0 0 0 .372-.292.074.074 0 0 1 .077-.01c3.929 1.793 8.18 1.793 12.061 0a.074.074 0 0 1 .078.01c.12.098.246.198.373.292a.077.077 0 0 1-.006.127 12.299 12.299 0 0 1-1.873.894.077.077 0 0 0-.041.107c.36.698.772 1.362 1.225 1.993a.076.076 0 0 0 .084.028 19.839 19.839 0 0 0 6.002-3.03.077.077 0 0 0 .032-.054c.5-5.177-.838-9.674-3.549-13.66a.061.061 0 0 0-.031-.028zM8.02 15.33c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.956-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.956 2.418-2.157 2.418zm7.975 0c-1.183 0-2.157-1.085-2.157-2.419 0-1.333.955-2.419 2.157-2.419 1.21 0 2.176 1.096 2.157 2.42 0 1.333-.946 2.418-2.157 2.418z"/></svg>`,
    teams: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M19.5 7.5a2 2 0 1 1-4 0 2 2 0 0 1 4 0M17.5 10c-.7 0-1.34.24-1.85.64A4.5 4.5 0 0 1 18 14.5v3.5h3.5c.83 0 1.5-.67 1.5-1.5v-3c0-1.93-1.57-3.5-3.5-3.5M10.5 4a3 3 0 1 1-6 0 3 3 0 0 1 6 0M12 11c0-2.48-2.02-4.5-4.5-4.5S3 8.52 3 11v6c0 .83.67 1.5 1.5 1.5h6c.83 0 1.5-.67 1.5-1.5z"/></svg>`,
    outlook: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="5" width="20" height="14" rx="2"/><path d="m2 7 10 7 10-7"/></svg>`,
    skype: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="16.5" cy="7.5" r="4"/><circle cx="7.5" cy="16.5" r="4"/><path d="M12 6a6 6 0 0 0-6 6c0 .7.12 1.36.35 1.98A6 6 0 0 0 18 12a6 6 0 0 0-6-6z"/></svg>`,
    zoom: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="6" width="12" height="12" rx="2"/><path d="m15 10 6-4v12l-6-4z"/></svg>`,

    // Microsoft & Office
    office_365: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="3" width="8" height="8" rx="1"/><rect x="13" y="3" width="8" height="8" rx="1"/><rect x="3" y="13" width="8" height="8" rx="1"/><rect x="13" y="13" width="8" height="8" rx="1"/></svg>`,
    azure: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M19 18H6a4 4 0 0 1-.5-7.9A6 6 0 0 1 17 8a4.5 4.5 0 0 1 2 10z"/></svg>`,
    bing: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="m5 3 5 2v11l5-3-2-2-4 1V6l7 3-1 12-10-6z"/></svg>`,
    edge: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="9" fill="none" stroke="#fff" stroke-width="2"/><path d="M12 7a5 5 0 0 1 5 5c0 3-2 5-5 5s-6-2-6-5a5 5 0 0 1 6-5z" fill="#fff"/></svg>`,
    copilot: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="8" fill="none" stroke="#fff" stroke-width="2"/><path d="M8 12h8M12 8v8" stroke="#fff" stroke-width="2"/></svg>`,

    // AI
    chatgpt: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M22.28 9.87a5.98 5.98 0 0 0-.52-4.94 6.05 6.05 0 0 0-6.51-2.82A6.06 6.06 0 0 0 10.6.49 6.05 6.05 0 0 0 5.2 3.42a6.05 6.05 0 0 0-4.13 4.14 6.06 6.06 0 0 0 1.45 6.37 5.98 5.98 0 0 0 .52 4.94 6.05 6.05 0 0 0 6.51 2.82 6.06 6.06 0 0 0 4.65 1.62 6.05 6.05 0 0 0 5.4-2.93 6.05 6.05 0 0 0 4.13-4.14 6.06 6.06 0 0 0-1.45-6.37"/></svg>`,
    claude_ai: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="4"/><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>`,
    character_ai: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="3" width="18" height="18" rx="4" fill="#111"/><text x="12" y="16" text-anchor="middle" font-size="12" font-weight="900" fill="#fff" font-family="sans-serif">[c]</text></svg>`,
    copy_ai: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M16 4H8a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2m-4 11a3 3 0 1 1 0-6 3 3 0 0 1 0 6"/></svg>`,
    dall_e: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="9" fill="none" stroke="#fff" stroke-width="2"/><circle cx="9" cy="10" r="1.5"/><circle cx="15" cy="10" r="1.5"/><circle cx="12" cy="15" r="1.5"/></svg>`,

    // Banks
    itau: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="3" width="18" height="18" rx="4" fill="#ea580c"/><text x="12" y="15" text-anchor="middle" font-size="9" font-weight="900" fill="#002d72" font-family="sans-serif">ITAU</text></svg>`,
    bradesco: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="9" fill="#dc2626"/><path d="M12 7v10M8 10h8" stroke="#fff" stroke-width="2"/></svg>`,
    banco_do_brasil: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#facc15"><rect x="2" y="2" width="20" height="20" rx="3" fill="#003882"/><text x="12" y="16" text-anchor="middle" font-size="11" font-weight="900" fill="#facc15" font-family="sans-serif">BB</text></svg>`,
    banco_inter: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#f97316"/><text x="12" y="15" text-anchor="middle" font-size="7.5" font-weight="900" fill="#fff" font-family="sans-serif">inter</text></svg>`,
    banco_pan: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0284c7"/><text x="12" y="16" text-anchor="middle" font-size="9" font-weight="900" fill="#fff" font-family="sans-serif">PAN</text></svg>`,
    banco_safra: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#facc15"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0a1931"/><text x="12" y="15" text-anchor="middle" font-size="7" font-weight="900" fill="#facc15" font-family="serif">SAFRA</text></svg>`,
    nubank: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#820ad1"/><text x="12" y="16" text-anchor="middle" font-size="11" font-weight="900" fill="#fff" font-family="sans-serif">nu</text></svg>`,
    santander: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#ec0000"/><path d="M12 5c-2.5 4-5 6.5-5 9.5a5 5 0 0 0 10 0c0-3-2.5-5.5-5-9.5z" fill="#fff"/></svg>`,

    // Betting
    bet365: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="3" width="20" height="18" rx="3" fill="#006b52"/><text x="12" y="15" text-anchor="middle" font-size="8" font-weight="900" fill="#facc15" font-family="sans-serif">365</text></svg>`,
    betano: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="9" fill="#f95900"/><text x="12" y="16" text-anchor="middle" font-size="11" font-weight="900" fill="#fff" font-family="sans-serif">B</text></svg>`,
    betfair: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#f59e0b"><rect x="2" y="2" width="20" height="20" rx="4" fill="#111"/><path d="m6 15 4-4 4 4M18 9l-4 4-4-4" stroke="#f59e0b" stroke-width="2.5" fill="none"/></svg>`,
    '1xbet': `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#105a96"/><text x="12" y="16" text-anchor="middle" font-size="10" font-weight="900" fill="#fff" font-family="sans-serif">1X</text></svg>`,
    bet_da_sorte: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#16a34a"/><circle cx="12" cy="12" r="6" fill="#facc15"/><text x="12" y="15" text-anchor="middle" font-size="8" font-weight="900" fill="#000" font-family="sans-serif">★</text></svg>`,
    blaze: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#000"/><path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z" fill="#ef4444"/></svg>`,
    kto: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#dc2626"/><text x="12" y="15" text-anchor="middle" font-size="8" font-weight="900" fill="#fff" font-family="sans-serif">KTO</text></svg>`,
    pixbet: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#2563eb"/><path d="m12 6-5 5 5 5 5-5-5-5zm0 3 2.5 2.5L12 14 9.5 11.5 12 9z"/></svg>`,
    estrelabet: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#facc15"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0a1931"/><polygon points="12 4 14.5 9 20 9.8 16 13.8 17 19.2 12 16.5 7 19.2 8 13.8 4 9.8 9.5 9"/></svg>`,
    sportingbet: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0284c7"/><text x="12" y="16" text-anchor="middle" font-size="9" font-weight="900" fill="#fff" font-family="sans-serif">SB</text></svg>`,

    // Games
    steam: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M12 2a10 10 0 0 0-9.95 9l4.5 1.86a3 3 0 0 1 3.45-1.42l3.23-4.66A3.49 3.49 0 0 1 16.5 6a3.5 3.5 0 1 1-3.5 3.5l-4.66 3.23a3 3 0 0 1-1.42 3.45L8.78 20.68A10 10 0 1 0 12 2"/></svg>`,
    roblox: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="m18.5 5.5-13 3.5 3.5 13 13-3.5zm-6.5 8a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3"/></svg>`,
    among_us: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#c51111"/><path d="M12 5c-2.2 0-4 1.8-4 4v7h2v2h2v-2h2v2h2v-2h2V9c0-2.2-1.8-4-4-4zm3 5H10a1.5 1.5 0 0 1 0-3h5a1.5 1.5 0 0 1 0 3z" fill="#fff"/></svg>`,
    brawl_stars: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#facc15"><circle cx="12" cy="12" r="9" fill="#f59e0b"/><circle cx="9" cy="11" r="2" fill="#000"/><circle cx="15" cy="11" r="2" fill="#000"/><rect x="10" y="15" width="4" height="2" rx="1" fill="#000"/></svg>`,
    epic_games: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#111"/><polygon points="12 4 19 6 17 18 12 20 7 18 5 6" fill="none" stroke="#fff" stroke-width="2"/></svg>`,

    // Google
    google_agenda: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="4" width="18" height="16" rx="3" fill="#4285f4"/><text x="12" y="16" text-anchor="middle" font-size="9" font-weight="900" fill="#fff" font-family="sans-serif">31</text></svg>`,
    google_docs: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="4" y="3" width="16" height="18" rx="2" fill="#2563eb"/><path d="M8 8h8M8 12h8M8 16h5" stroke="#fff" stroke-width="2" stroke-linecap="round"/></svg>`,
    google_drive: `<svg viewBox="0 0 24 24" width="16" height="16"><polygon points="8 4 16 4 21 13 13 13" fill="#ffba00"/><polygon points="3 13 8 4 13 13 8 22" fill="#0066da"/><polygon points="8 22 13 13 21 13 16 22" fill="#00ac47"/></svg>`,
    google_fotos: `<svg viewBox="0 0 24 24" width="16" height="16"><path d="M12 2a4 4 0 0 1 4 4v2h-4a4 4 0 0 1-4-4 4 4 0 0 1 4-2z" fill="#ea4335"/><path d="M22 12a4 4 0 0 1-4 4h-2v-4a4 4 0 0 1 4-4 4 4 0 0 1 2 4z" fill="#fbbc05"/><path d="M12 22a4 4 0 0 1-4-4v-2h4a4 4 0 0 1 4 4 4 4 0 0 1-4 2z" fill="#34a853"/><path d="M2 12a4 4 0 0 1 4-4h2v4a4 4 0 0 1-4 4 4 4 0 0 1-2-4z" fill="#4285f4"/></svg>`,
    gemini: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#9333ea"><path d="M12 2C12 7.5 7.5 12 2 12C7.5 12 12 16.5 12 22C12 16.5 16.5 12 22 12C16.5 12 12 7.5 12 2Z" fill="#9333ea"/></svg>`,

    // Meta
    facebook: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M18 2h-3a5 5 0 0 0-5 5v3H7v4h3v8h4v-8h3l1-4h-4V7a1 1 0 0 1 1-1h3z"/></svg>`,
    instagram: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="6" fill="none" stroke="#fff" stroke-width="2"/><circle cx="12" cy="12" r="4" fill="none" stroke="#fff" stroke-width="2"/><circle cx="18" cy="6" r="1.5" fill="#fff"/></svg>`,
    messenger: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M12 2C6.5 2 2 6.1 2 11.2c0 2.9 1.4 5.5 3.6 7.2V22l3.4-1.9c1 .3 2 .4 3 .4 5.5 0 10-4.1 10-9.3S17.5 2 12 2zm1 12.3-3.2-3.4-6.3 3.4 6.9-7.3 3.3 3.4 6.2-3.5-6.9 7.4z"/></svg>`,
    threads: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="9" fill="none" stroke="#fff" stroke-width="2"/><text x="12" y="16" text-anchor="middle" font-size="12" font-weight="900" fill="#fff" font-family="sans-serif">@</text></svg>`,

    // Music & Streaming
    spotify: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><circle cx="12" cy="12" r="10"/><path d="M7 9c3-1 7-1 10 1M8 12c2.5-.8 5.5-.8 8 .8M9 15c2-.5 4.5-.5 6 .6"/></svg>`,
    deezer: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="3" y="13" width="2.5" height="6" rx="1"/><rect x="7" y="9" width="2.5" height="10" rx="1"/><rect x="11" y="5" width="2.5" height="14" rx="1"/><rect x="15" y="10" width="2.5" height="9" rx="1"/><rect x="19" y="14" width="2.5" height="5" rx="1"/></svg>`,
    apple_music: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><path d="M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm11-2a3 3 0 1 1-6 0 3 3 0 0 1 6 0z"/></svg>`,
    netflix: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#e50914"><rect x="3" y="3" width="18" height="18" rx="3" fill="#000"/><path d="M7 5v14l4-8v8h2V5l-4 8V5z" fill="#e50914"/></svg>`,
    youtube: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#ff0000"><rect x="2" y="4" width="20" height="16" rx="4" fill="#ff0000"/><polygon points="10 8 16 12 10 16 10 8" fill="#fff"/></svg>`,

    // Education
    alura: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0070f3"/><text x="12" y="16" text-anchor="middle" font-size="11" font-weight="900" fill="#fff" font-family="sans-serif">A</text></svg>`,
    coursera: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#0056d2"/><path d="M12 5a7 7 0 1 0 7 7h-3a4 4 0 1 1-4-4V5z" fill="#fff"/></svg>`,
    descomplica: `<svg viewBox="0 0 24 24" width="16" height="16" fill="#fff"><rect x="2" y="2" width="20" height="20" rx="4" fill="#00e676"/><text x="12" y="16" text-anchor="middle" font-size="11" font-weight="900" fill="#000" font-family="sans-serif">D</text></svg>`,
  };

  const primaryDomain = (domains && domains.length > 0) ? domains[0] : (appId.includes('.') ? appId : `${appId}.com`);
  const vectorSvg = icons[appId];

  // Se tivermos ícone vetorial nativo de alta fidelidade
  if (vectorSvg) {
    return `<div class="app-brand-icon" style="background: ${color};">${vectorSvg}</div>`;
  }

  // Se tiver domínio, carrega favicon oficial da empresa em alta resolução (64px) com fallback gracioso
  const initials = (name || appId).substring(0, 2).toUpperCase();
  const fallbackHtml = `<span style="font-size:0.75rem; font-weight:800; color:#fff;">${escapeHtml(initials)}</span>`;

  if (primaryDomain) {
    const faviconUrl = `https://www.google.com/s2/favicons?domain=${encodeURIComponent(primaryDomain)}&sz=64`;
    return `
      <div class="app-brand-icon" style="background: ${color}; position: relative; overflow: hidden; display: flex; align-items: center; justify-content: center;">
        <img src="${faviconUrl}" alt="${escapeHtml(name)}" loading="lazy"
             style="width: 18px; height: 18px; object-fit: contain; border-radius: 4px;"
             onerror="this.style.display='none'; if(this.nextElementSibling) this.nextElementSibling.style.display='flex';" />
        <div style="display: none; align-items: center; justify-content: center; width: 100%; height: 100%;">
          ${fallbackHtml}
        </div>
      </div>
    `;
  }

  return `<div class="app-brand-icon" style="background: ${color}; font-size: 0.75rem;">${fallbackHtml}</div>`;
}

// ==========================================================
// Dashboard: Gráfico de Monitoramento 24h & Donut de Dispositivos
// ==========================================================
async function renderHeroMonitoringChart() {
  const svg = document.getElementById('heroTimelineSvg');
  if (!svg) return;

  try {
    const res = await fetch(`${API_BASE}/api/v1/stats/timeline-24h`);
    if (!res.ok) return;
    const data = await res.json();

    const badge = document.getElementById('chartBlockedSummary');
    const totalBlocked = data.reduce((acc, d) => acc + (d.blocked || 0), 0);
    if (badge) badge.textContent = `${totalBlocked} bloqueadas`;

    const width = 850;
    const height = 210;
    const paddingLeft = 35;
    const paddingRight = 20;
    const paddingTop = 20;
    const paddingBottom = 30;
    const plotW = width - paddingLeft - paddingRight;
    const plotH = height - paddingTop - paddingBottom;

    const maxQ = Math.max(...data.map(d => d.queries), 1500);
    const maxVal = Math.ceil(maxQ * 1.1);
    const stepX = plotW / Math.max(data.length - 1, 1);

    const points = data.map((d, i) => {
      const x = paddingLeft + i * stepX;
      const yQ = paddingTop + plotH - (d.queries / maxVal) * plotH;
      const yB = paddingTop + plotH - ((d.blocked || 0) / maxVal) * plotH;
      return { x, yQ, yB, ...d };
    });

    // Curva suavizada (Catmull-Rom para Bézier)
    function buildSmoothPath(pts, yKey) {
      if (pts.length < 2) return '';
      let path = `M ${pts[0].x.toFixed(1)},${pts[0][yKey].toFixed(1)}`;
      for (let i = 0; i < pts.length - 1; i++) {
        const p0 = pts[Math.max(i - 1, 0)];
        const p1 = pts[i];
        const p2 = pts[i + 1];
        const p3 = pts[Math.min(i + 2, pts.length - 1)];

        const cp1x = p1.x + (p2.x - p0.x) / 6;
        const cp1y = p1[yKey] + (p2[yKey] - p0[yKey]) / 6;
        const cp2x = p2.x - (p3.x - p1.x) / 6;
        const cp2y = p2[yKey] - (p3[yKey] - p1[yKey]) / 6;

        path += ` C ${cp1x.toFixed(1)},${cp1y.toFixed(1)} ${cp2x.toFixed(1)},${cp2y.toFixed(1)} ${p2.x.toFixed(1)},${p2[yKey].toFixed(1)}`;
      }
      return path;
    }

    const qLine = buildSmoothPath(points, 'yQ');
    const bLine = buildSmoothPath(points, 'yB');
    const baselineY = paddingTop + plotH;
    const qArea = `${qLine} L ${points[points.length - 1].x.toFixed(1)},${baselineY} L ${points[0].x.toFixed(1)},${baselineY} Z`;

    // Linhas de grade e marcadores X
    let gridLines = '';
    let xLabels = '';
    const yGridLevels = [0, 0.33, 0.66, 1];
    yGridLevels.forEach(lvl => {
      const y = paddingTop + plotH * (1 - lvl);
      const val = Math.round(maxVal * lvl);
      const labelText = val >= 1000 ? (val / 1000).toFixed(1) + 'k' : val;
      gridLines += `<line x1="${paddingLeft}" y1="${y}" x2="${width - paddingRight}" y2="${y}" stroke="rgba(255,255,255,0.04)" stroke-dasharray="3,3"/>`;
      gridLines += `<text x="${paddingLeft - 8}" y="${y + 4}" text-anchor="end" fill="#64748b" font-size="10" font-family="var(--font-mono)">${labelText}</text>`;
    });

    points.forEach((p, idx) => {
      if (idx % 2 === 0 || idx === points.length - 1) {
        xLabels += `<text x="${p.x}" y="${height - 8}" text-anchor="middle" fill="#64748b" font-size="10" font-family="var(--font-mono)">${p.time}</text>`;
      }
    });

    svg.innerHTML = `
      <defs>
        <linearGradient id="heroGreenGrad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color="#10b981" stop-opacity="0.38"/>
          <stop offset="100%" stop-color="#10b981" stop-opacity="0.00"/>
        </linearGradient>
      </defs>
      ${gridLines}
      <path d="${qArea}" fill="url(#heroGreenGrad)"/>
      <path d="${qLine}" fill="none" stroke="#10b981" stroke-width="2.2" stroke-linecap="round"/>
      <path d="${bLine}" fill="none" stroke="#ef4444" stroke-width="1.8" stroke-linecap="round"/>
      ${xLabels}
    `;
  } catch (err) {
    console.warn('Erro ao desenhar gráfico hero:', err);
  }
}

async function renderDeviceDonut() {
  try {
    const res = await fetch(`${API_BASE}/api/v1/stats/devices`);
    if (!res.ok) return;
    const d = await res.json();

    const centerCount = document.getElementById('donutCenterCount');
    const summary = document.getElementById('donutDevicesTotalSummary');
    const onlineEl = document.getElementById('donutOnlineCount');
    const offlineEl = document.getElementById('donutOfflineCount');
    const circle = document.getElementById('donutCircleOnline');

    if (centerCount) centerCount.textContent = d.total || 2;
    if (summary) summary.textContent = `${d.total || 2} dispositivos`;
    if (onlineEl) onlineEl.textContent = d.online || 2;
    if (offlineEl) offlineEl.textContent = d.offline || 0;

    if (circle) {
      const circ = 2 * Math.PI * 48; // ~301.59
      const ratio = d.total > 0 ? (d.online / d.total) : 1;
      const offset = circ * (1 - ratio);
      circle.style.strokeDashoffset = offset;
    }
  } catch (err) {
    console.warn('Erro ao atualizar donut de dispositivos:', err);
  }
}

async function loadDashboardTopWidgets() {
  try {
    // 1. Aplicações
    const resApps = await fetch(`${API_BASE}/api/v1/stats/applications`);
    if (resApps.ok) {
      const dataApps = await resApps.json();
      const listTopApps = document.getElementById('listTopApps');
      const listTopBlockedApps = document.getElementById('listTopBlockedApps');

      if (listTopApps && dataApps.top_used) {
        const maxQ = Math.max(...dataApps.top_used.map(a => a.queries), 1);
        listTopApps.innerHTML = dataApps.top_used.map((a, i) => {
          const rank = i + 1;
          const rankClass = rank === 1 ? 'top1' : (rank === 2 ? 'top2' : (rank === 3 ? 'top3' : ''));
          const pct = Math.round((a.queries / maxQ) * 100);
          const icon = getAppIconSvg(a.id, a.name, a.color, a.domains || []);

          return `
            <li class="top10-item">
              <div class="top10-bar-bg" style="width: ${pct}%;"></div>
              <div class="top10-left-group">
                <span class="top10-rank ${rankClass}">${rank}</span>
                ${icon}
                <span class="top10-name" title="${escapeHtml(a.name)}">${escapeHtml(a.name)}</span>
              </div>
              <span class="top10-count">${a.queries}</span>
            </li>
          `;
        }).join('');
      }

      if (listTopBlockedApps) {
        if (!dataApps.top_blocked || dataApps.top_blocked.length === 0) {
          listTopBlockedApps.innerHTML = `
            <div class="top10-empty-state">
              <svg class="empty-cup-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M17 8h1a4 4 0 1 1 0 8h-1"/><path d="M3 8h14v9a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4Z"/><line x1="6" y1="2" x2="6" y2="4"/><line x1="10" y1="2" x2="10" y2="4"/><line x1="14" y1="2" x2="14" y2="4"/>
              </svg>
              <span class="empty-cup-text">Nenhum dado encontrado</span>
            </div>
          `;
        } else {
          const maxB = Math.max(...dataApps.top_blocked.map(a => a.queries), 1);
          listTopBlockedApps.innerHTML = dataApps.top_blocked.map((a, i) => {
            const rank = i + 1;
            const pct = Math.round((a.queries / maxB) * 100);
            const icon = getAppIconSvg(a.id, a.name, a.color || '#ef4444', a.domains || []);

            return `
              <li class="top10-item">
                <div class="top10-bar-bg blocked" style="width: ${pct}%;"></div>
                <div class="top10-left-group">
                  <span class="top10-rank top1" style="background: rgba(239, 68, 68, 0.2); color: #f87171;">${rank}</span>
                  ${icon}
                  <span class="top10-name" title="${escapeHtml(a.name)}">${escapeHtml(a.name)}</span>
                </div>
                <span class="top10-count" style="color: #f87171;">${a.queries}</span>
              </li>
            `;
          }).join('');
        }
      }
    }

    // 2. Categorias
    const resCats = await fetch(`${API_BASE}/api/v1/stats/categories`);
    if (resCats.ok) {
      const dataCats = await resCats.json();
      const listTopCats = document.getElementById('listTopCategories');
      const listTopBlockedCats = document.getElementById('listTopBlockedCategories');

      if (listTopCats && dataCats.top_used) {
        const maxQ = Math.max(...dataCats.top_used.map(c => c.queries), 1);
        listTopCats.innerHTML = dataCats.top_used.map((c, i) => {
          const rank = i + 1;
          const pct = Math.round((c.queries / maxQ) * 100);
          return `
            <li class="top10-item">
              <div class="top10-bar-bg" style="width: ${pct}%;"></div>
              <div class="top10-left-group">
                <span class="top10-rank ${rank === 1 ? 'top1' : (rank === 2 ? 'top2' : '')}">${rank}</span>
                <span class="top10-name">${escapeHtml(c.name)}</span>
              </div>
              <span class="top10-count">${c.queries}</span>
            </li>
          `;
        }).join('');
      }

      if (listTopBlockedCats) {
        if (!dataCats.top_blocked || dataCats.top_blocked.length === 0) {
          listTopBlockedCats.innerHTML = `
            <div class="top10-empty-state">
              <svg class="empty-cup-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                <path d="M17 8h1a4 4 0 1 1 0 8h-1"/><path d="M3 8h14v9a4 4 0 0 1-4 4H7a4 4 0 0 1-4-4Z"/><line x1="6" y1="2" x2="6" y2="4"/><line x1="10" y1="2" x2="10" y2="4"/><line x1="14" y1="2" x2="14" y2="4"/>
              </svg>
              <span class="empty-cup-text">Nenhum dado encontrado</span>
            </div>
          `;
        } else {
          const maxB = Math.max(...dataCats.top_blocked.map(c => c.queries), 1);
          listTopBlockedCats.innerHTML = dataCats.top_blocked.map((c, i) => {
            const rank = i + 1;
            const pct = Math.round((c.queries / maxB) * 100);
            return `
              <li class="top10-item">
                <div class="top10-bar-bg blocked" style="width: ${pct}%;"></div>
                <div class="top10-left-group">
                  <span class="top10-rank top1" style="background: rgba(239, 68, 68, 0.2); color: #f87171;">${rank}</span>
                  <span class="top10-name">${escapeHtml(c.name)}</span>
                </div>
                <span class="top10-count" style="color: #f87171;">${c.queries}</span>
              </li>
            `;
          }).join('');
        }
      }
    }
  } catch (err) {
    console.warn('Erro nos widgets top 10:', err);
  }
}

async function loadDashboardGroupsSummary() {
  const container = document.getElementById('containerDashboardGroupsList');
  if (!container) return;

  try {
    const res = await fetch(`${API_BASE}/api/v1/stats/groups`);
    if (!res.ok) return;
    const groups = await res.json();

    container.innerHTML = groups.map(g => `
      <div class="card" style="padding: 1rem; border: 1px solid rgba(255,255,255,0.06); background: rgba(255,255,255,0.02);">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span class="group-dot" style="background: ${g.color}; width: 10px; height: 10px;"></span>
            <strong style="font-size: 0.88rem;">${escapeHtml(g.name)}</strong>
          </div>
          <span class="badge" style="background: rgba(255,255,255,0.06); font-size: 0.72rem;">${g.client_count} disp.</span>
        </div>
        <div style="font-size: 0.78rem; color: var(--text-dim); margin-bottom: 0.65rem;">
          Consultas: <strong>${g.total_queries.toLocaleString()}</strong> | Bloqueadas: <strong style="color: #f87171;">${g.blocked_queries.toLocaleString()}</strong> (${g.block_rate}%)
        </div>
        <button class="btn btn-secondary btn-sm" style="width: 100%; font-size: 0.76rem;" onclick="openPolicyForGroup(${g.id})">
          ⚙️ Ajustar Aplicações &amp; Horário &rarr;
        </button>
      </div>
    `).join('');
  } catch (err) {
    console.warn('Erro ao carregar resumo de grupos:', err);
  }
}

window.openPolicyForGroup = function(groupId) {
  state.currentPolicyGroup = groupId;
  switchTab('policies');
};

// ==========================================================
// ABA: POLÍTICAS & GRUPOS (APLICAÇÕES, HORÁRIOS & AGENDAMENTO)
// ==========================================================
function setupPoliciesControls() {
  // Botão voltar do header da política
  const btnBack = document.getElementById('btnPolicyBackToDashboard');
  if (btnBack) btnBack.addEventListener('click', () => switchTab('dashboard'));

  // Botão no dashboard para ir às políticas
  const btnGoPolicies = document.getElementById('btnGoToPoliciesTab');
  if (btnGoPolicies) btnGoPolicies.addEventListener('click', () => switchTab('policies'));

  // Navegação entre sub-abas da política
  document.querySelectorAll('.policy-subnav-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      const target = tab.getAttribute('data-pol-subtab');
      switchPolicySubtab(target);
    });
  });

  // Seletor de Grupo
  const selGroup = document.getElementById('selectCurrentPolicyGroup');
  if (selGroup) {
    selGroup.addEventListener('change', (e) => {
      state.currentPolicyGroup = parseInt(e.target.value, 10);
      loadPolicyApplications(state.currentPolicyGroup);
      loadPolicySchedule(state.currentPolicyGroup);
      loadPolicyCategories(state.currentPolicyGroup);
      loadPolicyGroupDevices(state.currentPolicyGroup);
      updatePolicyHeaderTitle();
    });
  }

  // Busca de Aplicações
  const searchInput = document.getElementById('inputSearchPolicyApps');
  if (searchInput) {
    searchInput.addEventListener('input', debounce((e) => {
      state.searchAppQuery = e.target.value.toLowerCase().trim();
      renderPolicyCategoryCards();
    }, 150));
  }

  // Filtro "Mostrar somente bloqueadas"
  const checkOnlyBlocked = document.getElementById('checkFilterOnlyBlockedApps');
  if (checkOnlyBlocked) {
    checkOnlyBlocked.addEventListener('change', (e) => {
      state.filterOnlyBlockedApps = e.target.checked;
      renderPolicyCategoryCards();
    });
  }

  // Switch Habilitar Horário
  const checkSchedule = document.getElementById('checkEnablePolicySchedule');
  if (checkSchedule) {
    checkSchedule.addEventListener('change', (e) => {
      state.groupSchedule.enabled = e.target.checked;
      savePolicySchedule();
    });
  }

  // Modos de Horário (Totalmente Bloqueado, Parcialmente Bloqueado, Totalmente Liberado)
  document.querySelectorAll('.mode-card-option').forEach(opt => {
    opt.addEventListener('click', () => {
      const mode = opt.getAttribute('data-schedule-mode');
      setScheduleMode(mode);
    });
  });

  // Presets de Agendamento
  const btnCom = document.getElementById('btnPresetCommercial');
  if (btnCom) btnCom.addEventListener('click', () => applySchedulePreset('commercial'));

  const btnFull = document.getElementById('btnPresetFullDay');
  if (btnFull) btnFull.addEventListener('click', () => applySchedulePreset('full_day'));

  const btnWk = document.getElementById('btnPresetWeekend');
  if (btnWk) btnWk.addEventListener('click', () => applySchedulePreset('weekend'));

  const btnAll = document.getElementById('btnPresetSelectAll');
  if (btnAll) btnAll.addEventListener('click', () => applySchedulePreset('all'));

  const btnClr = document.getElementById('btnPresetClearAll');
  if (btnClr) btnClr.addEventListener('click', () => applySchedulePreset('clear'));

  // Salvar Horário
  const btnSaveSched = document.getElementById('btnSaveScheduleOnly');
  if (btnSaveSched) btnSaveSched.addEventListener('click', () => savePolicySchedule(true));

  // Salvar Geral
  const btnSaveAll = document.getElementById('btnSaveAllPolicyChanges');
  if (btnSaveAll) {
    btnSaveAll.addEventListener('click', async () => {
      await savePolicyApplications();
      await savePolicySchedule();
      showToast('✨ Todas as regras da política foram salvas com sucesso!');
    });
  }
}

function switchPolicySubtab(subtabId) {
  state.currentPolicyTab = subtabId;
  document.querySelectorAll('.policy-subnav-tab').forEach(t => {
    t.classList.toggle('active', t.getAttribute('data-pol-subtab') === subtabId);
  });
  document.querySelectorAll('.policy-tab-pane').forEach(p => {
    p.classList.toggle('active', p.id === `subtab-policy-${subtabId}`);
  });

  if (subtabId === 'schedule') {
    renderWeeklyScheduleGrid();
  }
}

async function loadPoliciesSection() {
  await loadGroups();
  populatePolicyGroupSelect();
  updatePolicyHeaderTitle();
  await loadPolicyApplications(state.currentPolicyGroup);
  await loadPolicySchedule(state.currentPolicyGroup);
  loadPolicyCategories(state.currentPolicyGroup);
  loadPolicyGroupDevices(state.currentPolicyGroup);
}

function populatePolicyGroupSelect() {
  const sel = document.getElementById('selectCurrentPolicyGroup');
  if (!sel) return;
  sel.innerHTML = state.groups.map(g => `
    <option value="${g.id}" ${g.id === state.currentPolicyGroup ? 'selected' : ''}>
      ${escapeHtml(g.name)} (${g.client_count || 0} disp.)
    </option>
  `).join('');
}

function updatePolicyHeaderTitle() {
  const group = state.groups.find(g => g.id === state.currentPolicyGroup) || state.groups[0];
  const titleEl = document.getElementById('labelPolicyGroupTitle');
  if (titleEl && group) {
    titleEl.textContent = `Política: ${group.name}`;
  }
}

// Aplicações: Catálogo, Filtros e Regras
async function loadPolicyApplications(groupId) {
  try {
    // 1. Carregar catálogo se vazio
    if (!state.catalogApplications || state.catalogApplications.length === 0) {
      const resCat = await fetch(`${API_BASE}/api/v1/applications`);
      if (resCat.ok) state.catalogApplications = await resCat.json();
    }

    // 2. Carregar regras do grupo
    const resRules = await fetch(`${API_BASE}/api/v1/groups/applications?group_id=${groupId}`);
    if (resRules.ok) {
      const data = await resRules.json();
      state.groupAppRules = data.rules || {};
    }

    renderPolicyCategoryCards();
    updateBlockedCountBadge();
  } catch (err) {
    console.warn('Erro ao carregar aplicações do grupo:', err);
  }
}

function updateBlockedCountBadge() {
  const badge = document.getElementById('policyAppsBlockedBadge');
  if (!badge) return;
  const count = Object.values(state.groupAppRules).filter(v => v === 'block').length;
  badge.textContent = count;
}

function renderPolicyCategoryCards() {
  const container = document.getElementById('gridPolicyCategoryCards');
  if (!container || !state.catalogApplications) return;

  // Agrupar aplicações por categoria
  const grouped = {};
  state.catalogApplications.forEach(app => {
    if (!grouped[app.category]) grouped[app.category] = [];
    grouped[app.category].push(app);
  });

  const categoryLabels = {
    ai: 'AI',
    bank: 'Bank',
    betting: 'Betting',
    education: 'Education',
    finance: 'Finance',
    games: 'Games',
    google: 'Google',
    health: 'Health',
    messaging: 'Messaging',
    meta: 'Meta',
    microsoft: 'Microsoft',
    music: 'Music',
    streaming: 'Streaming',
    social: 'Social',
  };

  const htmlCards = [];

  Object.entries(grouped).forEach(([catKey, apps]) => {
    // Filtro por busca
    let filteredApps = apps;
    if (state.searchAppQuery) {
      filteredApps = apps.filter(a => a.name.toLowerCase().includes(state.searchAppQuery) || a.domains.some(d => d.includes(state.searchAppQuery)));
    }

    // Filtro por apenas bloqueadas
    if (state.filterOnlyBlockedApps) {
      filteredApps = filteredApps.filter(a => state.groupAppRules[a.id] === 'block');
    }

    if (filteredApps.length === 0 && (state.searchAppQuery || state.filterOnlyBlockedApps)) {
      return; // Oculta categoria vazia no filtro
    }

    // Verifica se todas as apps da categoria estão bloqueadas
    const allBlocked = apps.length > 0 && apps.every(a => state.groupAppRules[a.id] === 'block');
    const label = categoryLabels[catKey] || catKey.toUpperCase();

    const appItemsHtml = filteredApps.map(a => {
      const isBlocked = state.groupAppRules[a.id] === 'block';
      const icon = getAppIconSvg(a.id, a.name, a.color, a.domains);

      return `
        <div class="app-item-row">
          <div class="app-item-left">
            ${icon}
            <span class="app-brand-name" title="${escapeHtml(a.name)}">${escapeHtml(a.name)}</span>
          </div>
          <label class="tactile-switch">
            <input type="checkbox" ${isBlocked ? 'checked' : ''} onchange="toggleAppRule('${a.id}', this.checked)">
            <span class="tactile-slider"></span>
          </label>
        </div>
      `;
    }).join('');

    htmlCards.push(`
      <div class="app-category-card">
        <div class="app-category-card-header">
          <div class="category-name-count">
            <span>${label}</span>
            <span class="category-app-count-tag">(${apps.length})</span>
          </div>
          <label class="tactile-switch" title="Bloquear ou liberar toda a categoria">
            <input type="checkbox" ${allBlocked ? 'checked' : ''} onchange="toggleCategoryMasterRule('${catKey}', this.checked)">
            <span class="tactile-slider"></span>
          </label>
        </div>
        <div class="apps-scrollable-list">
          ${appItemsHtml}
        </div>
      </div>
    `);
  });

  if (htmlCards.length === 0) {
    container.innerHTML = `
      <div style="grid-column: 1 / -1; padding: 3rem; text-align: center; color: var(--text-dim);">
        <p>Nenhuma aplicação encontrada com os filtros atuais.</p>
      </div>
    `;
    return;
  }

  container.innerHTML = htmlCards.join('');
}

window.toggleAppRule = async function(appId, isBlocked) {
  state.groupAppRules[appId] = isBlocked ? 'block' : 'allow';
  updateBlockedCountBadge();
  await savePolicyApplications();
  renderPolicyCategoryCards();
  showToast(isBlocked ? `🛑 Aplicação bloqueada para este grupo.` : `✅ Aplicação liberada.`);
};

window.toggleCategoryMasterRule = async function(catKey, isBlocked) {
  const apps = state.catalogApplications.filter(a => a.category === catKey);
  apps.forEach(a => {
    state.groupAppRules[a.id] = isBlocked ? 'block' : 'allow';
  });
  updateBlockedCountBadge();
  await savePolicyApplications();
  renderPolicyCategoryCards();
  showToast(isBlocked ? `🛑 Categoria ${catKey.toUpperCase()} inteira bloqueada!` : `✅ Categoria ${catKey.toUpperCase()} liberada.`);
};

async function savePolicyApplications() {
  try {
    await fetch(`${API_BASE}/api/v1/groups/applications`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        group_id: state.currentPolicyGroup,
        rules: state.groupAppRules
      })
    });
  } catch (err) {
    console.warn('Erro ao salvar aplicações do grupo:', err);
  }
}

// ==========================================================
// Horário de Funcionamento & Agendamento (Screenshot 2)
// ==========================================================
async function loadPolicySchedule(groupId) {
  try {
    const res = await fetch(`${API_BASE}/api/v1/groups/schedule?group_id=${groupId}`);
    if (res.ok) {
      const data = await res.json();
      state.groupSchedule = data.schedule || { enabled: true, mode: 'partial_block', slots: [] };
    }

    const check = document.getElementById('checkEnablePolicySchedule');
    if (check) check.checked = state.groupSchedule.enabled !== false;

    setScheduleMode(state.groupSchedule.mode || 'partial_block', false);
    renderWeeklyScheduleGrid();
  } catch (err) {
    console.warn('Erro ao carregar agendamento do grupo:', err);
  }
}

function setScheduleMode(mode, save = true) {
  state.groupSchedule.mode = mode;
  document.querySelectorAll('.mode-card-option').forEach(opt => {
    opt.classList.toggle('active', opt.getAttribute('data-schedule-mode') === mode);
  });
  if (save) {
    savePolicySchedule();
    showToast(`Modo alterado para: ${mode === 'total_block' ? 'Totalmente Bloqueado' : (mode === 'total_allow' ? 'Totalmente Liberado' : 'Parcialmente Bloqueado')}`);
  }
}

function renderWeeklyScheduleGrid() {
  const tbody = document.getElementById('tbodyWeeklyGrid');
  if (!tbody) return;

  const days = ['Dom', 'Seg', 'Ter', 'Qua', 'Qui', 'Sex', 'Sáb'];
  const today = new Date().getDay(); // 0 a 6

  // Destacar cabeçalho do dia atual na tabela
  document.querySelectorAll('#tableWeeklySchedule th[data-day-index]').forEach(th => {
    const dIdx = parseInt(th.getAttribute('data-day-index'), 10);
    th.classList.toggle('active-day-col', dIdx === today);
  });

  // Mapear slots em matriz para renderização instantânea
  // slots: [{ day: 1, start: '08:00', end: '12:00' }, ...]
  const activeHours = {};
  (state.groupSchedule.slots || []).forEach(slot => {
    const sH = parseInt(slot.start.split(':')[0], 10);
    const eH = parseInt(slot.end.split(':')[0], 10);
    for (let h = sH; h < eH; h++) {
      activeHours[`${slot.day}_${h}`] = { start: slot.start, end: slot.end, isFirst: h === sH };
    }
  });

  let rowsHtml = '';
  for (let h = 0; h < 24; h++) {
    const timeLabel = `${String(h).padStart(2, '0')}:00`;
    let cellsHtml = '';

    for (let d = 0; d < 7; d++) {
      const key = `${d}_${h}`;
      const slotInfo = activeHours[key];
      const isActive = !!slotInfo;

      let cellContent = '';
      if (isActive && slotInfo.isFirst) {
        cellContent = `<div class="schedule-rendered-block">${slotInfo.start} - ${slotInfo.end}</div>`;
      }

      cellsHtml += `
        <td class="slot-cell ${isActive ? 'active-slot' : ''}" 
            data-day="${d}" data-hour="${h}" 
            onclick="toggleGridHour(${d}, ${h})">
          ${cellContent}
        </td>
      `;
    }

    rowsHtml += `
      <tr>
        <td class="time-label-cell">${timeLabel}</td>
        ${cellsHtml}
      </tr>
    `;
  }

  tbody.innerHTML = rowsHtml;
  updateCurrentTimeMarker();
}

function updateCurrentTimeMarker() {
  const marker = document.getElementById('currentTimeMarker');
  const badge = document.getElementById('currentTimeBadge');
  const wrapper = document.getElementById('calendarTableWrapper');
  if (!marker || !badge || !wrapper) return;

  const now = new Date();
  const day = now.getDay(); // 0 a 6
  const hours = now.getHours();
  const minutes = now.getMinutes();
  const timeStr = `${String(hours).padStart(2, '0')}:${String(minutes).padStart(2, '0')}`;

  // Calcular posição Y baseada na hora e minuto (linha do tempo)
  const headerHeight = 42;
  const rowHeight = 39;
  const topY = headerHeight + (hours + minutes / 60) * rowHeight;

  // Calcular posição X baseada na coluna do dia
  const colWidthPct = 100 / 8; // 1 label + 7 dias
  const leftX = (day + 1) * colWidthPct;

  marker.style.display = 'block';
  marker.style.top = `${topY}px`;
  marker.style.left = `${leftX}%`;
  marker.style.width = `${colWidthPct}%`;
  badge.textContent = timeStr;
}

window.toggleGridHour = function(day, hour) {
  const currentSlots = state.groupSchedule.slots || [];
  // Verificar se o clique cai em um intervalo existente
  const targetTime = `${String(hour).padStart(2, '0')}:00`;
  const nextTime = `${String(hour + 1).padStart(2, '0')}:00`;

  const existingIdx = currentSlots.findIndex(s => {
    if (s.day !== day) return false;
    const sH = parseInt(s.start.split(':')[0], 10);
    const eH = parseInt(s.end.split(':')[0], 10);
    return hour >= sH && hour < eH;
  });

  if (existingIdx !== -1) {
    // Remove o slot
    currentSlots.splice(existingIdx, 1);
  } else {
    // Adiciona bloco de 1 hora
    currentSlots.push({ day, start: targetTime, end: nextTime });
  }

  state.groupSchedule.slots = currentSlots;
  renderWeeklyScheduleGrid();
  savePolicySchedule();
};

function applySchedulePreset(type) {
  let slots = [];
  if (type === 'commercial') {
    // Seg a Sex 08:00 - 12:00 e 14:00 - 18:00 (Screenshot 2)
    for (let d = 1; d <= 5; d++) {
      slots.push({ day: d, start: '08:00', end: '12:00' });
      slots.push({ day: d, start: '14:00', end: '18:00' });
    }
  } else if (type === 'full_day') {
    for (let d = 1; d <= 5; d++) {
      slots.push({ day: d, start: '08:00', end: '18:00' });
    }
  } else if (type === 'weekend') {
    slots.push({ day: 0, start: '00:00', end: '24:00' });
    slots.push({ day: 6, start: '00:00', end: '24:00' });
  } else if (type === 'all') {
    for (let d = 0; d < 7; d++) {
      slots.push({ day: d, start: '00:00', end: '24:00' });
    }
  } else if (type === 'clear') {
    slots = [];
  }

  state.groupSchedule.slots = slots;
  renderWeeklyScheduleGrid();
  savePolicySchedule(true);
  showToast(`Atalho de grade aplicado com sucesso!`);
}

async function savePolicySchedule(showNotice = false) {
  const indicator = document.getElementById('scheduleSaveIndicator');
  if (indicator) indicator.textContent = 'Salvando...';

  try {
    await fetch(`${API_BASE}/api/v1/groups/schedule`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        group_id: state.currentPolicyGroup,
        schedule: state.groupSchedule
      })
    });

    if (indicator) {
      indicator.textContent = '✅ Horário salvo!';
      setTimeout(() => { if (indicator) indicator.textContent = ''; }, 3000);
    }
    if (showNotice) {
      showToast('Horário de funcionamento salvo com sucesso!');
    }
  } catch (err) {
    if (indicator) indicator.textContent = '❌ Erro ao salvar';
    console.warn('Erro ao salvar horário:', err);
  }
}

// Categorias e Vínculos da Política
async function loadPolicyCategories(groupId) {
  const container = document.getElementById('containerPolicyCategoriesGrid');
  if (!container) return;

  await loadCategories();
  const rules = (state.groupCategoryRules && state.groupCategoryRules[groupId]) || {};

  container.innerHTML = state.availableCategories.map(cat => {
    const isBlocked = rules[cat.id] === 'block';
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
        <button class="category-toggle-btn ${isBlocked ? 'btn-blocked' : 'btn-allowed'}" 
                onclick="togglePolicyCategoryAction('${cat.id}')">
          ${isBlocked ? '🚫 Bloqueado' : '✅ Permitido'}
        </button>
      </div>
    `;
  }).join('');
}

window.togglePolicyCategoryAction = async function(catId) {
  const groupId = state.currentPolicyGroup;
  if (!state.groupCategoryRules[groupId]) state.groupCategoryRules[groupId] = {};
  const current = state.groupCategoryRules[groupId][catId] || 'allow';
  state.groupCategoryRules[groupId][catId] = current === 'block' ? 'allow' : 'block';

  await fetch(`${API_BASE}/api/v1/groups/categories`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ group_id: groupId, rules: state.groupCategoryRules[groupId] })
  });

  loadPolicyCategories(groupId);
  showToast('Políticas de categoria atualizadas!');
};

function loadPolicyGroupDevices(groupId) {
  const tbody = document.getElementById('tbodyPolicyGroupClients');
  if (!tbody) return;

  const groupClients = (state.clients || []).filter(c => c.group_id === groupId);
  if (groupClients.length === 0) {
    tbody.innerHTML = '<tr><td colspan="5" class="text-center text-muted">Nenhum dispositivo associado a esta política ainda.</td></tr>';
    return;
  }

  tbody.innerHTML = groupClients.map(c => `
    <tr>
      <td>${c.is_remote ? '<span class="tag-badge allowed">Nuvem (DoH)</span>' : '<span class="tag-badge" style="background:rgba(56,189,248,0.15); color:var(--accent-blue)">Wi-Fi Local</span>'}</td>
      <td><strong>${escapeHtml(c.name)}</strong></td>
      <td><code>${c.ip}</code></td>
      <td><span class="badge-status ${c.status === 'revoked' ? 'offline' : 'online'}">${c.status === 'revoked' ? 'Revogado' : 'Ativo'}</span></td>
      <td>${escapeHtml(c.comment || '-')}</td>
    </tr>
  `).join('');
}

// Atalho de Busca Global (Ctrl + K)
function setupSidebarSearch() {
  const input = document.getElementById('sidebarSearchInput');
  window.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      if (input) {
        input.focus();
        input.select();
      }
    }
  });

  if (input) {
    input.addEventListener('input', debounce((e) => {
      const q = e.target.value.toLowerCase().trim();
      if (!q) return;

      // Se o usuário buscar algo de aplicação ou política, navega para a tela
      const foundApp = state.catalogApplications.find(a => a.name.toLowerCase().includes(q));
      if (foundApp) {
        switchTab('policies');
        const appSearch = document.getElementById('inputSearchPolicyApps');
        if (appSearch) {
          appSearch.value = q;
          state.searchAppQuery = q;
          renderPolicyCategoryCards();
        }
      }
    }, 300));
  }
}

// Emil Kowalski Spring Toast Notification
function showToast(message) {
  let toast = document.getElementById('coorpGlobalToast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'coorpGlobalToast';
    toast.className = 'coorp-toast';
    document.body.appendChild(toast);
  }

  toast.innerHTML = `
    <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="#10b981" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
    <span>${escapeHtml(message)}</span>
  `;
  toast.classList.add('show');

  setTimeout(() => {
    toast.classList.remove('show');
  }, 3200);
}



