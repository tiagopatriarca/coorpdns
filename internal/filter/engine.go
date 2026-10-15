package filter

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type CategoryInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
}

var AvailableCategories = []CategoryInfo{
	{ID: "adult", Name: "Conteúdo Adulto / Pornografia", Description: "Sites adultos, pornografia, nudez e conteúdo explícito", Icon: "fas fa-ban", Color: "#ef4444"},
	{ID: "gambling", Name: "Apostas & Jogos de Azar", Description: "Cassinos online, apostas esportivas (Bet365, Betano, Blaze)", Icon: "fas fa-dice", Color: "#f59e0b"},
	{ID: "social", Name: "Redes Sociais", Description: "Instagram, TikTok, Facebook, Twitter/X, Reddit, Snapchat", Icon: "fas fa-share-alt", Color: "#3b82f6"},
	{ID: "gaming", Name: "Jogos & Games Online", Description: "Steam, Roblox, Epic Games, PlayStation, Xbox, Discord, Twitch", Icon: "fas fa-gamepad", Color: "#8b5cf6"},
	{ID: "streaming", Name: "Streaming de Vídeo & Música", Description: "Netflix, YouTube, Disney+, Prime Video, Spotify, HBO", Icon: "fas fa-film", Color: "#ec4899"},
	{ID: "p2p_torrent", Name: "P2P, Torrents & Pirataria", Description: "ThePirateBay, 1337x, trackers torrent, clientes BitTorrent", Icon: "fas fa-magnet", Color: "#64748b"},
	{ID: "crypto", Name: "Criptomoedas & Mineração", Description: "Corretoras cripto (Binance, Coinbase) e mineradores web", Icon: "fas fa-coins", Color: "#eab308"},
	{ID: "dating", Name: "Relacionamentos & Encontros", Description: "Tinder, Badoo, Happn, Grindr, Bumble", Icon: "fas fa-heart", Color: "#f43f5e"},
	{ID: "trackers", Name: "Rastreadores & Telemetria", Description: "Telemetria de sistemas, métricas invasivas e rastreamento de comportamento", Icon: "fas fa-eye-slash", Color: "#6366f1"},
	{ID: "ads", Name: "Anúncios & Publicidade", Description: "Redes de anúncios, banners, popups e servidores de publicidade", Icon: "fas fa-bullhorn", Color: "#14b8a6"},
}

// Domínios base de cada categoria para detecção rápida e precisa
var categoryDatabase = map[string][]string{
	"adult": {
		"xvideos.com", "pornhub.com", "xhamster.com", "onlyfans.com", "redtube.com", "chaturbate.com",
		"brazzers.com", "youporn.com", "cam4.com", "stripchat.com", "livejasmin.com", "erome.com",
	},
	"gambling": {
		"bet365.com", "betano.com", "blaze.com", "sportingbet.com", "betfair.com", "pokerstars.com",
		"kto.com", "apostaganha.bet", "pixbet.com", "1xbet.com", "stake.com", "estrelabet.com", "betnacional.com",
	},
	"social": {
		"tiktok.com", "instagram.com", "facebook.com", "fb.com", "twitter.com", "x.com",
		"reddit.com", "snapchat.com", "linkedin.com", "pinterest.com", "threads.net", "kwai.com",
	},
	"gaming": {
		"roblox.com", "steampowered.com", "steamcommunity.com", "epicgames.com", "riotgames.com",
		"blizzard.com", "discord.com", "discord.gg", "twitch.tv", "playstation.com", "xbox.com", "ea.com",
	},
	"streaming": {
		"netflix.com", "youtube.com", "googlevideo.com", "disneyplus.com", "primevideo.com",
		"spotify.com", "hbomax.com", "max.com", "deezer.com", "crunchyroll.com", "paramountplus.com",
	},
	"p2p_torrent": {
		"thepiratebay.org", "1337x.to", "rarbg.to", "yts.mx", "torrentz2.eu", "nyaa.si",
		"rutracker.org", "bittorrent.com", "utorrent.com", "magnetdl.com",
	},
	"crypto": {
		"binance.com", "coinbase.com", "kraken.com", "coindesk.com", "coinmarketcap.com",
		"nicehash.com", "crypto.com", "bybit.com", "kucoin.com", "coingecko.com",
	},
	"dating": {
		"tinder.com", "badoo.com", "happn.com", "grindr.com", "bumble.com",
		"match.com", "okcupid.com", "innercircle.co", "ashleymadison.com",
	},
	"trackers": {
		"telemetry.microsoft.com", "analytics.google.com", "graph.facebook.com",
		"log-ingestion.samsung.com", "app-measurement.com", "scorecardresearch.com",
		"segment.io", "hotjar.com", "clarity.ms", "fls-na.amazon.com",
	},
	"ads": {
		"doubleclick.net", "googleads.g.doubleclick.net", "samsungads.com",
		"adnxs.com", "criteo.com", "taboola.com", "outbrain.com", "popads.net", "adcolony.com",
	},
}

