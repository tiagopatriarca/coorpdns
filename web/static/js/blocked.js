/**
 * CoorpDNS - Blocked Page Script (CSP Compliant - External JS)
 */
(function () {
  const categoryMap = {
    adult: { name: 'Conteúdo Adulto / Pornografia', icon: 'fa-ban', color: '#ef4444' },
    gambling: { name: 'Apostas & Jogos de Azar', icon: 'fa-dice', color: '#f59e0b' },
    social: { name: 'Redes Sociais', icon: 'fa-share-nodes', color: '#3b82f6' },
    gaming: { name: 'Jogos & Games Online', icon: 'fa-gamepad', color: '#8b5cf6' },
    streaming: { name: 'Streaming de Vídeo & Música', icon: 'fa-film', color: '#ec4899' },
    p2p_torrent: { name: 'P2P, Torrents & Pirataria', icon: 'fa-magnet', color: '#64748b' },
    crypto: { name: 'Criptomoedas & Mineração', icon: 'fa-coins', color: '#eab308' },
    dating: { name: 'Relacionamentos & Encontros', icon: 'fa-heart', color: '#f43f5e' },
    trackers: { name: 'Rastreadores & Telemetria', icon: 'fa-eye-slash', color: '#6366f1' },
    ads: { name: 'Anúncios & Publicidade', icon: 'fa-bullhorn', color: '#14b8a6' },
  };

  const urlParams = new URLSearchParams(window.location.search);
  const domain = urlParams.get('domain') || window.location.hostname || 'site-restrito.com';
  let category = urlParams.get('category') || '';
  const group = urlParams.get('group') || 'Estações de Trabalho Corporativas';
  const client = urlParams.get('client') || 'Dispositivo da Rede Local';

  if (!category) {
    if (/bet365|betano|blaze|betfair|poker|cassino/i.test(domain)) category = 'gambling';
    else if (/pornhub|xvideos|xhamster|onlyfans/i.test(domain)) category = 'adult';
    else if (/tiktok|instagram|facebook|twitter|reddit/i.test(domain)) category = 'social';
    else if (/roblox|steam|epicgames|discord|twitch/i.test(domain)) category = 'gaming';
    else if (/netflix|youtube|disney/i.test(domain)) category = 'streaming';
    else if (/torrent|piratebay/i.test(domain)) category = 'p2p_torrent';
    else category = 'ads';
  }

  const catInfo = Object.prototype.hasOwnProperty.call(categoryMap, category)
    ? categoryMap[category]
    : { name: 'Política de Bloqueio', icon: 'fa-ban', color: '#ef4444' };

  let supportEmail = 'suporte@empresa.com.br';
  let supportPhone = '5511999998888';

  const domainEl = document.getElementById('displayBlockedDomain');
  if (domainEl) domainEl.textContent = domain;

  const catBadgeEl = document.getElementById('displayCategoryBadge');
  if (catBadgeEl) {
    catBadgeEl.textContent = '';
    const iconEl = document.createElement('i');
    iconEl.className = `fa-solid ${catInfo.icon}`;
    catBadgeEl.appendChild(iconEl);
    catBadgeEl.appendChild(document.createTextNode(` ${catInfo.name}`));
    catBadgeEl.style.borderColor = catInfo.color;
    catBadgeEl.style.color = catInfo.color;
    catBadgeEl.style.background = `${catInfo.color}15`;
  }

  const groupBadgeEl = document.getElementById('displayGroupBadge');
  if (groupBadgeEl) groupBadgeEl.textContent = group;

  const clientEl = document.getElementById('displayClientIP');
  if (clientEl) clientEl.textContent = client;

  const tsEl = document.getElementById('displayTimestamp');
  if (tsEl) tsEl.textContent = new Date().toLocaleString('pt-BR');

  fetch('/api/v1/config/support')
    .then(res => res.json())
    .then(cfg => {
      if (cfg.support_email) supportEmail = cfg.support_email;
      if (cfg.support_phone) supportPhone = cfg.support_phone;
      if (cfg.org_name) {
        const orgEl = document.getElementById('displayOrgName');
        if (orgEl) {
          orgEl.textContent = '';
          const icon = document.createElement('i');
          icon.className = 'fa-solid fa-building-shield';
          orgEl.appendChild(icon);
          orgEl.appendChild(document.createTextNode(` ${cfg.org_name}`));
        }
      }
      updateActionLinks();
    })
    .catch(() => updateActionLinks());

  function updateActionLinks() {
    const waBtn = document.getElementById('btnWhatsAppSupport');
    if (waBtn) {
      const waText = encodeURIComponent(
        `Olá! Meu acesso ao site "${domain}" (Categoria: ${catInfo.name}) foi bloqueado na rede corporativa no dispositivo ${client}. Gostaria de solicitar a análise e liberação.`
      );
      waBtn.href = `https://wa.me/${supportPhone.replace(/\D/g, '')}?text=${waText}`;
    }

    const emailBtn = document.getElementById('btnEmailSupport');
    if (emailBtn) {
      const emailSubject = encodeURIComponent(`[CoorpDNS] Solicitação de Liberação de Site: ${domain}`);
      const emailBody = encodeURIComponent(
        `Olá equipe de suporte,\n\nSolicito a análise e liberação do domínio abaixo bloqueado pelas diretrizes corporativas:\n\n` +
        `• Domínio: ${domain}\n` +
        `• Categoria Bloqueada: ${catInfo.name}\n` +
        `• Grupo do Aparelho: ${group}\n` +
        `• Dispositivo/IP: ${client}\n` +
        `• Data/Hora: ${new Date().toLocaleString('pt-BR')}\n\n` +
        `Justificativa para liberação:\n`
      );
      emailBtn.href = `mailto:${encodeURIComponent(supportEmail)}?subject=${emailSubject}&body=${emailBody}`;
    }
  }

  function copyIncidentDetails() {
    const text = `=== INCIDENTE DE BLOQUEIO COORPDNS ===\nDomínio: ${domain}\nCategoria: ${catInfo.name}\nGrupo: ${group}\nDispositivo: ${client}\nData/Hora: ${new Date().toISOString()}\n`;
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(text).then(() => {
        const toast = document.getElementById('copyToast');
        if (toast) {
          toast.style.display = 'block';
          setTimeout(() => { toast.style.display = 'none'; }, 3000);
        }
      });
    }
  }

  const btnCopy = document.getElementById('btnCopyIncident');
  if (btnCopy) btnCopy.addEventListener('click', copyIncidentDetails);

  const btnBack = document.getElementById('btnGoBack');
  if (btnBack) btnBack.addEventListener('click', () => window.history.back());
})();
