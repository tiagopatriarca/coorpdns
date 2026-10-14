// Servidor de Pré-visualização Local do Painel CoorpDNS (Node.js)
// Simula a API do backend corporativo em Go para testes visuais imediatos da interface

const http = require('http');
const fs = require('fs');
const path = require('path');
const url = require('url');

const PORT = 8080;
const STATIC_DIR = path.join(__dirname, 'cmd', 'homedns', 'static');

let isBlocking = true;
let pauseTimeout = 0;

let mockGroups = [
  { id: 1, name: 'Geral / Padrão Corporativo', description: 'Dispositivos corporativos padrão da rede', color: '#6366f1', icon: 'fas fa-building', client_count: 2 },
  { id: 2, name: 'Diretoria & Executivos', description: 'Dispositivos de gerência e diretoria com perfil executivo', color: '#8b5cf6', icon: 'fas fa-user-tie', client_count: 1 },
  { id: 3, name: 'Desenvolvimento & TI', description: 'Laptops de engenharia, DevOps e VPNs corporativas', color: '#3b82f6', icon: 'fas fa-laptop-code', client_count: 1 },
  { id: 4, name: 'Financeiro & Administrativo', description: 'Estações do financeiro com bloqueio restrito de segurança', color: '#10b981', icon: 'fas fa-file-invoice-dollar', client_count: 1 },
];

// Dados simulados de 180 dias
let mockClients = [
  { id: 1, ip: '192.168.1.45', name: 'Estação HP Z4 - Mariana (Financeiro)', mac: 'AA:BB:CC:11:22:33', comment: 'Estação desktop setor financeiro', block_enabled: true, is_remote: false, auth_token: '', status: 'active', group_id: 4, group_name: 'Financeiro & Administrativo' },
  { id: 2, ip: '192.168.1.72', name: 'MacBook Pro - Carlos (Diretoria)', mac: 'AA:BB:CC:44:55:66', comment: 'Notebook executivo Wi-Fi Corporativo', block_enabled: true, is_remote: false, auth_token: '', status: 'active', group_id: 2, group_name: 'Diretoria & Executivos' },
  { id: 3, ip: 'remote-tok_a8f93e', name: 'Notebook Dell XPS - Pedro (TI / Remoto)', mac: '', comment: 'Notebook de viagens e suporte externo via DoH', block_enabled: true, is_remote: true, auth_token: 'tok_a8f93e2b1049c3', status: 'active', group_id: 3, group_name: 'Desenvolvimento & TI' },
  { id: 4, ip: 'remote-tok_b4c27d', name: 'iPhone Corporativo - Ana (Vendas)', mac: '', comment: 'Celular comercial externo com perfil DoH', block_enabled: true, is_remote: true, auth_token: 'tok_b4c27d559810ef', status: 'active', group_id: 1, group_name: 'Geral / Padrão Corporativo' },
  { id: 5, ip: '192.168.1.104', name: 'Totem Recepção / Smart Display', mac: 'AA:BB:CC:AA:BB:CC', comment: 'Display institucional do hall de entrada', block_enabled: true, is_remote: false, auth_token: '', status: 'active', group_id: 1, group_name: 'Geral / Padrão Corporativo' },
];

const mockCustomRules = ['||tiktok.com^', '||ads.bet365.com^'];
const mockAllowRules = ['@@||analytics.google.com^'];

const mockBlocklists = [
  { name: 'OISD Basic (Leve e Seguro)', url: 'https://small.oisd.nl', enabled: true },
  { name: 'StevenBlack Unified Hosts', url: 'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts', enabled: true },
  { name: 'HaGeZi Multi Light', url: 'https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/light.txt', enabled: true },
];

