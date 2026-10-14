# 🛡️ CoorpDNS - Servidor DNS Corporativo, Firewall de Conteúdo & Auditoria de Rede (180 Dias)

Plataforma corporativa autônoma e de alto desempenho desenvolvida em **Go (Golang 1.22)** com painel web moderno em **Vanilla (HTML5 / CSS3 / ES6+)** para segurança da informação, conformidade (compliance), bloqueio de ameaças/anúncios e auditoria completa de navegação em empresas, escritórios, filiais e estações de trabalho remotas.

---

## ✨ Principais Diferenciais Corporativos

- **Zero Instalação nas Redes Locais**: Aponte o DNS no DHCP do Firewall ou Switch Core L3 (Fortinet, pfSense, Mikrotik, Cisco, Sophos, Windows Server DHCP) e todas as estações de trabalho e dispositivos móveis da empresa ficam imediatamente protegidos e auditados.
- **Integração Nativa com Active Directory (AD DS) & Domínios Internos**:
  - **Encaminhamento Condicional (Conditional Forwarding)**: Consultas para registros internos do AD (`_ldap._tcp.dc._msdcs.empresa.local`, `_kerberos`, `kpasswd` e zona reversa `in-addr.arpa`) são encaminhadas diretamente aos Domain Controllers Windows Server.
  - O logon Kerberos, GPOs, compartilhamentos SMB e autenticações continuam funcionando perfeitamente sem interrupções, enquanto todo o tráfego de internet passa pelo filtro de segurança e relatórios individualizados por máquina.
- **Políticas de Categorias por Grupos de Dispositivos**:
  - Crie grupos departamentais (*Diretoria & Executivos*, *Financeiro & Administrativo*, *Desenvolvimento & TI*, *Geral / Padrão Corporativo*).
  - Defina com granularidade quais categorias corporativas são bloqueadas ou liberadas por grupo:
    - 🔞 **Conteúdo Adulto / Pornografia**
    - 🎲 **Apostas & Jogos de Azar (Bets/Cassinos)**
    - 📱 **Redes Sociais & Mídias de Distração**
    - 🎮 **Jogos & Games Online**
    - 🎬 **Streaming de Vídeo & Música**
    - 🧲 **P2P, Torrents & Pirataria**
    - 🪙 **Criptomoedas & Mineração Não Autorizada**
    - 💘 **Relacionamentos & Encontros**
    - 👁️ **Rastreadores, Telemetria & Spyware**
    - 📢 **Anúncios, Adware & Malvertising**
- **Página de Bloqueio & Alerta da TI (Splash Page Corporativa)**:
  - Quando um colaborador tenta acessar um domínio bloqueado pela política, o CoorpDNS exibe uma página de aviso visual informando o motivo, a categoria restrita, o IP/dispositivo e o grupo do colaborador.
  - Oferece botões rápidos para entrar em contato com o suporte de TI via **WhatsApp Corporativo** ou **E-mail**, com chamado pré-formatado contendo todos os detalhes do incidente para aprovação.
- **Auditoria e Relatórios de Conformidade (Histórico de 180 Dias)**:
  - 🌐 **Top Sites Mais Acessados na Empresa**: Ranking com volume, status e ação rápida.
  - 🖥️ **Top Máquinas que Mais Consomem**: Identificação de gargalos, infecções por malware e telemetria excessiva por estação de trabalho.
  - 🔍 **Matriz Máquina $\times$ Sites Acessados**: Cruzamento completo com retenção de 6 meses para auditorias trabalhistas e de segurança.
  - 👥 **Relatório por Grupo Departamental**: Visualize o comportamento agregado do departamento e os acessos de cada colaborador.
  - 📥 **Exportação em CSV** para relatórios de diretoria e auditorias de conformidade (LGPD / SOC2).
- **Proteção de Força de Trabalho Remota (Home Office & Filiais)**:
  - O **CoorpDNS Agent** tunela consultas de notebooks em trânsito ou home office diretamente para o CoorpDNS na nuvem via **DNS-over-HTTPS (DoH)** autenticado com token exclusivo.
  - A porta UDP 53 não precisa ser exposta para a internet pública na VPS, prevenindo ataques DDoS por amplificação DNS.

