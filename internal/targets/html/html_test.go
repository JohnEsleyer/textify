package html

import (
	"strings"
	"testing"
)

const sampleHTML = `<!DOCTYPE html>
<html>
<head>
	<title>Test Page</title>
	<style>
		body { background: #fff; }
	</style>
</head>
<body>
	<div id="root" class="container">
		<p>Some lengthy user text paragraph here...</p>
	</div>
	<script src="/bundle.js"></script>
	<script>
		console.log("embedded script");
	</script>
</body>
</html>
`

func TestHTMLSignatures(t *testing.T) {
	target := &HTMLTarget{}
	if !target.Match("public/index.html") {
		t.Errorf("HTMLTarget should match .html")
	}
	if !target.Match("src/App.svelte") {
		t.Errorf("HTMLTarget should match .svelte")
	}

	sigs := target.ExtractSignatures("index.html", sampleHTML)
	if !strings.Contains(sigs, "<div id=\"root\" class=\"container\">") {
		t.Errorf("signatures should preserve structural tags and IDs: %s", sigs)
	}
	if !strings.Contains(sigs, "/* styles omitted */") {
		t.Errorf("signatures should collapse inline styles: %s", sigs)
	}
	if !strings.Contains(sigs, "/* script content omitted */") {
		t.Errorf("signatures should collapse embedded scripts: %s", sigs)
	}
	if !strings.Contains(sigs, "<script src=\"/bundle.js\">") {
		t.Errorf("signatures should preserve external script tags: %s", sigs)
	}
}
