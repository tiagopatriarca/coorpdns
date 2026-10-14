package storage

import (
	"fmt"
	"time"
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
	if days <= 0 {
		days = 1
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	query := `
		SELECT 
			COALESCE(SUM(total_queries), 0) AS total,
			COALESCE(SUM(blocked_queries), 0) AS blocked,
			COUNT(DISTINCT domain) AS unique_domains,
			COUNT(DISTINCT client_ip) AS active_clients
		FROM daily_client_domain_stats
		WHERE date >= ?
	`
	row := d.db.QueryRow(query, cutoffDate)

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
	if days <= 0 || days > 180 {
		days = 180
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	whereClause := "WHERE date >= ?"
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

	rows, err := d.db.Query(query, cutoffDate, limit)
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
	if days <= 0 || days > 180 {
		days = 180
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	query := `
		SELECT 
			s.client_ip,
			COALESCE(c.name, '') as client_name,
			SUM(s.total_queries) as total,
			SUM(s.blocked_queries) as blocked
		FROM daily_client_domain_stats s
		LEFT JOIN clients c ON s.client_ip = c.ip
		WHERE s.date >= ?
		GROUP BY s.client_ip
		ORDER BY total DESC
		LIMIT ?
	`

	rows, err := d.db.Query(query, cutoffDate, limit)
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
	if days <= 0 || days > 180 {
		days = 30
	}
	cutoffDate := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	query := `
		SELECT 
			date,
			SUM(total_queries) as total,
			SUM(blocked_queries) as blocked
		FROM daily_client_domain_stats
		WHERE date >= ?
		GROUP BY date
		ORDER BY date ASC
	`

	rows, err := d.db.Query(query, cutoffDate)
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

// Relatório 4: Relatório por Grupo de Dispositivos (Dispositivos do grupo e os sites que cada um acessou no período)
type GroupDeviceActivity struct {
	ClientIP       string                     `json:"client_ip"`
	ClientName     string                     `json:"client_name"`
	IsRemote       bool                       `json:"is_remote"`
	TotalQueries   int64                      `json:"total_queries"`
	BlockedQueries int64                      `json:"blocked_queries"`
	BlockRate      float64                    `json:"block_rate"`
	DomainsCount   int                        `json:"domains_count"`
	Domains        []ClientDomainActivityItem `json:"domains"`
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
	clientRows, err := d.db.Query("SELECT ip, name, is_remote FROM clients WHERE group_id = ? AND status = 'active' ORDER BY name ASC", groupID)
	if err != nil {
		return nil, err
	}

	type clientEntry struct {
		ip       string
		name     string
		isRemote bool
	}
	var clientsList []clientEntry

	for clientRows.Next() {
		var ip, name string
		var isRemoteVal int
		if err := clientRows.Scan(&ip, &name, &isRemoteVal); err == nil {
			clientsList = append(clientsList, clientEntry{
				ip:       ip,
				name:     name,
				isRemote: isRemoteVal == 1,
			})
		}
	}
	clientRows.Close()

	for _, c := range clientsList {
		dev := GroupDeviceActivity{
			ClientIP:   c.ip,
			ClientName: c.name,
			IsRemote:   c.isRemote,
			Domains:    []ClientDomainActivityItem{},
		}

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
					dev.TotalQueries += it.TotalQueries
					dev.BlockedQueries += it.BlockedQueries
					dev.Domains = append(dev.Domains, it)
				}
			}
			domRows.Close()
		}

		dev.DomainsCount = len(dev.Domains)
		if dev.TotalQueries > 0 {
			dev.BlockRate = float64(dev.BlockedQueries) / float64(dev.TotalQueries) * 100.0
		}

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
