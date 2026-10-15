package storage

import (
	"fmt"
	"sort"
	"time"

	"github.com/coorpdns/coorpdns/internal/filter"
)

type TopDomainItem struct {
	Domain         string  `json:"domain"`
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	BlockRate      float64 `json:"block_rate"`
}

type TopClientItem struct {
	ClientIP       string  `json:"client_ip"`
	ClientName     string  `json:"client_name"`
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	BlockRate      float64 `json:"block_rate"`
}

type ClientDomainActivityItem struct {
	Domain         string `json:"domain"`
	Category       string `json:"category,omitempty"`
	CategoryName   string `json:"category_name,omitempty"`
	TotalQueries   int64  `json:"total_queries"`
	BlockedQueries int64  `json:"blocked_queries"`
}

type TimelinePoint struct {
	Date           string `json:"date"`
	TotalQueries   int64  `json:"total_queries"`
	BlockedQueries int64  `json:"blocked_queries"`
}

type GlobalStats struct {
	TotalQueries   int64   `json:"total_queries"`
	BlockedQueries int64   `json:"blocked_queries"`
	BlockRate      float64 `json:"block_rate"`
	UniqueDomains  int64   `json:"unique_domains"`
	ActiveClients  int64   `json:"active_clients"`
}

func (d *DB) GetGlobalStats(days int) (*GlobalStats, error) {
	return d.GetGlobalStatsFiltered(days, nil)
}

