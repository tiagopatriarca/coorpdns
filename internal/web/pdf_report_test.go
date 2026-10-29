package web

import (
	"bytes"
	"testing"

	"github.com/coorpdns/coorpdns/internal/storage"
)

func TestGenerateGroupReportPDF(t *testing.T) {
	mockReport := &storage.GroupReportResult{
		GroupID:        2,
		GroupName:      "Smart TVs & Streaming",
		Description:    "Televisores e reprodutores de mídia",
		GroupColor:     "#38bdf8",
		FromDate:       "2026-10-22",
		ToDate:         "2026-10-29",
		DevicesCount:   2,
		TotalQueries:   1540,
		BlockedQueries: 320,
		BlockRate:      20.8,
		Devices: []storage.GroupDeviceActivity{
			{
				ClientIP:       "192.168.1.15",
				ClientName:     "Smart TV Sala",
				IsRemote:       false,
				TotalQueries:   1000,
				BlockedQueries: 200,
				BlockRate:      20.0,
				Domains: []storage.ClientDomainActivityItem{
					{
						Domain:         "netflix.com",
						Category:       "streaming",
						CategoryName:   "Streaming de Vídeo",
						TotalQueries:   500,
						BlockedQueries: 0,
					},
					{
						Domain:         "fls-na.amazon.com",
						Category:       "trackers",
						CategoryName:   "Rastreadores & Telemetria",
						TotalQueries:   200,
						BlockedQueries: 200,
					},
				},
			},
			{
				ClientIP:       "remote-tok_test1234",
				ClientName:     "Notebook Diretoria",
				IsRemote:       true,
				TotalQueries:   540,
				BlockedQueries: 120,
				BlockRate:      22.2,
				Domains: []storage.ClientDomainActivityItem{
					{
						Domain:         "chatgpt.com",
						Category:       "general",
						CategoryName:   "ChatGPT (IA)",
						TotalQueries:   300,
						BlockedQueries: 0,
					},
				},
			},
		},
	}

	pdfBytes, err := GenerateGroupReportPDF(mockReport, 7)
	if err != nil {
		t.Fatalf("Erro ao gerar PDF: %v", err)
	}

	if len(pdfBytes) < 500 {
		t.Fatalf("PDF gerado muito pequeno: %d bytes", len(pdfBytes))
	}

	// Verifica cabeçalho padrão de arquivos PDF (%PDF-)
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("Arquivo gerado não possui assinatura válida de PDF")
	}
}
