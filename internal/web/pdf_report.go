package web

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/coorpdns/coorpdns/internal/storage"
	"github.com/go-pdf/fpdf"
)

// GenerateGroupReportPDF gera um relatório corporativo em PDF estruturado por Grupo -> Máquina -> Sites
// incluindo cabeçalho com nome do grupo, tabela por dispositivo e gráfico comparativo final.
func GenerateGroupReportPDF(report *storage.GroupReportResult, days int) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 18)
	pdf.AliasNbPages("{nb}")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	latin := func(s string) string {
		return tr(s)
	}

	pdf.SetHeaderFunc(func() {
		// Barra superior corporativa
		pdf.SetFillColor(15, 23, 42) // #0f172a slate-900
		pdf.Rect(0, 0, 210, 14, "F")

		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetXY(10, 3)
		pdf.CellFormat(100, 8, latin("CoorpDNS Enterprise — Gestão e Auditoria de Segurança DNS"), "", 0, "L", false, 0, "")

		pdf.SetFont("Helvetica", "", 8)
		pdf.SetXY(110, 3)
		pdf.CellFormat(90, 8, latin(time.Now().Format("02/01/2006 15:04")), "", 0, "R", false, 0, "")
		pdf.SetY(18)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(100, 8, latin("Relatório Gerado pelo Painel Administrativo CoorpDNS"), "", 0, "L", false, 0, "")
		pdf.CellFormat(90, 8, latin(fmt.Sprintf("Página %d de {nb}", pdf.PageNo())), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()

	// 1. TOPO: Nome do Grupo em destaque
	pdf.SetY(20)
	pdf.SetTextColor(30, 41, 59)
	pdf.SetFont("Helvetica", "B", 18)
	groupTitle := fmt.Sprintf("Grupo: %s", report.GroupName)
	pdf.CellFormat(190, 9, latin(groupTitle), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(71, 85, 105)
	periodStr := fmt.Sprintf("Período Analisado: Últimos %d dias (%s a %s)", days, report.FromDate, report.ToDate)
	pdf.CellFormat(190, 6, latin(periodStr), "", 1, "L", false, 0, "")

	if report.Description != "" {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(190, 5, latin(fmt.Sprintf("Descrição: %s", report.Description)), "", 1, "L", false, 0, "")
	}

	pdf.Ln(3)

	// Cards de Resumo Executivo do Grupo
	cardW := 45.0
	cardH := 16.0
	startX := 10.0
	currentY := pdf.GetY()

	drawMetricCard := func(x float64, title string, value string, r, g, b int) {
		pdf.SetFillColor(248, 250, 252)
		pdf.SetDrawColor(226, 232, 240)
		pdf.RoundedRect(x, currentY, cardW, cardH, 2, "1234", "FD")

		pdf.SetTextColor(100, 116, 139)
		pdf.SetFont("Helvetica", "B", 7)
		pdf.SetXY(x+2, currentY+2)
		pdf.CellFormat(cardW-4, 4, latin(strings.ToUpper(title)), "", 0, "L", false, 0, "")

		pdf.SetTextColor(r, g, b)
		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetXY(x+2, currentY+7)
		pdf.CellFormat(cardW-4, 7, latin(value), "", 0, "L", false, 0, "")
	}

	allowedTotal := report.TotalQueries - report.BlockedQueries
	drawMetricCard(startX, "Total de Consultas", fmt.Sprintf("%d", report.TotalQueries), 30, 41, 59)
	drawMetricCard(startX+cardW+3, "Acessos Liberados", fmt.Sprintf("%d", allowedTotal), 16, 185, 129)
	drawMetricCard(startX+(cardW+3)*2, "Acessos Bloqueados", fmt.Sprintf("%d", report.BlockedQueries), 239, 68, 68)
	drawMetricCard(startX+(cardW+3)*3, "Dispositivos Ativos", fmt.Sprintf("%d", report.DevicesCount), 59, 130, 246)

	pdf.SetY(currentY + cardH + 5)

	// 2. RESUMO GRÁFICO: Gráfico de Categorias e Gráfico de Aplicativos (Gráficos de Barras com Ícones e Nomes)
	chartSectionY := pdf.GetY()
	colW := 92.0
	colH := 62.0

	// Card 1: Gráfico de Barras por Categoria (Coluna Esquerda)
	pdf.SetFillColor(248, 250, 252)
	pdf.SetDrawColor(226, 232, 240)
	pdf.RoundedRect(10, chartSectionY, colW, colH, 2, "1234", "FD")

	pdf.SetXY(13, chartSectionY+3)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(colW-6, 5, latin("TOP CATEGORIAS MAIS ACESSADAS"), "", 1, "L", false, 0, "")

	if len(report.CategoriesStats) == 0 {
		pdf.SetXY(13, chartSectionY+25)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(148, 163, 184)
		pdf.CellFormat(colW-6, 6, latin("Nenhuma consulta por categoria registrada."), "", 0, "C", false, 0, "")
	} else {
		var maxCatQueries int64 = 1
		for _, c := range report.CategoriesStats {
			if c.TotalQueries > maxCatQueries {
				maxCatQueries = c.TotalQueries
			}
		}

		itemStartY := chartSectionY + 9.0
		for i, cat := range report.CategoriesStats {
			if i >= 5 {
				break
			}
			rowY := itemStartY + float64(i)*10.0
			cr, cg, cb := parseHexColor(cat.Color)

			// Badge com ícone/símbolo da categoria
			pdf.SetFillColor(cr, cg, cb)
			pdf.RoundedRect(13, rowY, 5, 5, 1.2, "1234", "F")
			pdf.SetTextColor(255, 255, 255)
			sym := getCategorySymbol(cat.ID)
			if len(sym) >= 2 {
				pdf.SetFont("Helvetica", "B", 4.5)
			} else {
				pdf.SetFont("Helvetica", "B", 6)
			}
			pdf.SetXY(13, rowY+0.6)
			pdf.CellFormat(5, 4.2, sym, "", 0, "C", false, 0, "")

			// Nome da Categoria
			catNameDisplay := cat.Name
			if len(catNameDisplay) > 23 {
				catNameDisplay = catNameDisplay[:21] + ".."
			}
			pdf.SetXY(19, rowY)
			pdf.SetFont("Helvetica", "B", 7.5)
			pdf.SetTextColor(30, 41, 59)
			pdf.CellFormat(46, 5, latin(catNameDisplay), "", 0, "L", false, 0, "")

			// Total e Porcentagem
			pdf.SetXY(65, rowY)
			pdf.SetFont("Helvetica", "", 7)
			pdf.SetTextColor(100, 116, 139)
			catCountStr := fmt.Sprintf("%d (%.1f%%)", cat.TotalQueries, cat.Percentage)
			pdf.CellFormat(34, 5, latin(catCountStr), "", 0, "R", false, 0, "")

			// Barra de Progresso
			barX := 19.0
			barY := rowY + 5.2
			barMaxW := 80.0
			barH := 2.8

			// Trilha vazia
			pdf.SetFillColor(226, 232, 240)
			pdf.RoundedRect(barX, barY, barMaxW, barH, 0.8, "1234", "F")

			// Barra proporcional preenchida
			wBar := (float64(cat.TotalQueries) / float64(maxCatQueries)) * barMaxW
			if wBar > 0 {
				if cat.Blocked > 0 {
					wBlocked := (float64(cat.Blocked) / float64(cat.TotalQueries)) * wBar
					wAllowed := wBar - wBlocked
					if wAllowed > 0 {
						pdf.SetFillColor(cr, cg, cb)
						pdf.Rect(barX, barY, wAllowed, barH, "F")
					}
					if wBlocked > 0 {
						pdf.SetFillColor(239, 68, 68) // Vermelho bloqueado
						pdf.Rect(barX+wAllowed, barY, wBlocked, barH, "F")
					}
				} else {
					pdf.SetFillColor(cr, cg, cb)
					pdf.RoundedRect(barX, barY, wBar, barH, 0.8, "1234", "F")
				}
			}
		}
	}

	// Card 2: Gráfico de Barras por Aplicativo (Coluna Direita)
	pdf.SetFillColor(248, 250, 252)
	pdf.SetDrawColor(226, 232, 240)
	pdf.RoundedRect(108, chartSectionY, colW, colH, 2, "1234", "FD")

	pdf.SetXY(111, chartSectionY+3)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(colW-6, 5, latin("TOP APLICATIVOS MAIS UTILIZADOS"), "", 1, "L", false, 0, "")

	if len(report.ApplicationsStats) == 0 {
		pdf.SetXY(111, chartSectionY+25)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(148, 163, 184)
		pdf.CellFormat(colW-6, 6, latin("Nenhum aplicativo mapeado no período."), "", 0, "C", false, 0, "")
	} else {
		var maxAppQueries int64 = 1
		for _, a := range report.ApplicationsStats {
			if a.TotalQueries > maxAppQueries {
				maxAppQueries = a.TotalQueries
			}
		}

		itemStartY := chartSectionY + 9.0
		for i, app := range report.ApplicationsStats {
			if i >= 5 {
				break
			}
			rowY := itemStartY + float64(i)*10.0
			ar, ag, ab := parseHexColor(app.Color)

			// Badge com emblema oficial da marca do aplicativo
			pdf.SetFillColor(ar, ag, ab)
			pdf.RoundedRect(111, rowY, 5, 5, 1.2, "1234", "F")
			pdf.SetTextColor(255, 255, 255)
			emblem := getAppEmblem(app.ID, app.Name)
			if len(emblem) >= 3 {
				pdf.SetFont("Helvetica", "B", 4.5)
			} else if len(emblem) == 2 {
				pdf.SetFont("Helvetica", "B", 5.5)
			} else {
				pdf.SetFont("Helvetica", "B", 6)
			}
			pdf.SetXY(111, rowY+0.6)
			pdf.CellFormat(5, 4.2, emblem, "", 0, "C", false, 0, "")

			// Nome do Aplicativo
			appNameDisplay := app.Name
			if len(appNameDisplay) > 23 {
				appNameDisplay = appNameDisplay[:21] + ".."
			}
			pdf.SetXY(117, rowY)
			pdf.SetFont("Helvetica", "B", 7.5)
			pdf.SetTextColor(30, 41, 59)
			pdf.CellFormat(46, 5, latin(appNameDisplay), "", 0, "L", false, 0, "")

			// Total e Porcentagem
			pdf.SetXY(163, rowY)
			pdf.SetFont("Helvetica", "", 7)
			pdf.SetTextColor(100, 116, 139)
			appCountStr := fmt.Sprintf("%d (%.1f%%)", app.TotalQueries, app.Percentage)
			pdf.CellFormat(34, 5, latin(appCountStr), "", 0, "R", false, 0, "")

			// Barra de Progresso
			barX := 117.0
			barY := rowY + 5.2
			barMaxW := 80.0
			barH := 2.8

			// Trilha vazia
			pdf.SetFillColor(226, 232, 240)
			pdf.RoundedRect(barX, barY, barMaxW, barH, 0.8, "1234", "F")

			// Barra proporcional preenchida
			wBar := (float64(app.TotalQueries) / float64(maxAppQueries)) * barMaxW
			if wBar > 0 {
				if app.Blocked > 0 {
					wBlocked := (float64(app.Blocked) / float64(app.TotalQueries)) * wBar
					wAllowed := wBar - wBlocked
					if wAllowed > 0 {
						pdf.SetFillColor(ar, ag, ab)
						pdf.Rect(barX, barY, wAllowed, barH, "F")
					}
					if wBlocked > 0 {
						pdf.SetFillColor(239, 68, 68) // Vermelho bloqueado
						pdf.Rect(barX+wAllowed, barY, wBlocked, barH, "F")
					}
				} else {
					pdf.SetFillColor(ar, ag, ab)
					pdf.RoundedRect(barX, barY, wBar, barH, 0.8, "1234", "F")
				}
			}
		}
	}

	pdf.SetY(chartSectionY + colH + 6)

	// 3. CORPO: Por Máquina (Nome da máquina -> Categoria -> Site -> Acessos -> Bloqueado ou Não)
	if len(report.Devices) == 0 {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(190, 10, latin("Nenhum dispositivo associado a este grupo possui tráfego registrado no período."), "1", 1, "C", false, 0, "")
	}

	for _, dev := range report.Devices {
		if pdf.GetY() > 240 {
			pdf.AddPage()
		}

		// Banner da Máquina
		pdf.SetFillColor(241, 245, 249) // #f1f5f9
		pdf.SetDrawColor(203, 213, 225)
		pdf.SetTextColor(15, 23, 42)
		pdf.SetFont("Helvetica", "B", 11)

		devTypeStr := "Wi-Fi Local"
		if dev.IsRemote {
			devTypeStr = "Nuvem (DoH)"
		}

		machineTitle := fmt.Sprintf("  Máquina: %s  [%s]  —  IP/ID: %s", dev.ClientName, devTypeStr, dev.ClientIP)
		pdf.CellFormat(190, 8, latin(machineTitle), "1", 1, "L", true, 0, "")

		// Sub-estatísticas da máquina
		pdf.SetFillColor(255, 255, 255)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(71, 85, 105)
		devAllowed := dev.TotalQueries - dev.BlockedQueries
		devSubStats := fmt.Sprintf("Total Acessos: %d  |  Liberados: %d  |  Bloqueados: %d (%.1f%%)",
			dev.TotalQueries, devAllowed, dev.BlockedQueries, dev.BlockRate)
		pdf.CellFormat(190, 5, latin("  "+devSubStats), "LR", 1, "L", false, 0, "")

		// Cabeçalho da Tabela
		pdf.SetFillColor(226, 232, 240)
		pdf.SetTextColor(51, 65, 85)
		pdf.SetFont("Helvetica", "B", 8)

		pdf.CellFormat(45, 6, latin("Categoria"), "1", 0, "L", true, 0, "")
		pdf.CellFormat(80, 6, latin("Site / Domínio"), "1", 0, "L", true, 0, "")
		pdf.CellFormat(30, 6, latin("Acessos"), "1", 0, "R", true, 0, "")
		pdf.CellFormat(35, 6, latin("Status"), "1", 1, "C", true, 0, "")

		if len(dev.Domains) == 0 {
			pdf.SetFont("Helvetica", "I", 8)
			pdf.SetTextColor(148, 163, 184)
			pdf.CellFormat(190, 6, latin("Sem consultas DNS para esta máquina."), "1", 1, "C", false, 0, "")
		} else {
			pdf.SetFont("Helvetica", "", 8)

			for _, domItem := range dev.Domains {
				if pdf.GetY() > 270 {
					pdf.AddPage()
					// Repete cabeçalho para leitura contínua
					pdf.SetFillColor(226, 232, 240)
					pdf.SetTextColor(51, 65, 85)
					pdf.SetFont("Helvetica", "B", 8)
					pdf.CellFormat(45, 6, latin("Categoria"), "1", 0, "L", true, 0, "")
					pdf.CellFormat(80, 6, latin("Site / Domínio"), "1", 0, "L", true, 0, "")
					pdf.CellFormat(30, 6, latin("Acessos"), "1", 0, "R", true, 0, "")
					pdf.CellFormat(35, 6, latin("Status"), "1", 1, "C", true, 0, "")
					pdf.SetFont("Helvetica", "", 8)
				}

				// Status de Bloqueado ou Não
				statusText := "Liberado"
				isBlocked := domItem.BlockedQueries > 0
				if domItem.BlockedQueries > 0 && domItem.BlockedQueries == domItem.TotalQueries {
					statusText = "Bloqueado"
				} else if domItem.BlockedQueries > 0 {
					statusText = fmt.Sprintf("Bloqueado (%d/%d)", domItem.BlockedQueries, domItem.TotalQueries)
				}

				// Cores alternadas sutis de linha
				pdf.SetFillColor(255, 255, 255)
				pdf.SetTextColor(30, 41, 59)

				catName := domItem.CategoryName
				if catName == "" {
					catName = "Geral"
				}
				if len(catName) > 26 {
					catName = catName[:24] + ".."
				}

				domainStr := domItem.Domain
				if len(domainStr) > 48 {
					domainStr = domainStr[:46] + ".."
				}

				pdf.CellFormat(45, 5.5, latin(catName), "1", 0, "L", false, 0, "")
				pdf.CellFormat(80, 5.5, latin(domainStr), "1", 0, "L", false, 0, "")
				pdf.CellFormat(30, 5.5, fmt.Sprintf("%d", domItem.TotalQueries), "1", 0, "R", false, 0, "")

				// Status com cor semafórica
				if isBlocked {
					pdf.SetTextColor(220, 38, 38) // Vermelho #dc2626
					pdf.SetFont("Helvetica", "B", 8)
				} else {
					pdf.SetTextColor(22, 163, 74) // Verde #16a34a
					pdf.SetFont("Helvetica", "B", 8)
				}
				pdf.CellFormat(35, 5.5, latin(statusText), "1", 1, "C", false, 0, "")
				pdf.SetFont("Helvetica", "", 8)
			}
		}

		pdf.Ln(4)
	}

	// 3. FINAL: Gráfico de Bloqueados e Liberados
	if pdf.GetY() > 210 {
		pdf.AddPage()
	} else {
		pdf.Ln(4)
	}

	chartBoxY := pdf.GetY()
	chartBoxW := 190.0
	chartBoxH := 62.0

	pdf.SetFillColor(248, 250, 252)
	pdf.SetDrawColor(203, 213, 225)
	pdf.RoundedRect(10, chartBoxY, chartBoxW, chartBoxH, 3, "1234", "FD")

	pdf.SetXY(15, chartBoxY+4)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(180, 6, latin("Resumo Gráfico: Consultas Bloqueadas vs. Liberadas"), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(100, 116, 139)
	pdf.SetX(15)
	pdf.CellFormat(180, 5, latin(fmt.Sprintf("Volume total auditado para o grupo '%s': %d consultas DNS", report.GroupName, report.TotalQueries)), "", 1, "L", false, 0, "")

	// Desenho de Barra Proporcional Empilhada Vetorial
	barY := chartBoxY + 18
	barX := 20.0
	barW := 170.0
	barH := 12.0

	var pctAllowed, pctBlocked float64
	if report.TotalQueries > 0 {
		pctAllowed = (float64(allowedTotal) / float64(report.TotalQueries)) * 100.0
		pctBlocked = (float64(report.BlockedQueries) / float64(report.TotalQueries)) * 100.0
	} else {
		pctAllowed = 100.0
		pctBlocked = 0.0
	}

	wAllowed := (pctAllowed / 100.0) * barW
	wBlocked := (pctBlocked / 100.0) * barW

	// Barra Liberados (Verde)
	if wAllowed > 0 {
		pdf.SetFillColor(16, 185, 129) // #10b981
		pdf.Rect(barX, barY, wAllowed, barH, "F")
	}

	// Barra Bloqueados (Vermelho)
	if wBlocked > 0 {
		pdf.SetFillColor(239, 68, 68) // #ef4444
		pdf.Rect(barX+wAllowed, barY, wBlocked, barH, "F")
	}

	// Borda da barra
	pdf.SetDrawColor(148, 163, 184)
	pdf.Rect(barX, barY, barW, barH, "D")

	// Legendas e Métricas Detalhadas do Gráfico
	legendY := barY + barH + 6

	// Indicador Liberados
	pdf.SetFillColor(16, 185, 129)
	pdf.Rect(25, legendY+1, 5, 5, "F")
	pdf.SetXY(33, legendY)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(70, 7, latin(fmt.Sprintf("Liberadas: %d (%.1f%%)", allowedTotal, pctAllowed)), "", 0, "L", false, 0, "")

	// Indicador Bloqueados
	pdf.SetFillColor(239, 68, 68)
	pdf.Rect(115, legendY+1, 5, 5, "F")
	pdf.SetXY(123, legendY)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(70, 7, latin(fmt.Sprintf("Bloqueadas: %d (%.1f%%)", report.BlockedQueries, pctBlocked)), "", 1, "L", false, 0, "")

	// Pequeno gráfico comparativo de colunas
	subColY := legendY + 8
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(100, 116, 139)
	pdf.SetXY(20, subColY)
	pdf.CellFormat(170, 5, latin("Status de conformidade da política: verde indica tráfego legítimo permitido; vermelho indica ameaças e categorias restritas."), "", 1, "C", false, 0, "")

	_ = math.Abs(0) // para manter math se necessário

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseHexColor(hexStr string) (int, int, int) {
	hexStr = strings.TrimPrefix(hexStr, "#")
	if len(hexStr) == 3 {
		hexStr = string([]byte{hexStr[0], hexStr[0], hexStr[1], hexStr[1], hexStr[2], hexStr[2]})
	}
	if len(hexStr) != 6 {
		return 100, 116, 139 // fallback slate
	}
	var r, g, b int
	fmt.Sscanf(hexStr, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

func getAppEmblem(appID, appName string) string {
	switch strings.ToLower(appID) {
	case "whatsapp":
		return "WA"
	case "youtube":
		return "YT"
	case "chatgpt":
		return "AI"
	case "instagram":
		return "IG"
	case "facebook":
		return "FB"
	case "netflix":
		return "NF"
	case "spotify":
		return "SP"
	case "discord":
		return "DC"
	case "telegram":
		return "TG"
	case "teams":
		return "MS"
	case "roblox":
		return "RB"
	case "steam":
		return "ST"
	case "bet365":
		return "365"
	case "betano":
		return "B"
	case "blaze":
		return "BZ"
	case "nubank":
		return "NU"
	case "itau":
		return "IT"
	case "bradesco":
		return "BD"
	case "banco_do_brasil":
		return "BB"
	case "claude_ai":
		return "CL"
	case "tiktok":
		return "TT"
	case "twitter", "x":
		return "X"
	case "google_drive", "google_docs", "google_agenda":
		return "G"
	case "office_365", "outlook", "azure":
		return "MS"
	default:
		clean := strings.ToUpper(strings.TrimSpace(appName))
		if len(clean) >= 2 {
			return clean[:2]
		}
		if len(clean) == 1 {
			return clean
		}
		return "AP"
	}
}

func getCategorySymbol(catID string) string {
	switch catID {
	case "streaming":
		return ">"
	case "social":
		return "#"
	case "gaming":
		return "+"
	case "adult":
		return "X"
	case "gambling":
		return "$"
	case "trackers":
		return "!"
	case "ads":
		return "AD"
	case "crypto":
		return "C"
	case "dating":
		return "&"
	case "p2p_torrent":
		return "U"
	default:
		return "G"
	}
}