const mockCategories = [
  { id: 'adult', name: 'Conteúdo Adulto / Pornografia', description: 'Sites adultos, pornografia, nudez e conteúdo explícito', icon: 'fas fa-ban', color: '#ef4444' },
  { id: 'gambling', name: 'Apostas & Jogos de Azar', description: 'Cassinos online, apostas esportivas (Bet365, Betano, Blaze)', icon: 'fas fa-dice', color: '#f59e0b' },
  { id: 'social', name: 'Redes Sociais', description: 'Instagram, TikTok, Facebook, Twitter/X, Reddit, Snapchat', icon: 'fas fa-share-alt', color: '#3b82f6' },
  { id: 'gaming', name: 'Jogos & Games Online', description: 'Steam, Roblox, Epic Games, PlayStation, Xbox, Discord, Twitch', icon: 'fas fa-gamepad', color: '#8b5cf6' },
  { id: 'streaming', name: 'Streaming de Vídeo & Música', description: 'Netflix, YouTube, Disney+, Prime Video, Spotify, HBO', icon: 'fas fa-film', color: '#ec4899' },
  { id: 'p2p_torrent', name: 'P2P, Torrents & Pirataria', description: 'ThePirateBay, 1337x, trackers torrent, clientes BitTorrent', icon: 'fas fa-magnet', color: '#64748b' },
  { id: 'crypto', name: 'Criptomoedas & Mineração', description: 'Corretoras cripto (Binance, Coinbase) e mineradores web', icon: 'fas fa-coins', color: '#eab308' },
  { id: 'dating', name: 'Relacionamentos & Encontros', description: 'Tinder, Badoo, Happn, Grindr, Bumble', icon: 'fas fa-heart', color: '#f43f5e' },
  { id: 'trackers', name: 'Rastreadores & Telemetria', description: 'Telemetria de sistemas, métricas invasivas e rastreamento de comportamento', icon: 'fas fa-eye-slash', color: '#6366f1' },
  { id: 'ads', name: 'Anúncios & Publicidade', description: 'Redes de anúncios, banners, popups e servidores de publicidade', icon: 'fas fa-bullhorn', color: '#14b8a6' },
];

let mockGroupCategoryRules = {
  1: { ads: 'block', trackers: 'block', adult: 'block', gambling: 'allow', social: 'allow', gaming: 'allow', streaming: 'allow', p2p_torrent: 'allow', crypto: 'allow', dating: 'allow' },
  2: { ads: 'block', trackers: 'block', adult: 'block', gambling: 'block', dating: 'block', p2p_torrent: 'block', crypto: 'block', gaming: 'block', social: 'allow', streaming: 'allow' },
  3: { ads: 'block', trackers: 'block', adult: 'block', gambling: 'block', p2p_torrent: 'block', social: 'block', gaming: 'block', dating: 'block', streaming: 'allow', crypto: 'allow' },
  4: { ads: 'block', trackers: 'block', adult: 'block', gambling: 'block', p2p_torrent: 'block', crypto: 'block', social: 'allow', gaming: 'allow', streaming: 'allow', dating: 'allow' },
};

let mockAdForwarders = [
  { id: 1, domain: 'empresa.local', upstream_ip: '192.168.1.10:53', comment: 'DC Primário Windows Server 2022 (AD DS / Kerberos)', enabled: true, created_at: new Date().toISOString() },
  { id: 2, domain: '_msdcs.empresa.local', upstream_ip: '192.168.1.10:53', comment: 'Zona SRV de Localização de Domain Controllers', enabled: true, created_at: new Date().toISOString() },
  { id: 3, domain: '1.168.192.in-addr.arpa', upstream_ip: '192.168.1.10:53', comment: 'Zona de Pesquisa Inversa do AD', enabled: true, created_at: new Date().toISOString() },
];

let mockSupportConfig = {
  block_mode: 'block_page',
  server_ip: '192.168.1.100',
  support_email: 'suporte@empresa.com.br',
  support_phone: '5511999998888',
  org_name: 'TI Corporativa - Segurança da Informação'
};

function updateGroupCounts() {
  mockGroups.forEach(g => {
    g.client_count = mockClients.filter(c => c.group_id === g.id).length;
  });
}

