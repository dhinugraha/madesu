package main

import (
	"github.com/charmbracelet/lipgloss"
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

func render(t *testing.T, src string) string {
	t.Helper()
	return renderW(t, src, 60)
}

func renderW(t *testing.T, src string, width int) string {
	t.Helper()
	out, err := renderMarkdown([]byte(src), width, true)
	if err != nil {
		t.Fatal(err)
	}
	return plain(out)
}

func TestInlineMarkersConsumed(t *testing.T) {
	out := render(t, "a **tebal** b *miring* c ~~coret~~ d `kode` e\n")
	for _, raw := range []string{"**", "~~", "`"} {
		if strings.Contains(out, raw) {
			t.Errorf("marker %q masih tampil: %q", raw, out)
		}
	}
	for _, want := range []string{"tebal", "miring", "coret", "kode"} {
		if !strings.Contains(out, want) {
			t.Errorf("teks %q hilang: %q", want, out)
		}
	}
}

func TestHeadings(t *testing.T) {
	out := render(t, "# Satu\n\n## Dua\n\n###### Enam\n")
	for _, want := range []string{"# Satu", "## Dua", "###### Enam"} {
		if !strings.Contains(out, want) {
			t.Errorf("heading %q hilang: %q", want, out)
		}
	}
}

func TestLinksAndImages(t *testing.T) {
	// lebar besar supaya satu paragraf tidak ter-wrap di tengah URL
	out := renderW(t, "[contoh](https://example.com) dan ![alt](https://x.y/a.png) dan https://go.dev\n", 200)
	for _, want := range []string{"contoh <https://example.com>", "[img: alt] https://x.y/a.png", "https://go.dev"} {
		if !strings.Contains(out, want) {
			t.Errorf("tidak ada %q: %q", want, out)
		}
	}
}

func TestNestedLists(t *testing.T) {
	out := render(t, "- a\n  - b\n    - c\n\n1. satu\n2. dua\n")
	for _, want := range []string{"• a", "◦ b", "▪ c", "1. satu", "2. dua"} {
		if !strings.Contains(out, want) {
			t.Errorf("list %q hilang: %q", want, out)
		}
	}
	// list tight: sub-item harus langsung di bawah induknya, tanpa baris kosong.
	if strings.Contains(out, "• a\n\n") {
		t.Errorf("tight list punya baris kosong setelah induk: %q", out)
	}
}

func TestTaskList(t *testing.T) {
	out := render(t, "- [x] selesai\n- [ ] belum\n")
	if !strings.Contains(out, "☑ selesai") || !strings.Contains(out, "☐ belum") {
		t.Errorf("task list salah: %q", out)
	}
}

func TestBlockquoteNested(t *testing.T) {
	out := render(t, "> level 1\n> > level 2\n")
	if !strings.Contains(out, "│ level 1") || !strings.Contains(out, "level 2") {
		t.Errorf("blockquote salah: %q", out)
	}
}

func TestCodeBlock(t *testing.T) {
	out := render(t, "```go\nfunc main() {}\n```\n\n    indented\n")
	for _, want := range []string{"go", "func main() {}", "indented"} {
		if !strings.Contains(out, want) {
			t.Errorf("code block tidak punya %q: %q", want, out)
		}
	}
}

func TestTable(t *testing.T) {
	out := render(t, "| Kiri | Tengah | Kanan |\n|:-----|:------:|------:|\n| a | b | c |\n")
	if !strings.Contains(out, "│") || !strings.Contains(out, "┼") {
		t.Errorf("tabel tanpa border: %q", out)
	}
	if strings.Contains(out, "|---") || strings.Contains(out, ":-----") {
		t.Errorf("delimiter tabel mentah masih tampil: %q", out)
	}
}

func TestFootnote(t *testing.T) {
	out := render(t, "teks[^1] lanjut.\n\n[^1]: isi catatan\n")
	if !strings.Contains(out, "[1]") || !strings.Contains(out, "isi catatan") {
		t.Errorf("footnote salah: %q", out)
	}
	if strings.Contains(out, "[^1]") {
		t.Errorf("sintaks footnote mentah masih tampil: %q", out)
	}
}

func TestDefinitionList(t *testing.T) {
	out := render(t, "Istilah\n: definisi\n")
	if !strings.Contains(out, "Istilah") || !strings.Contains(out, ": definisi") {
		t.Errorf("definition list salah: %q", out)
	}
}

func TestEscapeAndThematicBreak(t *testing.T) {
	out := render(t, "Escape \\*bukan\\* miring\n\n---\n")
	if !strings.Contains(out, "*bukan*") {
		t.Errorf("escape salah: %q", out)
	}
	if !strings.Contains(out, "───") {
		t.Errorf("thematic break hilang: %q", out)
	}
}

func TestTableWrapsLongCells(t *testing.T) {
	src := "| # | Aksi | Jalur |\n|---|---|---|\n| G1 | Tambah `151578`, `151594` → respiration_rate + priority | `/parameters` web admin (tanpa rebuild) |\n"
	out := renderW(t, src, 60)
	for _, want := range []string{"respiration_rate", "priority", "rebuild", "151594"} {
		if !strings.Contains(out, want) {
			t.Errorf("sel terpotong, %q hilang: %q", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 60 {
			t.Errorf("baris tabel melebihi lebar 60 (%d): %q", w, line)
		}
	}
}

func TestRenderMermaidFence(t *testing.T) {
	src := "```mermaid\nflowchart TD\nA --> B\n```\n"
	out, err := renderMarkdown([]byte(src), 40, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[A]") || !strings.Contains(out, "▼") {
		t.Fatalf("mermaid not rendered as a diagram:\n%s", out)
	}
	if strings.Contains(out, "mermaid") {
		t.Fatalf("language label should be replaced by the diagram:\n%s", out)
	}
}

func TestRenderMermaidFallback(t *testing.T) {
	src := "```mermaid\nflowchart TD\nclassDef green fill:#9f6\nA --> B\n```\n"
	out, err := renderMarkdown([]byte(src), 40, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "mermaid") {
		t.Fatalf("fallback should keep the language label:\n%s", out)
	}
	if strings.Contains(out, "[A]") {
		t.Fatalf("unsupported diagram must not render as one:\n%s", out)
	}
}

func TestRenderMermaidCaseInsensitive(t *testing.T) {
	src := "```Mermaid\nflowchart TD\nA --> B\n```\n"
	out, err := renderMarkdown([]byte(src), 40, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "▼") {
		t.Fatalf("fence language should match case-insensitively:\n%s", out)
	}
}

func TestRenderMermaidSubgraphFlattened(t *testing.T) {
	src := "```mermaid\nflowchart TD\nsubgraph one[\"G\"]\nA --> B\nend\n```\n"
	out, err := renderMarkdown([]byte(src), 40, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[A]") || !strings.Contains(out, "▼") {
		t.Fatalf("subgraph block not flattened into a diagram:\n%s", out)
	}
}

func TestRenderMermaidIndentedFence(t *testing.T) {
	src := "    ```mermaid\n    flowchart LR\n    REQ[\"Request\"] --> UP[\"Upstream\"]\n    ```\n"
	out, err := renderMarkdown([]byte(src), 80, true)
	if err != nil {
		t.Fatal(err)
	}
	p := plain(out)
	if !strings.Contains(p, "[Request]") || !strings.Contains(p, "▶") {
		t.Fatalf("indented mermaid fence not rendered:\n%s", p)
	}
}
