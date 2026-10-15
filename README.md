# 🛡️ CoorpDNS - Servidor DNS Corporativo, Firewall de Conteúdo & Auditoria de Rede (180 Dias)

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows%20%7C%20Docker-blue)](https://github.com/tiagopatriarca/coorpdns)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Traefik](https://img.shields.io/badge/Traefik-v2%20%2F%20v3-24A1C1?logo=traefik)](https://traefik.io)

Plataforma corporativa autônoma e de alto desempenho desenvolvida em **Go (Golang 1.22)** com painel web moderno em **Vanilla (HTML5 / CSS3 / ES6+)** para segurança da informação, conformidade (compliance), bloqueio de ameaças/anúncios, filtragem por categorias e auditoria completa de navegação em empresas, escritórios, filiais e estações de trabalho remotas.

---

## 📑 Sumário

- [Visão Geral & Diferenciais](#-principais-diferenciais-corporativos)
- [Autenticação & Controle de Acesso por Grupos (RBAC)](#-autenticação--controle-de-acesso-por-grupos-rbac)
- [Instalação em 1 Linha (PowerShell / Endpoint Shield)](#-instalação-em-1-linha-no-windows-powershell)
- [Gráfico de Atividade em Tempo Real (24 Horas)](#-gráfico-de-atividade-em-tempo-real-últimas-24h)
- [Políticas de Categorias por Departamentos](#-políticas-de-categorias-por-grupos-de-dispositivos)
- [Página de Bloqueio & Alerta da TI](#-página-de-bloqueio--alerta-da-ti-splash-page-corporativa)
- [Integração com Active Directory (AD DS)](#-integração-com-active-directory-ad-ds)
- [Deploy com Docker & Traefik v3 (Nuvem / VPS)](#-deploy-em-produção-com-traefik-v3-e-lets-encrypt)
- [Auditoria e Relatórios de Conformidade (180 Dias)](#-auditoria-e-relatórios-de-conformidade-180-dias)
- [Endpoints da API REST](#-principais-endpoints-da-api-rest)

---

## ✨ Principais Diferenciais Corporativos

- **Autenticação Segura & RBAC por Departamentos**: Gestão de usuários com login individualizado. Usuários do perfil **Operador** têm visão estritamente restrita aos grupos autorizados, sem acesso a dados, relatórios ou dispositivos de outros setores.
- **Zero Instalação nas Redes Locais**: Aponte o IP do CoorpDNS no DHCP do Firewall ou Switch Core L3 (Fortinet, pfSense, Mikrotik, Cisco, Sophos, Windows Server DHCP) e todas as estações de trabalho e dispositivos móveis da empresa ficam imediatamente protegidos e auditados.
- **Instalação Silenciosa em 1 Linha para Estações Remotas**: Qualquer notebook fora da empresa é integrado em segundos via PowerShell sem necessidade de instalador complexo ou configuração manual de adaptador.
- **Proteção Completa via DNS-over-HTTPS (DoH)**: O **CoorpDNS Agent** tunela e criptografa o tráfego de notebooks em trânsito com **tokens exclusivos** por dispositivo. A porta UDP 53 não precisa ser exposta para a internet pública na VPS, prevenindo ataques de amplificação DDoS.
- **Identidade Visual Integrada**: Favicon SVG oficial do escudo de proteção na aba do navegador e painel em glassmorphism moderno.
- **Gráfico de Consultas Horário (Últimas 24h)**: Visualização visual e interativa com curvas em gradiente, dividida hora a hora para acompanhamento em tempo real de picos de tráfego e ameaças.
- **Resiliência e Desempenho com SQLite em Modo WAL**: Pool otimizado para concorrência de até 25 conexões simultâneas, sem deadlocks ou bloqueios de banco de dados, com retenção histórica de até 180 dias.

---

## 🔐 Autenticação & Controle de Acesso por Grupos (RBAC)

O CoorpDNS possui sistema completo de autenticação e governança corporativa multi-tenant por departamentos:

### 1. Credenciais Iniciais de Administrador

> [!IMPORTANT]
> **Acesso Inicial ao Painel Web:**
> * **Usuário Padrão:** `admin`
> * **Senha Padrão:** `admin`
> 
> *Por motivos de segurança corporativa, as credenciais padrão não são exibidas na tela de login da aplicação. Altere a senha do usuário `admin` na aba **Usuários** logo após a primeira autenticação.*

### 2. Perfis de Usuário
| Perfil | Acesso ao Sistema | Grupos de Dispositivos | Gerenciamento de Usuários |
| :--- | :--- | :--- | :--- |
| 👑 **Administrador** | Irrestrito | Acesso a **todos** os grupos do sistema | Pode cadastrar, editar e excluir usuários |
| 🛡️ **Operador** | Restrito | Acesso **apenas** aos grupos marcados no seu cadastro | Não acessa a tela de usuários nem configurações globais |

### 3. Isolamento Estrito de Dados
* Operadores **não conseguem visualizar**, consultar relatórios, inspecionar logs ou cadastrar dispositivos em grupos aos quais não tenham permissão explícita.
* No cadastro de dispositivos remotos e estações locais, o seletor exibe exclusivamente os grupos permitidos ao operador autenticado.
* Todas as rotas de API validam no backend a sessão criptográfica (armazenada em cookies seguros e tokens com hash SHA-256 com salt).

---

## ⚡ Instalação em 1 Linha no Windows (PowerShell)

Para proteger estações de trabalho remotas, notebooks corporativos e colaboradores em Home Office, o CoorpDNS disponibiliza um comando autônomo que realiza toda a configuração automaticamente:

### Passo a Passo:

1. No menu Iniciar do Windows, pesquise por **PowerShell**, clique com o botão direito e selecione **"Executar como Administrador"**.
2. Cole o comando correspondente ao token do dispositivo e pressione Enter:

```powershell
irm https://dns.tiplus.inf.br/install.ps1?token=tok_SEU_TOKEN_AQUI | iex
```

### O que o instalador e o agente fazem automaticamente:
* 🔒 **Verificação de Permissões**: Garante execução com privilégios de Administrador.
* 🛑 **Encerramento Limpo**: Finaliza instâncias anteriores para evitar arquivos em uso.
* 📦 **Download Seguro**: Baixa o binário oficial `coorpdns-agent.exe` diretamente para `C:\Program Files\CoorpDNS`.
* ⚙️ **Agendador do Windows (Task Scheduler)**: Registra o serviço para iniciar automaticamente com o Windows como `SYSTEM` no boot do sistema.
* 🚀 **Execução em Segundo Plano**: Inicia o agente de forma silenciosa (`-WindowStyle Hidden`), sem deixar janelas abertas na tela do colaborador.
* 🌐 **Apontamento de Rede**: Configura as placas de rede ativas com o DNS local `127.0.0.1` apontado para o agente.
* 🔄 **Autocura Contínua & Proteção de Roaming (Zero-Downtime)**: Quando o colaborador se desloca com o notebook sem reiniciar e conecta em outra rede Wi-Fi/cabo, o agente Go detecta via goroutine (ciclo de 5s) a sobrescrita do DNS pelo DHCP local e reaplica `127.0.0.1` imediatamente nas placas ativas.
* 📡 **Resiliência a Captive Portals (Hotéis e Aeroportos)**: O agente captura dinamicamente o DNS do roteador local antes de forçar o loopback e armazena como contingência dinâmica. Se a nuvem DoH estiver bloqueada por portal de login, o fallback local resolve a tela de login para liberar a navegação.
* 🧹 **Graceful Shutdown**: Em desinstalações ou paradas controladas do agente, as interfaces de rede são restauradas automaticamente para DHCP (`ResetServerAddresses`).

> [!TIP]
> Na aba **Clientes & Grupos** do painel web, basta clicar no botão **`⚡ Instalar`** ao lado do dispositivo desejado para copiar o comando personalizado na hora.

---

## 📊 Gráfico de Atividade em Tempo Real (Últimas 24h)

O painel principal conta com monitoramento em tempo real do tráfego:

* **Série Temporal de 24 Horas**: Exibição contínua dividida de hora em hora das últimas 24 horas (`hours=24`).
* **Design em Gradiente de Área**:
  * 🟢 **Verde Esmeralda**: Volume total de consultas permitidas.
  * 🔴 **Vermelho**: Consultas bloqueadas por políticas ou listas de proteção.
* **Escala Dinâmica com Headroom**: Ajuste automático da escala vertical com margem superior para que as curvas nunca encostem no topo.
* **Pontos Interativos**: Tooltips ao passar o mouse com contagem detalhada de consultas permitidas e bloqueadas em cada hora.

---

## 🛡️ Políticas de Categorias por Grupos de Dispositivos

Crie grupos departamentais (*Diretoria*, *Financeiro*, *Desenvolvimento/TI*, *Operação*, *Vendas*) e controle granularmente o que cada equipe pode acessar:

| Categoria | Descrição / Exemplos |
| :--- | :--- |
| 🔞 **Conteúdo Adulto** | Sites pornográficos, material explícito e OnlyFans |
| 🎲 **Apostas & Jogos de Azar** | Casas de apostas, bets, cassinos virtuais e loterias |
| 📱 **Redes Sociais** | TikTok, Instagram, Facebook, Twitter/X, Reddit |
| 🎮 **Jogos & Games Online** | Steam, Roblox, Epic Games, Twitch, Discord |
| 🎬 **Streaming de Vídeo & Música** | Netflix, YouTube, Spotify, Disney+, Prime Video |
| 🧲 **P2P, Torrents & Pirataria** | ThePirateBay, 1337x, trackers torrent e downloads ilegais |
| 🪙 **Criptomoedas & Mineração** | Binance, corretoras cripto e mineradores web |
| 💘 **Relacionamentos & Encontros** | Tinder, Badoo, Happn, Grindr, Bumble |
| 👁️ **Rastreadores & Telemetria** | Telemetria invasiva de sistemas e coletores de dados |
| 📢 **Anúncios & Publicidade** | Redes de anúncios, banners, popups e servidores de tracking |

> As políticas de categorias são avaliadas em tempo real tanto para clientes da rede local quanto para clientes remotos conectados via DoH (`coorpdns-agent`).

---

## 🚫 Página de Bloqueio & Alerta da TI (Splash Page Corporativa)

Quando uma estação de trabalho tenta acessar um domínio bloqueado, o CoorpDNS redireciona o colaborador para uma página institucional personalizada:

* **Identificação do Incidente**: Exibe o domínio restrito, o nome da categoria corporativa, o grupo departamental e o IP/identificador da máquina.
* **Canais de Liberação de Acesso**:
  * 💬 **Falar no WhatsApp Corporativo**: Abre conversa com a equipe de suporte com mensagem pré-formatada contendo os dados do bloqueio.
  * ✉️ **Solicitar por E-mail**: Abre cliente de e-mail com modelo de chamado corporativo estruturado.
  * 📋 **Copiar Dados do Incidente**: Copia relatório estruturado com carimbo de data/hora para a área de transferência.

---

## 🏢 Integração com Active Directory (AD DS)

| Arquitetura | Funcionamento | Vantagem |
| :--- | :--- | :--- |
| **Encaminhamento Condicional [Recomendado]** | As estações recebem o CoorpDNS como DNS primário via DHCP. No CoorpDNS, a zona interna (ex: `empresa.local`) é apontada para o IP do Domain Controller. | Autenticação Kerberos, GPOs e logon instantâneos + Auditoria individualizada por máquina para a internet. |
| **Forwarder no Windows Server DNS** | As estações apontam para o DC. No Windows Server DNS, o CoorpDNS é configurado como encaminhador externo. | Não altera o fluxo de rede existente, mas as consultas externas chegam ao CoorpDNS com o IP do DC. |

---

## 🚀 Deploy em Produção com Traefik v3 e Let's Encrypt

Para hospedar o CoorpDNS na nuvem com certificado SSL automático e roteamento de bloqueio compatível com Traefik v3:

### `docker-compose.traefik.yml`
```yaml
version: '3.8'

services:
  coorpdns:
    build:
      context: .
      dockerfile: Dockerfile
    image: coorpdns:latest
    container_name: coorpdns
    restart: unless-stopped
    environment:
      - TZ=America/Sao_Paulo
    volumes:
      - ./data:/app/data
      - ./config:/app/config
      - ./downloads:/app/downloads:ro
    expose:
      - "8080"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.coorpdns.rule=Host(`dns.suaempresa.com.br`)"
      - "traefik.http.routers.coorpdns.entrypoints=websecure"
      - "traefik.http.routers.coorpdns.tls.certresolver=letsencrypt"
      - "traefik.http.routers.coorpdns.priority=100"
      - "traefik.http.services.coorpdns.loadbalancer.server.port=8080"
      - "traefik.http.routers.coorpdns-blocked.rule=HostRegexp(`^.+$`)"
      - "traefik.http.routers.coorpdns-blocked.entrypoints=web,websecure"
      - "traefik.http.routers.coorpdns-blocked.tls=true"
      - "traefik.http.routers.coorpdns-blocked.priority=1"
      - "traefik.http.routers.coorpdns-blocked.service=coorpdns"
```

### Inicialização:
```bash
docker compose -f docker-compose.traefik.yml up -d --build
```

---

## 📈 Auditoria e Relatórios de Conformidade (180 Dias)

* 🌐 **Top Sites Mais Acessados na Empresa**: Ranking com volume, status (permitido/bloqueado) e taxa percentual de bloqueio.
* 🖥️ **Top Máquinas com Maior Consumo**: Identificação de estações infectadas, telemetria excessiva ou desvios de conduta.
* 🔍 **Matriz Máquina $\times$ Sites**: Rastreabilidade completa de navegação com filtros por data, domínio e estação de trabalho.
* 👥 **Relatório por Grupo Departamental**: Análise comparativa do perfil de navegação de cada setor da empresa.
* 📥 **Exportação em CSV**: Relatórios prontos para auditorias externas (LGPD, ISO 27001 e SOC2).

---

## 🔌 Principais Endpoints da API REST

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| `POST` | `/api/v1/auth/login` | Autentica usuário e inicia sessão segura (`admin` / `admin`). |
| `POST` | `/api/v1/auth/logout` | Encerra a sessão ativa e revoga o cookie seguro. |
| `GET` | `/api/v1/auth/me` | Retorna o usuário logado e a lista de grupos permitidos. |
| `GET` | `/api/v1/users` | Lista todos os usuários cadastrados e seus grupos (Admin). |
| `POST` | `/api/v1/users` | Cadastra novo usuário vinculando grupos autorizados (Admin). |
| `PUT` | `/api/v1/users/{id}` | Atualiza dados, senha e grupos de um usuário (Admin). |
| `DELETE` | `/api/v1/users/{id}` | Remove um usuário do sistema (Admin). |
| `GET` | `/api/v1/status` | Estado operacional, uptime e volume de regras ativas. |
| `POST` | `/api/v1/control/toggle` | Habilita, desabilita ou pausa temporariamente a proteção. |
| `GET` | `/api/v1/stats?days=1` | Métricas gerais de consultas, bloqueios e clientes ativos. |
| `GET` | `/api/v1/reports/timeline?hours=24` | Série temporal das últimas 24h agrupada por hora. |
| `GET` | `/api/v1/reports/timeline?days=180` | Série temporal diária de até 180 dias. |
| `GET` | `/api/v1/reports/top-domains?days=30` | Domínios mais requisitados no período. |
| `GET` | `/api/v1/reports/top-clients?days=30` | Dispositivos com maior volume de requisições. |
| `GET` | `/api/v1/reports/client-activity?client_ip=...` | Histórico detalhado de sites acessados por uma estação. |
| `GET` | `/api/v1/groups/categories?group_id={id}` | Categorias bloqueadas para o grupo especificado. |
| `PUT` | `/api/v1/groups/categories` | Atualiza regras de categorias do grupo. |
| `GET` | `/install.ps1?token={token}` | Script PowerShell de instalação automatizada em 1 linha. |
| `GET` | `/blocked?domain=...&category=...` | Splash page de aviso e suporte corporativo. |
| `GET` | `/downloads/coorpdns-agent.exe` | Download direto do agente oficial para Windows x64. |
| `GET` | `/favicon.svg` | Ícone oficial de escudo de proteção para navegadores. |

---

## 📄 Licença

Distribuído sob a licença **MIT**. Consulte `LICENSE` para mais informações.