const server = http.createServer((req, res) => {
  const parsedUrl = url.parse(req.url, true);
  const pathname = parsedUrl.pathname;

  // CORS
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type');

  if (req.method === 'OPTIONS') {
    res.writeHead(200);
    res.end();
    return;
  }

let mockUsers = [
  { id: 1, username: 'admin', full_name: 'Administrador Principal', role: 'admin', allowed_groups: [1, 2, 3, 4], created_at: new Date().toISOString() },
  { id: 2, username: 'operador_ti', full_name: 'Lucas (Operador TI)', role: 'operator', allowed_groups: [3], created_at: new Date().toISOString() },
];
let currentSessionUser = null;

// API Endpoints
  // Autenticação & Usuários
  if (pathname === '/api/v1/auth/me') {
    if (!currentSessionUser) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ error: 'Não autenticado' }));
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(currentSessionUser));
  }

  if (pathname === '/api/v1/auth/login' && req.method === 'POST') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      const data = JSON.parse(body || '{}');
      const username = (data.username || '').trim();
      const password = (data.password || '').trim();

      if (username === 'admin' && password === 'admin') {
        currentSessionUser = mockUsers[0];
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({ token: 'ses_mock_admin', user: currentSessionUser }));
      }

      const found = mockUsers.find(u => u.username === username);
      if (found && (password === 'admin' || password === '123456' || password === 'operador')) {
        currentSessionUser = found;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({ token: 'ses_mock_' + found.id, user: currentSessionUser }));
      }

      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'Usuário ou senha incorretos' }));
    });
    return;
  }

  if (pathname === '/api/v1/auth/logout' && req.method === 'POST') {
    currentSessionUser = null;
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ success: true }));
  }

  if (pathname === '/api/v1/users') {
    if (req.method === 'GET') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify(mockUsers));
    }
    if (req.method === 'POST') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const newUser = {
          id: Date.now(),
          username: data.username,
          full_name: data.full_name,
          role: data.role || 'operator',
          allowed_groups: data.group_ids || [],
          created_at: new Date().toISOString()
        };
        mockUsers.push(newUser);
        res.writeHead(201, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(newUser));
      });
      return;
    }
  }

  if (pathname.startsWith('/api/v1/users/')) {
    const id = parseInt(pathname.replace('/api/v1/users/', ''), 10);
    if (req.method === 'PUT') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const idx = mockUsers.findIndex(u => u.id === id);
        if (idx !== -1) {
          mockUsers[idx].username = data.username || mockUsers[idx].username;
          mockUsers[idx].full_name = data.full_name || mockUsers[idx].full_name;
          mockUsers[idx].role = data.role || mockUsers[idx].role;
          mockUsers[idx].allowed_groups = data.group_ids || mockUsers[idx].allowed_groups;
        }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true }));
      });
      return;
    }
    if (req.method === 'DELETE') {
      mockUsers = mockUsers.filter(u => u.id !== id);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ success: true }));
    }
  }

  if (pathname === '/api/v1/status') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    let pauseRemaining = 0;
    if (pauseTimeout > Date.now()) {
      pauseRemaining = Math.ceil((pauseTimeout - Date.now()) / 1000);
    }
    return res.end(JSON.stringify({
      blocking_active: isBlocking && pauseRemaining === 0,
      pause_remaining: pauseRemaining,
      uptime_seconds: 14420,
      total_rules: 184520,
      rules_updated_at: new Date().toISOString(),
      version: '1.0.0',
      dns_address: ':53'
    }));
  }

  if (pathname === '/api/v1/control/toggle') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      const data = JSON.parse(body || '{}');
      if (data.duration_seconds > 0) {
        pauseTimeout = Date.now() + (data.duration_seconds * 1000);
      } else {
        isBlocking = data.enabled;
        pauseTimeout = 0;
      }
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ success: true, blocking_active: isBlocking }));
    });
    return;
  }

  if (pathname === '/api/v1/stats') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({
      total_queries: 48920,
      blocked_queries: 11450,
      block_rate: 23.4,
      unique_domains: 1240,
      active_clients: mockClients.length
    }));
  }

  // Grupos CRUD
  if (pathname === '/api/v1/groups') {
    if (req.method === 'GET') {
      updateGroupCounts();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify(mockGroups));
    }
    if (req.method === 'POST') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const newGroup = {
          id: Date.now(),
          name: data.name || 'Novo Grupo',
          description: data.description || '',
          color: data.color || '#6366f1',
          icon: data.icon || 'fas fa-folder',
          client_count: 0
        };
        mockGroups.push(newGroup);
        res.writeHead(201, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(newGroup));
      });
      return;
    }
  }

  if (pathname.startsWith('/api/v1/groups/') && pathname !== '/api/v1/groups/categories') {
    const groupId = parseInt(pathname.split('/')[4], 10);
    if (req.method === 'PUT') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const g = mockGroups.find(item => item.id === groupId);
        if (g) {
          if (data.name) g.name = data.name;
          if (data.description !== undefined) g.description = data.description;
          if (data.color) g.color = data.color;
          if (data.icon) g.icon = data.icon;
          // atualizar nome nos clients
          mockClients.forEach(c => {
            if (c.group_id === groupId) c.group_name = g.name;
          });
        }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(g || { error: 'Not found' }));
      });
      return;
    }
    if (req.method === 'DELETE') {
      if (groupId === 1) {
        res.writeHead(400, { 'Content-Type': 'application/json' });
        return res.end(JSON.stringify({ error: 'Não é possível remover o grupo padrão' }));
      }
      mockGroups = mockGroups.filter(g => g.id !== groupId);
      mockClients.forEach(c => {
        if (c.group_id === groupId) {
          c.group_id = 1;
          c.group_name = 'Padrão / Casa';
        }
      });
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ success: true }));
    }
  }

  // Associar cliente ao grupo
  if (pathname === '/api/v1/clients/group' && req.method === 'POST') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      const data = JSON.parse(body || '{}');
      const client = mockClients.find(c => (c.ip === data.client_ip_or_token || c.auth_token === data.client_ip_or_token));
      const group = mockGroups.find(g => g.id === parseInt(data.group_id, 10));
      if (client && group) {
        client.group_id = group.id;
        client.group_name = group.name;
      }
      updateGroupCounts();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ success: true }));
    });
    return;
  }

  // Categorias disponíveis
  if (pathname === '/api/v1/categories') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(mockCategories));
  }

  // Regras de categorias por grupo
  if (pathname === '/api/v1/groups/categories') {
    if (req.method === 'GET') {
      const groupId = parseInt(parsedUrl.query.group_id || '1', 10);
      const rules = mockGroupCategoryRules[groupId] || mockGroupCategoryRules[1];
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ group_id: groupId, rules: rules }));
    }
    if (req.method === 'PUT' || req.method === 'POST') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const groupId = parseInt(data.group_id || '1', 10);
        if (data.rules) {
          mockGroupCategoryRules[groupId] = data.rules;
        }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true }));
      });
      return;
    }
  }

  // Encaminhadores Condicionais (Active Directory / Zonas Locais)
  if (pathname === '/api/v1/forwarders') {
    if (req.method === 'GET') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify(mockAdForwarders));
    }
    if (req.method === 'POST') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        const newF = {
          id: Date.now(),
          domain: data.domain || 'novo.local',
          upstream_ip: data.upstream_ip || '192.168.1.10:53',
          comment: data.comment || '',
          enabled: true,
          created_at: new Date().toISOString()
        };
        mockAdForwarders.push(newF);
        res.writeHead(201, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify(newF));
      });
      return;
    }
  }

  if (pathname.startsWith('/api/v1/forwarders/')) {
    if (req.method === 'DELETE') {
      const fId = parseInt(pathname.split('/')[4], 10);
      mockAdForwarders = mockAdForwarders.filter(f => f.id !== fId);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ success: true }));
    }
  }

  // Configuração de Suporte e Modo de Bloqueio
  if (pathname === '/api/v1/config/support') {
    if (req.method === 'GET') {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify(mockSupportConfig));
    }
    if (req.method === 'PUT' || req.method === 'POST') {
      let body = '';
      req.on('data', chunk => body += chunk);
      req.on('end', () => {
        const data = JSON.parse(body || '{}');
        if (data.block_mode) mockSupportConfig.block_mode = data.block_mode;
        if (data.server_ip) mockSupportConfig.server_ip = data.server_ip;
        if (data.org_name) mockSupportConfig.org_name = data.org_name;
        if (data.support_email) mockSupportConfig.support_email = data.support_email;
        if (data.support_phone) mockSupportConfig.support_phone = data.support_phone;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ success: true }));
      });
      return;
    }
  }

  // Página de Bloqueio de Conteúdo
  if (pathname === '/blocked') {
    const blockedHtmlPath = path.join(STATIC_DIR, 'blocked.html');
    fs.readFile(blockedHtmlPath, (err, data) => {
      if (err) {
        res.writeHead(404, { 'Content-Type': 'text/plain' });
        return res.end('Blocked page not found');
      }
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
      res.end(data);
    });
    return;
  }

  // Relatório por Grupo de Dispositivos (180 dias)
  if (pathname === '/api/v1/reports/group') {
    const groupId = parseInt(parsedUrl.query.group_id || '1', 10);
    const days = parseInt(parsedUrl.query.days || '30', 10);
    const m = Math.max(1, Math.round(days / 10));

    const group = mockGroups.find(g => g.id === groupId) || mockGroups[0];
    const groupClients = mockClients.filter(c => c.group_id === group.id);

    // Mockar dados de cada dispositivo no grupo
    const devices = groupClients.map(c => {
      let domains = [];
      let totalQ = 0;
      let blockedQ = 0;

      if (c.ip === '192.168.1.45') { // Smart TV
        domains = [
          { domain: 'samsungads.com', total_queries: 3150 * m, blocked_queries: 3150 * m, block_rate: 100, last_seen: 'Hoje, 17:45' },
          { domain: 'config.samsungcloudsolution.net', total_queries: 1420 * m, blocked_queries: 1420 * m, block_rate: 100, last_seen: 'Hoje, 17:40' },
          { domain: 'netflix.com', total_queries: 2800 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:20' },
          { domain: 'youtube.com', total_queries: 2100 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 16:15' },
          { domain: 'log-ingestion.samsung.com', total_queries: 980 * m, blocked_queries: 980 * m, block_rate: 100, last_seen: 'Ontem, 22:10' }
        ];
        totalQ = 10450 * m;
        blockedQ = 5550 * m;
      } else if (c.ip === '192.168.1.72') { // iPhone do João
        domains = [
          { domain: 'googleads.g.doubleclick.net', total_queries: 1200 * m, blocked_queries: 1200 * m, block_rate: 100, last_seen: 'Hoje, 17:50' },
          { domain: 'apple.com', total_queries: 3400 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:48' },
          { domain: 'whatsapp.net', total_queries: 4500 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:46' },
          { domain: 'graph.facebook.com', total_queries: 890 * m, blocked_queries: 890 * m, block_rate: 100, last_seen: 'Hoje, 16:30' },
          { domain: 'icloud.com', total_queries: 2100 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 15:10' }
        ];
        totalQ = 12090 * m;
        blockedQ = 2090 * m;
      } else if (c.is_remote && c.auth_token.includes('a8f93e')) { // Notebook Dell Remoto (Trabalho)
        domains = [
          { domain: 'github.com', total_queries: 4200 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:55' },
          { domain: 'telemetry.microsoft.com', total_queries: 1850 * m, blocked_queries: 1850 * m, block_rate: 100, last_seen: 'Hoje, 17:52' },
          { domain: 'slack.com', total_queries: 3100 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:35' },
          { domain: 'segment.io', total_queries: 920 * m, blocked_queries: 920 * m, block_rate: 100, last_seen: 'Hoje, 16:00' },
          { domain: 'aws.amazon.com', total_queries: 2400 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 14:20' }
        ];
        totalQ = 12470 * m;
        blockedQ = 2770 * m;
      } else if (c.ip === '192.168.1.104') { // Chromecast
        domains = [
          { domain: 'netflix.com', total_queries: 3100 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 16:00' },
          { domain: 'youtube.com', total_queries: 2400 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 15:30' },
          { domain: 'fls-na.amazon.com', total_queries: 850 * m, blocked_queries: 850 * m, block_rate: 100, last_seen: 'Hoje, 14:10' }
        ];
        totalQ = 6350 * m;
        blockedQ = 850 * m;
      } else {
        domains = [
          { domain: 'google.com', total_queries: 1500 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 17:00' },
          { domain: 'tiktokcdn.com', total_queries: 820 * m, blocked_queries: 820 * m, block_rate: 100, last_seen: 'Hoje, 15:20' },
          { domain: 'instagram.com', total_queries: 1100 * m, blocked_queries: 0, block_rate: 0, last_seen: 'Hoje, 14:00' }
        ];
        totalQ = 3420 * m;
        blockedQ = 820 * m;
      }

      const rate = totalQ > 0 ? parseFloat(((blockedQ / totalQ) * 100).toFixed(1)) : 0;
      return {
        client_id: c.id,
        client_ip: c.ip,
        client_name: c.name,
        mac: c.mac,
        is_remote: c.is_remote,
        auth_token: c.auth_token,
        total_queries: totalQ,
        blocked_queries: blockedQ,
        block_rate: rate,
        domains: domains
      };
    });

    const sumTotal = devices.reduce((acc, d) => acc + d.total_queries, 0);
    const sumBlocked = devices.reduce((acc, d) => acc + d.blocked_queries, 0);
    const overallRate = sumTotal > 0 ? parseFloat(((sumBlocked / sumTotal) * 100).toFixed(1)) : 0;

    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({
      group: group,
      days: days,
      total_devices: devices.length,
      total_queries: sumTotal,
      blocked_queries: sumBlocked,
      block_rate: overallRate,
      devices: devices
    }));
  }

  // Exportar relatório de grupo para CSV
  if (pathname === '/api/v1/reports/group/export') {
    const groupId = parseInt(parsedUrl.query.group_id || '1', 10);
    const group = mockGroups.find(g => g.id === groupId) || mockGroups[0];
    
    let csv = '\uFEFF"Grupo","Dispositivo","IP/Identificador","Tipo","Dominio","Consultas_Totais","Consultas_Bloqueadas","Ultimo_Acesso"\n';
    const groupClients = mockClients.filter(c => c.group_id === group.id);
    groupClients.forEach(c => {
      csv += `"${group.name}","${c.name}","${c.ip}","${c.is_remote ? 'Remoto/DoH' : 'Rede Local'}","example.com",100,20,"2026-10-13"\n`;
    });

    res.writeHead(200, {
      'Content-Type': 'text/csv; charset=utf-8',
      'Content-Disposition': `attachment; filename="coorpdns_relatorio_grupo_${group.id}.csv"`
    });
    return res.end(csv);
  }

  if (pathname === '/api/v1/reports/top-domains') {
    const days = parseInt(parsedUrl.query.days || '30', 10);
    const multiplier = Math.max(1, Math.round(days / 10));
    const items = [
      { domain: 'googleads.g.doubleclick.net', total_queries: 4200 * multiplier, blocked_queries: 4200 * multiplier, block_rate: 100 },
      { domain: 'samsungads.com', total_queries: 3150 * multiplier, blocked_queries: 3150 * multiplier, block_rate: 100 },
      { domain: 'youtube.com', total_queries: 8900 * multiplier, blocked_queries: 0, block_rate: 0 },
      { domain: 'graph.facebook.com', total_queries: 2800 * multiplier, blocked_queries: 2800 * multiplier, block_rate: 100 },
      { domain: 'netflix.com', total_queries: 4100 * multiplier, blocked_queries: 0, block_rate: 0 },
      { domain: 'telemetry.microsoft.com', total_queries: 2100 * multiplier, blocked_queries: 2100 * multiplier, block_rate: 100 },
      { domain: 'cloudflare.com', total_queries: 3400 * multiplier, blocked_queries: 0, block_rate: 0 },
      { domain: 'app-measurement.com', total_queries: 1850 * multiplier, blocked_queries: 1850 * multiplier, block_rate: 100 },
      { domain: 'github.com', total_queries: 1950 * multiplier, blocked_queries: 0, block_rate: 0 },
    ];
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(items));
  }

  if (pathname === '/api/v1/reports/top-clients') {
    const days = parseInt(parsedUrl.query.days || '30', 10);
    const m = Math.max(1, Math.round(days / 10));
    const items = [
      { client_ip: '192.168.1.45', client_name: 'Smart TV da Sala (Samsung 4K)', total_queries: 9800 * m, blocked_queries: 4200 * m, block_rate: 42.8 },
      { client_ip: '192.168.1.72', client_name: 'iPhone do João', total_queries: 8400 * m, blocked_queries: 1900 * m, block_rate: 22.6 },
      { client_ip: '192.168.1.88', client_name: 'Notebook Dell (Trabalho)', total_queries: 7200 * m, blocked_queries: 1100 * m, block_rate: 15.2 },
      { client_ip: '192.168.1.104', client_name: 'Chromecast Quarto', total_queries: 3400 * m, blocked_queries: 950 * m, block_rate: 27.9 },
      { client_ip: '192.168.1.115', client_name: 'Tablet das Crianças', total_queries: 2800 * m, blocked_queries: 1400 * m, block_rate: 50.0 },
    ];
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(items));
  }

  if (pathname === '/api/v1/reports/client-activity') {
    const clientIP = parsedUrl.query.client_ip;
    const days = parseInt(parsedUrl.query.days || '30', 10);
    const m = Math.max(1, Math.round(days / 10));

    let items = [];
    if (clientIP === '192.168.1.45') {
      items = [
        { domain: 'samsungads.com', total_queries: 3150 * m, blocked_queries: 3150 * m },
        { domain: 'config.samsungcloudsolution.net', total_queries: 1420 * m, blocked_queries: 1420 * m },
        { domain: 'netflix.com', total_queries: 2800 * m, blocked_queries: 0 },
        { domain: 'youtube.com', total_queries: 2100 * m, blocked_queries: 0 },
        { domain: 'log-ingestion.samsung.com', total_queries: 980 * m, blocked_queries: 980 * m },
      ];
    } else {
      items = [
        { domain: 'googleads.g.doubleclick.net', total_queries: 1200 * m, blocked_queries: 1200 * m },
        { domain: 'apple.com', total_queries: 3400 * m, blocked_queries: 0 },
        { domain: 'icloud.com', total_queries: 2100 * m, blocked_queries: 0 },
        { domain: 'graph.facebook.com', total_queries: 890 * m, blocked_queries: 890 * m },
        { domain: 'whatsapp.net', total_queries: 4500 * m, blocked_queries: 0 },
      ];
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(items));
  }

  if (pathname === '/api/v1/reports/timeline') {
    const days = parseInt(parsedUrl.query.days || '30', 10);
    const points = [];
    for (let i = days; i >= 0; i--) {
      const d = new Date();
      d.setDate(d.getDate() - i);
      const total = 1200 + Math.floor(Math.sin(i) * 300) + Math.floor(Math.random() * 200);
      const blocked = Math.floor(total * 0.24);
      points.push({
        date: d.toISOString().split('T')[0],
        total_queries: total,
        blocked_queries: blocked
      });
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(points));
  }

  if (pathname === '/api/v1/logs') {
    const logs = [
      { timestamp: new Date().toISOString(), client_ip: '192.168.1.45', domain: 'samsungads.com', qtype: 'A', status: 'blocked', rule_matched: '||samsungads.com^', upstream: 'sinkhole', latency_ms: 0.2 },
      { timestamp: new Date(Date.now() - 2000).toISOString(), client_ip: '192.168.1.72', domain: 'google.com', qtype: 'A', status: 'allowed', rule_matched: '', upstream: 'cache', latency_ms: 0.1 },
      { timestamp: new Date(Date.now() - 4000).toISOString(), client_ip: '192.168.1.88', domain: 'github.com', qtype: 'AAAA', status: 'allowed', rule_matched: '', upstream: 'https://cloudflare-dns.com/dns-query', latency_ms: 12.4 },
      { timestamp: new Date(Date.now() - 7000).toISOString(), client_ip: '192.168.1.72', domain: 'googleads.g.doubleclick.net', qtype: 'A', status: 'blocked', rule_matched: '||doubleclick.net^', upstream: 'sinkhole', latency_ms: 0.3 },
      { timestamp: new Date(Date.now() - 11000).toISOString(), client_ip: '192.168.1.104', domain: 'netflix.com', qtype: 'HTTPS', status: 'allowed', rule_matched: '', upstream: 'https://dns.quad9.net/dns-query', latency_ms: 18.2 },
    ];
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(logs));
  }

  if (pathname === '/api/v1/clients') {
    updateGroupCounts();
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(mockClients));
  }

  if (pathname === '/api/v1/clients/remote') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      const data = JSON.parse(body || '{}');
      const token = 'tok_' + Math.random().toString(36).substring(2, 12) + Math.random().toString(36).substring(2, 8);
      const targetGroupId = parseInt(data.group_id || '1', 10);
      const group = mockGroups.find(g => g.id === targetGroupId) || mockGroups[0];
      const newClient = {
        id: Date.now(),
        ip: 'remote-' + token.substring(0, 10),
        name: data.name || 'Dispositivo Remoto',
        comment: data.comment || '',
        mac: '',
        block_enabled: true,
        is_remote: true,
        auth_token: token,
        status: 'active',
        group_id: group.id,
        group_name: group.name
      };
      mockClients.unshift(newClient);
      updateGroupCounts();
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify(newClient));
    });
    return;
  }

  if (pathname === '/api/v1/clients/revoke') {
    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {
      const data = JSON.parse(body || '{}');
      const found = mockClients.find(c => c.auth_token === data.token);
      if (found) found.status = 'revoked';
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ success: true }));
    });
    return;
  }

  if (pathname.startsWith('/dns-query')) {
    const token = pathname.replace('/dns-query', '').replace(/^\//, '');
    const client = mockClients.find(c => c.auth_token === token && c.status === 'active');
    if (!token || !client) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      return res.end(JSON.stringify({ error: 'CoorpDNS Cloud: Não autorizado. Token corporativo inválido ou revogado.' }));
    }
    res.writeHead(200, { 'Content-Type': 'application/dns-message' });
    return res.end(Buffer.from([0, 0, 129, 128, 0, 1, 0, 1, 0, 0, 0, 0])); // DNS response stub
  }

  if (pathname === '/api/v1/rules') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify({ custom_rules: mockCustomRules, allow_rules: mockAllowRules }));
  }

  if (pathname === '/api/v1/lists') {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    return res.end(JSON.stringify(mockBlocklists));
  }

  // Arquivos estáticos
  let filePath = path.join(STATIC_DIR, pathname === '/' ? 'index.html' : pathname);
  const ext = path.extname(filePath);
  const mimeTypes = {
    '.html': 'text/html',
    '.css': 'text/css',
    '.js': 'application/javascript',
    '.svg': 'image/svg+xml',
    '.json': 'application/json',
  };

  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404, { 'Content-Type': 'text/plain' });
      res.end('404 Not Found');
      return;
    }
    res.writeHead(200, { 'Content-Type': mimeTypes[ext] || 'text/plain' });
    res.end(data);
  });
});

server.listen(PORT, () => {
  console.log(`[CoorpDNS Mock Server] Rodando em http://localhost:${PORT}`);
});

