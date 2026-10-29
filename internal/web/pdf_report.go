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
func GenerateGroupReportPDF(report *storage.GroupDetailedReport, days int) ([]byte, error) {
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

	if report.GroupDescription != "" {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(190, 5, latin(fmt.Sprintf("Descrição: %s", report.GroupDescription)), "", 1, "L", false, 0, "")
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

	pdf.SetY(currentY + cardH + 6)

	// 2. CORPO: Por Máquina (Nome da máquina -> Categoria -> Site -> Acessos -> Bloqueado ou Não)
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
