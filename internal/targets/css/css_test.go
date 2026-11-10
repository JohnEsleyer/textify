package css

import (
	"strings"
	"testing"
)

const sampleCSS = `:root {
	--primary-color: #0070f3;
	--font-main: 'Inter', sans-serif;
}

@media (max-width: 768px) {
	.mobile-nav {
		display: block;
		padding: 10px;
	}
}

.btn-primary {
	background-color: var(--primary-color);
	color: #fff;
	border-radius: 4px;
	padding: 8px 16px;
}
`

func TestCSSSignatures(t *testing.T) {
	target := &CSSTarget{}
	if !target.Match("styles/theme.scss") {
		t.Errorf("CSSTarget should match .scss")
	}

	sigs := target.ExtractSignatures("styles/theme.css", sampleCSS)
	if !strings.Contains(sigs, "--primary-color: #0070f3;") {
		t.Errorf("signatures should preserve :root variables: %s", sigs)
	}
	if !strings.Contains(sigs, "@media (max-width: 768px)") {
		t.Errorf("signatures should preserve @media queries: %s", sigs)
	}
	if !strings.Contains(sigs, ".btn-primary { /* ... */ }") {
		t.Errorf("signatures should collapse rule properties: %s", sigs)
	}
	if strings.Contains(sigs, "border-radius: 4px;") {
		t.Errorf("signatures should not contain inner properties: %s", sigs)
	}
}