type Engine struct {
	mu sync.RWMutex

	exactBlocked map[string]struct{}
	suffixBlocks map[string]struct{}
	exactAllowed map[string]struct{}
	suffixAllows map[string]struct{}

	totalRules  int
	lastUpdated time.Time
}

func NewEngine() *Engine {
	return &Engine{
		exactBlocked: make(map[string]struct{}),
		suffixBlocks: make(map[string]struct{}),
		exactAllowed: make(map[string]struct{}),
		suffixAllows: make(map[string]struct{}),
		lastUpdated:  time.Now(),
	}
}

// Normaliza o domínio: remove ponto final e coloca em minúsculas
func CleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimSuffix(d, ".")
	return d
}

// Identifica se um domínio pertence a uma categoria pré-mapeada
func (e *Engine) CategorizeDomain(domain string) string {
	catID, _, _, _ := DetectCategory(domain)
	if catID == "general" {
		return ""
	}
	return catID
}

// Retorna as informações completas da categoria para um determinado domínio
func DetectCategory(domain string) (catID, catName, icon, color string) {
	domain = CleanDomain(domain)
	if domain == "" {
		return "general", "Geral / Navegação", "fas fa-globe", "#64748b"
	}

	for cat, domains := range categoryDatabase {
		for _, base := range domains {
			if domain == base || strings.HasSuffix(domain, "."+base) {
				info := GetCategoryInfo(cat)
				return info.ID, info.Name, info.Icon, info.Color
			}
		}
	}
	return "general", "Geral / Navegação", "fas fa-globe", "#64748b"
}

// Retorna metadados de uma categoria pelo ID
func GetCategoryInfo(catID string) CategoryInfo {
	for _, cat := range AvailableCategories {
		if cat.ID == catID {
			return cat
		}
	}
	if catID == "general" {
		return CategoryInfo{
			ID:          "general",
			Name:        "Geral / Navegação",
			Description: "Sites e serviços gerais de navegação na web",
			Icon:        "fas fa-globe",
			Color:       "#64748b",
		}
	}
	return CategoryInfo{
		ID:    catID,
		Name:  catID,
		Icon:  "fas fa-folder",
		Color: "#64748b",
	}
}

// Checa regras considerando as políticas de categorias do grupo do dispositivo
func (e *Engine) CheckWithGroupCategories(domain string, groupRules map[string]string) (blocked bool, matchedRule string) {
	domain = CleanDomain(domain)
	if domain == "" {
		return false, ""
	}

	e.mu.RLock()
	// 1. Checa Allowlist geral
	if _, ok := e.exactAllowed[domain]; ok {
		e.mu.RUnlock()
		return false, "allow:" + domain
	}
	parts := strings.Split(domain, ".")
	for i := 0; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if _, ok := e.suffixAllows[parent]; ok {
			e.mu.RUnlock()
			return false, "allow:*." + parent
		}
	}
	e.mu.RUnlock()

	// 2. Checa se o domínio cai em alguma categoria com regra definida no grupo
	cat := e.CategorizeDomain(domain)
	if cat != "" {
		if action, exists := groupRules[cat]; exists {
			if action == "block" {
				return true, "category:" + cat
			} else if action == "allow" {
				// Categoria explicitamente permitida pelo grupo
				return false, "allow-category:" + cat
			}
		}
	}

	// 3. Fallback para verificação de listas padrão
	return e.Check(domain)
}

func (e *Engine) Check(domain string) (blocked bool, matchedRule string) {
	domain = CleanDomain(domain)
	if domain == "" {
		return false, ""
	}


	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Checagem de Lista de Permissão (Allowlist / Exceções)
	if _, ok := e.exactAllowed[domain]; ok {
		return false, "allow:" + domain
	}

	// Checa sufixos na Allowlist (ex: @@||google.com^)
	parts := strings.Split(domain, ".")
	for i := 0; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if _, ok := e.suffixAllows[parent]; ok {
			return false, "allow:*." + parent
		}
	}

	// 2. Checagem de Lista de Bloqueio Exata
	if _, ok := e.exactBlocked[domain]; ok {
		return true, "block:" + domain
	}

	// 3. Checagem de Sufixo de Bloqueio (ex: ||doubleclick.net^)
	for i := 0; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if _, ok := e.suffixBlocks[parent]; ok {
			return true, "block:*." + parent
		}
	}

	return false, ""
}