func (d *DB) GetGlobalStatsFiltered(days int, allowedGroupIDs []int64) (*GlobalStats, error) {
	if days <= 0 {
		days = 1
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	whereClause := "WHERE date >= ?"
	var args []interface{}
	args = append(args, cutoffDate)

	if allowedGroupIDs != nil {
		cond, condArgs := buildClientInGroupSubquery("client_ip", allowedGroupIDs)
		whereClause += " AND " + cond
		args = append(args, condArgs...)
	}

	query := fmt.Sprintf(`
		SELECT 
			COALESCE(SUM(total_queries), 0) AS total,
			COALESCE(SUM(blocked_queries), 0) AS blocked,
			COUNT(DISTINCT domain) AS unique_domains,
			COUNT(DISTINCT client_ip) AS active_clients
		FROM daily_client_domain_stats
		%s
	`, whereClause)

	row := d.db.QueryRow(query, args...)

	stats := &GlobalStats{}
	err := row.Scan(&stats.TotalQueries, &stats.BlockedQueries, &stats.UniqueDomains, &stats.ActiveClients)
	if err != nil {
		return nil, err
	}

	if stats.TotalQueries > 0 {
		stats.BlockRate = float64(stats.BlockedQueries) / float64(stats.TotalQueries) * 100.0
	}
	return stats, nil
}

// Relatório 1: Top Sites Mais Acessados (até 180 dias)
func (d *DB) GetTopDomains(days int, limit int, statusFilter string) ([]TopDomainItem, error) {
	return d.GetTopDomainsFiltered(days, limit, statusFilter, nil)
}

func (d *DB) GetTopDomainsFiltered(days int, limit int, statusFilter string, allowedGroupIDs []int64) ([]TopDomainItem, error) {
	if days <= 0 || days > 180 {
		days = 180
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	whereClause := "WHERE date >= ?"
	var args []interface{}
	args = append(args, cutoffDate)

	if allowedGroupIDs != nil {
		cond, condArgs := buildClientInGroupSubquery("client_ip", allowedGroupIDs)
		whereClause += " AND " + cond
		args = append(args, condArgs...)
	}

	if statusFilter == "blocked" {
		whereClause += " AND blocked_queries > 0"
	} else if statusFilter == "allowed" {
		whereClause += " AND (total_queries - blocked_queries) > 0"
	}

	query := fmt.Sprintf(`
		SELECT 
			domain,
			SUM(total_queries) as total,
			SUM(blocked_queries) as blocked
		FROM daily_client_domain_stats
		%s
		GROUP BY domain
		ORDER BY total DESC
		LIMIT ?
	`, whereClause)
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TopDomainItem
	for rows.Next() {
		var item TopDomainItem
		if err := rows.Scan(&item.Domain, &item.TotalQueries, &item.BlockedQueries); err != nil {
			continue
		}
		if item.TotalQueries > 0 {
			item.BlockRate = float64(item.BlockedQueries) / float64(item.TotalQueries) * 100.0
		}
		results = append(results, item)
	}
	return results, nil
}

// Relatório 2: Top Máquinas Que Mais Consomem (até 180 dias)
func (d *DB) GetTopClients(days int, limit int) ([]TopClientItem, error) {
	return d.GetTopClientsFiltered(days, limit, nil)
}

func (d *DB) GetTopClientsFiltered(days int, limit int, allowedGroupIDs []int64) ([]TopClientItem, error) {
	if days <= 0 || days > 180 {
		days = 180
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	whereClause := "WHERE s.date >= ?"
	var args []interface{}
	args = append(args, cutoffDate)

	if allowedGroupIDs != nil {
		cond, condArgs := buildClientInGroupSubquery("s.client_ip", allowedGroupIDs)
		whereClause += " AND " + cond
		args = append(args, condArgs...)
	}

	query := fmt.Sprintf(`
		SELECT 
			s.client_ip,
			COALESCE(c.name, '') as client_name,
			SUM(s.total_queries) as total,
			SUM(s.blocked_queries) as blocked
		FROM daily_client_domain_stats s
		LEFT JOIN clients c ON s.client_ip = c.ip OR s.client_ip = c.auth_token OR s.client_ip = ('remote-' || substr(c.auth_token, 1, 10))
		%s
		GROUP BY s.client_ip
		ORDER BY total DESC
		LIMIT ?
	`, whereClause)
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TopClientItem
	for rows.Next() {
		var item TopClientItem
		if err := rows.Scan(&item.ClientIP, &item.ClientName, &item.TotalQueries, &item.BlockedQueries); err != nil {
			continue
		}
		if item.ClientName == "" {
			item.ClientName = item.ClientIP
		}
		if item.TotalQueries > 0 {
			item.BlockRate = float64(item.BlockedQueries) / float64(item.TotalQueries) * 100.0
		}
		results = append(results, item)
	}
	return results, nil
}

// Relatório 3: Máquina x Sites Acessados por Período (até 180 dias)
func (d *DB) GetClientDomainActivity(clientIP string, days int, fromDate, toDate string, limit int) ([]ClientDomainActivityItem, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var args []interface{}
	query := `
		SELECT 
			domain,
			SUM(total_queries) as total,
			SUM(blocked_queries) as blocked
		FROM daily_client_domain_stats
		WHERE 1=1
	`

	if clientIP != "" && clientIP != "all" {
		query += " AND client_ip = ?"
		args = append(args, clientIP)
	}

	if fromDate != "" && toDate != "" {
		query += " AND date BETWEEN ? AND ?"
		args = append(args, fromDate, toDate)
	} else {
		if days <= 0 || days > 180 {
			days = 180
		}
		cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
		query += " AND date >= ?"
		args = append(args, cutoffDate)
	}

	query += `
		GROUP BY domain
		ORDER BY total DESC
		LIMIT ?
	`
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ClientDomainActivityItem
	for rows.Next() {
		var item ClientDomainActivityItem
		if err := rows.Scan(&item.Domain, &item.TotalQueries, &item.BlockedQueries); err != nil {
			continue
		}
		results = append(results, item)
	}
	return results, nil
}

// Série temporal diária para montar gráficos ao longo de 180 dias
func (d *DB) GetTimeline(days int) ([]TimelinePoint, error) {
	return d.GetTimelineFiltered(days, nil)
}

func (d *DB) GetTimelineFiltered(days int, allowedGroupIDs []int64) ([]TimelinePoint, error) {
	if days <= 0 || days > 180 {
		days = 30
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	whereClause := "WHERE date >= ?"
	var args []interface{}
	args = append(args, cutoffDate)

	if allowedGroupIDs != nil {
		cond, condArgs := buildClientInGroupSubquery("client_ip", allowedGroupIDs)
		whereClause += " AND " + cond
		args = append(args, condArgs...)
	}

	query := fmt.Sprintf(`
		SELECT 
			date,
			SUM(total_queries) as total,
			SUM(blocked_queries) as blocked
		FROM daily_client_domain_stats
		%s
		GROUP BY date
		ORDER BY date ASC
	`, whereClause)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TimelinePoint
	for rows.Next() {
		var p TimelinePoint
		if err := rows.Scan(&p.Date, &p.TotalQueries, &p.BlockedQueries); err != nil {
			continue
		}
		points = append(points, p)
	}
	return points, nil
}

// Série temporal horária das últimas 24 horas para o gráfico do dashboard
func (d *DB) GetActivity24h() ([]TimelinePoint, error) {
	return d.GetActivity24hFiltered(nil)
}

func (d *DB) GetActivity24hFiltered(allowedGroupIDs []int64) ([]TimelinePoint, error) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	cutoffISO := now.Add(-24 * time.Hour).Format(time.RFC3339)

	buckets := make(map[string]*TimelinePoint)
	var orderedKeys []string

	for i := 23; i >= 0; i-- {
		t := now.Add(-time.Duration(i) * time.Hour)
		key := t.Format("2006-01-02 15:00")
		label := t.Format("15:00")
		p := &TimelinePoint{
			Date:           label,
			TotalQueries:   0,
			BlockedQueries: 0,
		}
		buckets[key] = p
		orderedKeys = append(orderedKeys, key)
	}

	whereClause := "WHERE (timestamp >= ? OR timestamp >= ?)"
	var args []interface{}
	args = append(args, cutoff, cutoffISO)

	if allowedGroupIDs != nil {
		cond, condArgs := buildClientInGroupSubquery("client_ip", allowedGroupIDs)
		whereClause += " AND " + cond
		args = append(args, condArgs...)
	}

	query := fmt.Sprintf(`
		SELECT 
			replace(substr(timestamp, 1, 13), 'T', ' ') || ':00' as hour_bucket,
			COUNT(*) as total,
			SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) as blocked
		FROM dns_queries
		%s
		GROUP BY hour_bucket
	`, whereClause)

	rows, err := d.db.Query(query, args...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var hr string
			var total, blocked int64
			if err := rows.Scan(&hr, &total, &blocked); err == nil {
				if p, ok := buckets[hr]; ok {
					p.TotalQueries = total
					p.BlockedQueries = blocked
				}
			}
		}
	}

	var results []TimelinePoint
	for _, k := range orderedKeys {
		results = append(results, *buckets[k])
	}
	return results, nil
}

// Relatório 4: Relatório por Grupo de Dispositivos (Dispositivos do grupo, categorias e sites acessados por cada um)
type GroupDeviceCategorySummary struct {
	CategoryID     string                     `json:"category_id"`
	CategoryName   string                     `json:"category_name"`
	Icon           string                     `json:"icon"`
	Color          string                     `json:"color"`
	TotalQueries   int64                      `json:"total_queries"`
	BlockedQueries int64                      `json:"blocked_queries"`
	DomainsCount   int                        `json:"domains_count"`
	Domains        []ClientDomainActivityItem `json:"domains"`
}

type GroupDeviceActivity struct {
	ClientIP       string                        `json:"client_ip"`
	ClientName     string                        `json:"client_name"`
	IsRemote       bool                          `json:"is_remote"`
	OSType         string                        `json:"os_type"`
	TotalQueries   int64                         `json:"total_queries"`
	BlockedQueries int64                         `json:"blocked_queries"`
	BlockRate      float64                       `json:"block_rate"`
	DomainsCount   int                           `json:"domains_count"`
	Domains        []ClientDomainActivityItem    `json:"domains"`
	Categories     []*GroupDeviceCategorySummary `json:"categories"`
}

type GroupReportResult struct {
	GroupID        int64                 `json:"group_id"`
	GroupName      string                `json:"group_name"`
	GroupColor     string                `json:"group_color"`
	Description    string                `json:"description"`
	Days           int                   `json:"days"`
	TotalQueries   int64                 `json:"total_queries"`
	BlockedQueries int64                 `json:"blocked_queries"`
	BlockRate      float64               `json:"block_rate"`
	DevicesCount   int                   `json:"devices_count"`
	Devices        []GroupDeviceActivity `json:"devices"`
}

func (d *DB) GetGroupReport(groupID int64, days int, maxDomainsPerDevice int) (*GroupReportResult, error) {
	if days <= 0 || days > 180 {
		days = 30
	}
	if maxDomainsPerDevice <= 0 || maxDomainsPerDevice > 500 {
		maxDomainsPerDevice = 50
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	// 1. Busca dados do grupo
	groupRow := d.db.QueryRow("SELECT id, name, COALESCE(description, ''), color FROM client_groups WHERE id = ?", groupID)
	result := &GroupReportResult{Days: days, Devices: []GroupDeviceActivity{}}
	if err := groupRow.Scan(&result.GroupID, &result.GroupName, &result.Description, &result.GroupColor); err != nil {
		return nil, fmt.Errorf("grupo não encontrado: %w", err)
	}

	// 2. Busca todos os dispositivos que pertencem a este grupo
	clientRows, err := d.db.Query("SELECT ip, name, is_remote, COALESCE(os_type, 'windows') FROM clients WHERE group_id = ? AND status = 'active' ORDER BY name ASC", groupID)
	if err != nil {
		return nil, err
	}

	type clientEntry struct {
		ip       string
		name     string
		isRemote bool
		osType   string
	}
	var clientsList []clientEntry

	for clientRows.Next() {
		var ip, name, osType string
		var isRemoteVal int
		if err := clientRows.Scan(&ip, &name, &isRemoteVal, &osType); err == nil {
			if osType == "" {
				osType = "windows"
			}
			clientsList = append(clientsList, clientEntry{
				ip:       ip,
				name:     name,
				isRemote: isRemoteVal == 1,
				osType:   osType,
			})
		}
	}
	clientRows.Close()

	for _, c := range clientsList {
		dev := GroupDeviceActivity{
			ClientIP:   c.ip,
			ClientName: c.name,
			IsRemote:   c.isRemote,
			OSType:     c.osType,
			Domains:    []ClientDomainActivityItem{},
			Categories: []*GroupDeviceCategorySummary{},
		}

		catMap := make(map[string]*GroupDeviceCategorySummary)

		// 3. Busca os sites que este dispositivo específico acessou no período
		domRows, err := d.db.Query(`
			SELECT domain, SUM(total_queries) as total, SUM(blocked_queries) as blocked
			FROM daily_client_domain_stats
			WHERE client_ip = ? AND date >= ?
			GROUP BY domain
			ORDER BY total DESC
			LIMIT ?
		`, dev.ClientIP, cutoffDate, maxDomainsPerDevice)
		if err == nil {
			for domRows.Next() {
				var it ClientDomainActivityItem
				if err := domRows.Scan(&it.Domain, &it.TotalQueries, &it.BlockedQueries); err == nil {
					catID, catName, icon, color := filter.DetectCategory(it.Domain)
					it.Category = catID
					it.CategoryName = catName

					dev.TotalQueries += it.TotalQueries
					dev.BlockedQueries += it.BlockedQueries
					dev.Domains = append(dev.Domains, it)

					cSummary, exists := catMap[catID]
					if !exists {
						cSummary = &GroupDeviceCategorySummary{
							CategoryID:   catID,
							CategoryName: catName,
							Icon:         icon,
							Color:        color,
							Domains:      []ClientDomainActivityItem{},
						}
						catMap[catID] = cSummary
					}
					cSummary.TotalQueries += it.TotalQueries
					cSummary.BlockedQueries += it.BlockedQueries
					cSummary.Domains = append(cSummary.Domains, it)
					cSummary.DomainsCount = len(cSummary.Domains)
				}
			}
			domRows.Close()
		}

		dev.DomainsCount = len(dev.Domains)
		if dev.TotalQueries > 0 {
			dev.BlockRate = float64(dev.BlockedQueries) / float64(dev.TotalQueries) * 100.0
		}

		// Ordenar categorias: categorias com bloqueios ou mais acessadas primeiro
		var catList []*GroupDeviceCategorySummary
		for _, catSum := range catMap {
			catList = append(catList, catSum)
		}
		sort.Slice(catList, func(i, j int) bool {
			if catList[i].BlockedQueries != catList[j].BlockedQueries {
				return catList[i].BlockedQueries > catList[j].BlockedQueries
			}
			if catList[i].CategoryID == "general" && catList[j].CategoryID != "general" {
				return false
			}
			if catList[j].CategoryID == "general" && catList[i].CategoryID != "general" {
				return true
			}
			return catList[i].TotalQueries > catList[j].TotalQueries
		})
		dev.Categories = catList

		result.TotalQueries += dev.TotalQueries
		result.BlockedQueries += dev.BlockedQueries
		result.Devices = append(result.Devices, dev)
	}

	result.DevicesCount = len(result.Devices)
	if result.TotalQueries > 0 {
		result.BlockRate = float64(result.BlockedQueries) / float64(result.TotalQueries) * 100.0
	}

	return result, nil
}
