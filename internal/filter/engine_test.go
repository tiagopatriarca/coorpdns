package filter

import (
	"testing"
)

func TestAdditivePolicyAndGroupDomains(t *testing.T) {
	eng := NewEngine()

	// 1. Domínio liberado explicitamente no grupo (allowlist de grupo) deve liberar
	domainRules := map[string]string{
		"allowed.com": "allow",
		"blocked.com": "block",
	}
	catRules := map[string]string{
		"adult": "block",
	}
	appRules := map[string]string{
		"chatgpt": "block",
	}

	// allowed.com deve ser liberado pela allowlist do grupo
	blocked, rule := eng.CheckPolicy("allowed.com", domainRules, appRules, catRules, false, "", false)
	if blocked {
		t.Fatalf("allowed.com deveria ser liberado pela allowlist do grupo, mas foi bloqueado por %s", rule)
	}

	// subdomínio de allowed.com também deve ser liberado
	blocked, rule = eng.CheckPolicy("sub.allowed.com", domainRules, appRules, catRules, false, "", false)
	if blocked {
		t.Fatalf("sub.allowed.com deveria ser liberado pela allowlist do grupo, mas foi bloqueado por %s", rule)
	}

	// 2. Domínio bloqueado no grupo deve ser bloqueado
	blocked, rule = eng.CheckPolicy("blocked.com", domainRules, appRules, catRules, false, "", false)
	if !blocked || rule != "group-domain:blocked.com" {
		t.Fatalf("blocked.com deveria ser bloqueado por group-domain:blocked.com, obteve blocked=%v, rule=%s", blocked, rule)
	}

	// 3. Lógica Aditiva: Aplicação 'chatgpt' bloqueada (chatgpt.com)
	blocked, rule = eng.CheckPolicy("chatgpt.com", nil, appRules, nil, false, "", false)
	if !blocked || rule != "app:chatgpt" {
		t.Fatalf("chatgpt.com deveria ser bloqueado por app:chatgpt, obteve %v (%s)", blocked, rule)
	}

	// 4. Lógica Aditiva: Categoria 'adult' bloqueada (xvideos.com) com appRules desmarcada/vazia
	blocked, rule = eng.CheckPolicy("xvideos.com", nil, nil, catRules, false, "", false)
	if !blocked || rule != "category:adult" {
		t.Fatalf("xvideos.com deveria ser bloqueado por category:adult, obteve %v (%s)", blocked, rule)
	}

	// 5. Lógica Aditiva: Aplicação desmarcada (não presente em appRules) NÃO anula bloqueio de categoria
	// spotify.com pertence à categoria 'streaming' e app 'spotify'
	streamCatRules := map[string]string{"streaming": "block"}
	emptyAppRules := map[string]string{} // desmarcada na aba de aplicações

	blocked, rule = eng.CheckPolicy("spotify.com", nil, emptyAppRules, streamCatRules, false, "", false)
	if !blocked || rule != "category:streaming" {
		t.Fatalf("spotify.com com app desmarcada mas categoria bloqueada DEVE ser bloqueado por category:streaming! Obteve: blocked=%v, rule=%s", blocked, rule)
	}
}

func TestNormalizeCustomDomainInput(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		hasErr   bool
	}{
		{"https://www.tiktok.com/", "www.tiktok.com", false},
		{"http://bet365.com/sports", "bet365.com", false},
		{"*.youtube.com", "youtube.com", false},
		{"||blaze.com^", "blaze.com", false},
		{"sub.dominio.empresa.local:8080", "sub.dominio.empresa.local", false},
		{"invalido", "", true},
		{"", "", true},
		{"site inválido com espaço.com", "", true},
	}

	for _, c := range cases {
		out, err := NormalizeCustomDomainInput(c.input)
		if (err != nil) != c.hasErr {
			t.Errorf("NormalizeCustomDomainInput(%q) err=%v, esperado erro=%v", c.input, err, c.hasErr)
		}
		if !c.hasErr && out != c.expected {
			t.Errorf("NormalizeCustomDomainInput(%q) = %q, esperado %q", c.input, out, c.expected)
		}
	}
}