---

## 🚀 Como Executar

### Opção 1: Via Docker Compose (Rede Local / Data Center On-Premises)

1. Clone ou copie a pasta do projeto para o servidor local (Ubuntu Server, Debian, Rocky Linux ou Proxmox):
```bash
docker compose up -d --build
```
2. Acesse o painel pelo navegador:
```
http://IP_DO_SERVIDOR:8080
```

> [!TIP]
> Em servidores Linux corporativos, `network_mode: host` é o recomendado no `docker-compose.yml` para que o servidor identifique o IP exato de cada workstation na LAN.

---

### Opção 2: Pré-visualização Local do Painel (Demonstração / Testes Rápidos)

Para visualizar e validar a interface web, relatórios e a página de bloqueio:

```bash
node preview_server.js
```
Acesse: **[http://localhost:8080](http://localhost:8080)**

---

### Opção 3: Deploy em Nuvem (VPS Corporativa com DoH & Token Autorizado)

Para proteger filiais e colaboradores remotos via nuvem com HTTPS automático (Let's Encrypt):

1. Aponte seu subdomínio corporativo (ex: `dns.suaempresa.com.br`) para o IP da VPS.
2. Configure o `Caddyfile` com seu domínio.
3. Inicie os serviços isolados:
```bash
docker compose -f docker-compose.cloud.yml up -d --build
```

---

## 🌐 Conexão de Laptops Remotos (Home Office)

1. No painel web do CoorpDNS, vá na aba **Dispositivos** e clique em **`+ Cadastrar Dispositivo Remoto`**.
2. Defina o identificador do colaborador (ex: `Notebook Dell - Maria Finanças`) e vincule ao grupo desejado.
3. Copie o comando do **CoorpDNS Agent**:
```bash
./coorpdns-agent -server https://dns.suaempresa.com.br -token tok_a8f93e2b1049c3
```
*O tráfego DNS do notebook será criptografado e auditado centralmente nas políticas da empresa.*

---

## 🏢 Integração com Active Directory (AD DS)

| Arquitetura | Funcionamento | Vantagem |
| :--- | :--- | :--- |
| **Encaminhamento Condicional no CoorpDNS [Padrão Recomendado]** | As estações recebem o IP do CoorpDNS como DNS primário via DHCP. No CoorpDNS, o domínio `empresa.local` é apontado para o IP do Domain Controller. | Autenticação AD, Kerberos e GPOs instantâneos + Auditoria individualizada por máquina para a internet. |
| **Forwarder no Windows Server DNS** | As estações apontam para o DC. No Windows Server DNS, o CoorpDNS é configurado como encaminhador externo. | Não altera o fluxo de rede existente, mas as consultas externas chegam ao CoorpDNS com o IP do DC. |

---

## 🔌 Principais Endpoints da API REST

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| `GET` | `/api/v1/status` | Estado operacional, uptime e volume de regras ativas no CoorpDNS. |
| `POST` | `/api/v1/control/toggle` | Habilita, desabilita ou pausa temporariamente a proteção. |
| `GET` | `/api/v1/reports/top-domains?days=180` | Ranking dos domínios mais requisitados nos últimos 180 dias. |
| `GET` | `/api/v1/reports/top-clients?days=180` | Ranking das estações com maior tráfego. |
| `GET` | `/api/v1/reports/client-activity?client_ip=...&days=180` | Auditoria detalhada de acessos por estação de trabalho. |
| `GET` | `/api/v1/reports/group?group_id=...&days=180` | Relatório consolidado por grupo departamental. |
| `GET` | `/api/v1/groups/categories?group_id={id}` | Políticas de bloqueio por categoria do grupo. |
| `PUT` | `/api/v1/groups/categories` | Atualiza regras de categorias (adulto, apostas, redes sociais, etc.). |
| `GET` | `/api/v1/forwarders` | Zonas e Domain Controllers configurados no Encaminhamento Condicional. |
| `GET` | `/blocked?domain=...&category=...` | Splash page de alerta ao colaborador com botões de suporte. |
| `GET` | `/api/v1/config/support` | Configurações de contato da TI e modo de resposta do sinkhole. |