// Carrega regras em lote substituindo as regras antigas
func (e *Engine) LoadRules(blockedDomains []string, customBlock []string, customAllow []string) {
	exactBlocked := make(map[string]struct{}, len(blockedDomains)+len(customBlock))
	suffixBlocks := make(map[string]struct{}, len(blockedDomains)/2)
	exactAllowed := make(map[string]struct{}, len(customAllow))
	suffixAllows := make(map[string]struct{}, len(customAllow))

	for _, rule := range customAllow {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") || strings.HasPrefix(rule, "!") {
			continue
		}
		if strings.HasPrefix(rule, "@@||") {
			rule = strings.TrimPrefix(rule, "@@||")
			rule = strings.TrimSuffix(rule, "^")
			clean := CleanDomain(rule)
			if clean != "" {
				suffixAllows[clean] = struct{}{}
			}
		} else {
			clean := CleanDomain(rule)
			if clean != "" {
				exactAllowed[clean] = struct{}{}
			}
		}
	}

	for _, rule := range customBlock {
		rule = strings.TrimSpace(rule)
		if rule == "" || strings.HasPrefix(rule, "#") || strings.HasPrefix(rule, "!") {
			continue
		}
		if strings.HasPrefix(rule, "||") {
			rule = strings.TrimPrefix(rule, "||")
			rule = strings.TrimSuffix(rule, "^")
			clean := CleanDomain(rule)
			if clean != "" {
				suffixBlocks[clean] = struct{}{}
			}
		} else {
			clean := CleanDomain(rule)
			if clean != "" {
				exactBlocked[clean] = struct{}{}
			}
		}
	}

	for _, item := range blockedDomains {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.HasPrefix(item, "||") {
			clean := CleanDomain(strings.TrimSuffix(strings.TrimPrefix(item, "||"), "^"))
			if clean != "" {
				suffixBlocks[clean] = struct{}{}
			}
		} else {
			clean := CleanDomain(item)
			if clean != "" {
				exactBlocked[clean] = struct{}{}
			}
		}
	}

	e.mu.Lock()
	e.exactBlocked = exactBlocked
	e.suffixBlocks = suffixBlocks
	e.exactAllowed = exactAllowed
	e.suffixAllows = suffixAllows
	e.totalRules = len(exactBlocked) + len(suffixBlocks)
	e.lastUpdated = time.Now()
	e.mu.Unlock()
}

func (e *Engine) Stats() (totalRules int, lastUpdated time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.totalRules, e.lastUpdated
}

// Analisador de regras a partir de um leitor (hosts file, Adblock syntax ou lista simples)
func ParseRulesReader(r io.Reader) []string {
	var rules []string
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		// Caso seja formato /etc/hosts: 0.0.0.0 dominio.com ou 127.0.0.1 dominio.com
		if strings.HasPrefix(line, "127.0.0.1") || strings.HasPrefix(line, "0.0.0.0") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				domain := parts[1]
				if domain != "localhost" && domain != "broadcasthost" && domain != "local" {
					rules = append(rules, domain)
				}
			}
			continue
		}

		// Caso formato Adblock: ||dominio.com^
		if strings.HasPrefix(line, "||") && strings.HasSuffix(line, "^") {
			rules = append(rules, line)
			continue
		}

		// Caso formato lista de domínios pura
		if !strings.Contains(line, " ") && !strings.Contains(line, "/") {
			rules = append(rules, line)
		}
	}
	return rules
}

// Download com timeout, validação de esquema anti-SSRF e limite de tamanho
func FetchList(urlStr string) ([]string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("esquema não permitido (apenas http e https): %s", parsedURL.Scheme)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CoorpDNS-Security/1.0 (+https://github.com/coorpdns/coorpdns)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("servidor retornou HTTP %d ao baixar lista", resp.StatusCode)
	}

	// Limita leitura a 50MB para mitigar exaustão de memória DoS por listas maliciosas
	limitedReader := io.LimitReader(resp.Body, 50*1024*1024)
	return ParseRulesReader(limitedReader), nil
}
