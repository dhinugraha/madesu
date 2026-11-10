package main

// Renderer markdown ke teks ANSI untuk panel preview.
// Parser: goldmark dengan GFM (tabel, strikethrough, task list, autolink),
// footnote, dan definition list. Math ($...$) belum dirender, tampil apa adanya.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type mdStyles struct {
	h1, h2, h3, h4  lipgloss.Style
	bold, italic    lipgloss.Style
	strike          lipgloss.Style
	code, codeBlock lipgloss.Style
	link, muted     lipgloss.Style
	quote, hr       lipgloss.Style
	tableHead       lipgloss.Style
}

func newMDStyles(dark bool) mdStyles {
	pick := func(light, darkC string) lipgloss.Color {
		if dark {
			return lipgloss.Color(darkC)
		}
		return lipgloss.Color(light)
	}
	accent := pick("25", "117")
	return mdStyles{
		h1:        lipgloss.NewStyle().Bold(true).Foreground(accent),
		h2:        lipgloss.NewStyle().Bold(true).Foreground(accent),
		h3:        lipgloss.NewStyle().Bold(true),
		h4:        lipgloss.NewStyle().Italic(true).Bold(true),
		bold:      lipgloss.NewStyle().Bold(true),
		italic:    lipgloss.NewStyle().Italic(true),
		strike:    lipgloss.NewStyle().Strikethrough(true),
		code:      lipgloss.NewStyle().Foreground(pick("124", "216")),
		codeBlock: lipgloss.NewStyle().Foreground(pick("235", "252")).Background(pick("254", "236")),
		link:      lipgloss.NewStyle().Underline(true).Foreground(pick("27", "75")),
		muted:     lipgloss.NewStyle().Faint(true),
		quote:     lipgloss.NewStyle().Foreground(pick("240", "245")),
		hr:        lipgloss.NewStyle().Faint(true),
		tableHead: lipgloss.NewStyle().Bold(true),
	}
}

type mdRenderer struct {
	src []byte
	st  mdStyles
}

// renderMarkdown mengubah src jadi teks ANSI yang di-wrap ke width kolom.
func renderMarkdown(src []byte, width int, dark bool) (string, error) {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList, extension.Footnote))
	doc := md.Parser().Parse(text.NewReader(src))
	r := &mdRenderer{src: src, st: newMDStyles(dark)}

	var parts []string
	var notes []ast.Node
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() == extast.KindFootnoteList {
			notes = append(notes, c)
			continue
		}
		if s := r.block(c, width, 0); s != "" {
			parts = append(parts, s)
		}
	}
	for _, fl := range notes {
		parts = append(parts, r.st.hr.Render(strings.Repeat("─", max(1, min(width, 40)))))
		for fn := fl.FirstChild(); fn != nil; fn = fn.NextSibling() {
			f, ok := fn.(*extast.Footnote)
			if !ok {
				continue
			}
			var body []string
			for c := f.FirstChild(); c != nil; c = c.NextSibling() {
				if s := r.block(c, width-4, 0); s != "" {
					body = append(body, s)
				}
			}
			label := r.st.muted.Render(fmt.Sprintf("[%d] ", f.Index))
			parts = append(parts, indentFirst(strings.Join(body, "\n"), label, "    "))
		}
	}
	return strings.TrimRight(strings.Join(parts, "\n\n"), "\n"), nil
}

// block merender satu node blok. indent dipakai untuk wrap lebar di dalam list/quote.
func (r *mdRenderer) block(n ast.Node, width, depth int) string {
	switch n.Kind() {
	case ast.KindParagraph, ast.KindTextBlock:
		return lipgloss.NewStyle().Width(width).Render(r.inline(n))

	case ast.KindHeading:
		h := n.(*ast.Heading)
		var st lipgloss.Style
		switch h.Level {
		case 1:
			st = r.st.h1
		case 2:
			st = r.st.h2
		case 3:
			st = r.st.h3
		default:
			st = r.st.h4
		}
		marker := r.st.muted.Render(strings.Repeat("#", h.Level) + " ")
		return marker + st.Width(width).Render(r.inline(n))

	case ast.KindBlockquote:
		inner := r.blocks(n, width-2, depth)
		lines := strings.Split(inner, "\n")
		for i, l := range lines {
			lines[i] = r.st.quote.Render("│ ") + l
		}
		return strings.Join(lines, "\n")

	case ast.KindList:
		return r.list(n.(*ast.List), width, depth)

	case ast.KindFencedCodeBlock, ast.KindCodeBlock:
		return r.codeBlock(n, width)

	case ast.KindThematicBreak:
		return r.st.hr.Render(strings.Repeat("─", max(1, min(width, 60))))

	case ast.KindHTMLBlock:
		return r.st.muted.Render(strings.TrimRight(string(r.rawLines(n)), "\n"))

	case extast.KindTable:
		return r.table(n, width)

	case extast.KindDefinitionList:
		var parts []string
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c.Kind() {
			case extast.KindDefinitionTerm:
				parts = append(parts, r.st.bold.Render(r.inline(c)))
			case extast.KindDefinitionDescription:
				body := r.blocks(c, width-2, depth)
				parts = append(parts, indentFirst(body, r.st.muted.Render(": "), "  "))
			}
		}
		return strings.Join(parts, "\n")

	case extast.KindFootnoteList:
		return ""

	default:
		// Container tak dikenal: render anaknya saja supaya isi tidak hilang.
		return r.blocks(n, width, depth)
	}
}

// blocks render semua anak blok, dipisah baris kosong.
func (r *mdRenderer) blocks(n ast.Node, width, depth int) string {
	return r.blocksSep(n, width, depth, "\n\n")
}

// blocksSep sama seperti blocks, dengan pemisah antar blok yang bisa diatur
// (list tight pakai "\n" supaya sub-list tidak terpisah baris kosong).
func (r *mdRenderer) blocksSep(n ast.Node, width, depth int, sep string) string {
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := r.block(c, width, depth); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, sep)
}

func (r *mdRenderer) list(l *ast.List, width, depth int) string {
	bullets := []string{"•", "◦", "▪"}
	var items []string
	n := 1
	if l.Start > 0 {
		n = l.Start
	}
	inner := "\n\n"
	if l.IsTight {
		inner = "\n"
	}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		var marker string
		if l.IsOrdered() {
			marker = strconv.Itoa(n) + ". "
			n++
		} else {
			marker = bullets[depth%len(bullets)] + " "
		}
		body := r.blocksSep(it, width-len(marker), depth+1, inner)
		items = append(items, indentFirst(body, r.st.muted.Render(marker), strings.Repeat(" ", len(marker))))
	}
	sep := "\n"
	if !l.IsTight {
		sep = "\n\n"
	}
	return strings.Join(items, sep)
}

func (r *mdRenderer) codeBlock(n ast.Node, width int) string {
	if fc, ok := n.(*ast.FencedCodeBlock); ok {
		if lang := fc.Language(r.src); strings.EqualFold(strings.TrimSpace(string(lang)), "mermaid") {
			if out := r.mermaid(r.rawLines(n)); out != "" {
				return out
			}
		}
		return r.plainCode(n)
	}
	// A fence indented four or more spaces parses as an indented code block;
	// still render mermaid inside it rather than printing the fence as text.
	if body, ok := mermaidFenceBody(r.rawLines(n)); ok {
		if out := r.mermaid(body); out != "" {
			return out
		}
	}
	return r.plainCode(n)
}

// mermaidFenceBody unwraps a mermaid fence that markdown parsed as an
// indented code block.
func mermaidFenceBody(raw []byte) ([]byte, bool) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 {
		return nil, false
	}
	first := strings.TrimSpace(lines[0])
	lower := strings.ToLower(first)
	if !strings.HasPrefix(lower, "```mermaid") && !strings.HasPrefix(lower, "~~~mermaid") {
		return nil, false
	}
	fence := first[:3]
	end := len(lines)
	for end > 1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if end > 1 && strings.TrimSpace(lines[end-1]) == fence {
		end--
	}
	return []byte(strings.Join(lines[1:end], "\n")), true
}

// mermaid renders flowchart source, or "" to fall back to plain rendering.
func (r *mdRenderer) mermaid(raw []byte) string {
	g, err := parseFlowchart(raw)
	if err != nil {
		return ""
	}
	out, err := layoutFlowchartStyled(g, r.st.muted)
	if err != nil {
		return ""
	}
	return strings.Join(out, "\n")
}

// plainCode is the original code-block rendering: language label plus lines.
func (r *mdRenderer) plainCode(n ast.Node) string {
	var b strings.Builder
	if fc, ok := n.(*ast.FencedCodeBlock); ok {
		if lang := fc.Language(r.src); len(lang) > 0 {
			b.WriteString(r.st.muted.Render(string(lang)) + "\n")
		}
	}
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		line := strings.TrimRight(string(seg.Value(r.src)), "\n")
		b.WriteString(r.st.codeBlock.Render(" " + line + " "))
		if i < lines.Len()-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (r *mdRenderer) table(n ast.Node, width int) string {
	var rows [][]string
	var aligns []extast.Alignment
	var header []bool
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		isHead := row.Kind() == extast.KindTableHeader
		var cells []string
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			tc, ok := c.(*extast.TableCell)
			if !ok {
				continue
			}
			cell := r.inline(tc)
			if isHead {
				cell = r.st.tableHead.Render(cell)
			}
			cells = append(cells, cell)
			if isHead {
				aligns = append(aligns, tc.Alignment)
			}
		}
		rows = append(rows, cells)
		header = append(header, isHead)
	}
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	widths := tableColumnWidths(rows, cols, width-3*(cols-1))

	var out []string
	for ri, row := range rows {
		out = append(out, renderTableRow(row, widths, cols, alignFor(aligns, cols))...)
		if ri == 0 && header[0] {
			seps := make([]string, cols)
			for i, w := range widths {
				seps[i] = strings.Repeat("─", w)
			}
			out = append(out, r.st.hr.Render(strings.Join(seps, "─┼─")))
		}
	}
	return strings.Join(out, "\n")
}

// tableColumnWidths membagi budget ke kolom. Kalau muat, tiap kolom pakai lebar isinya.
// Kalau tidak, budget dibagi sebanding dengan lebar isi, minimal 4 kolom per kolom.
func tableColumnWidths(rows [][]string, cols, budget int) []int {
	nat := make([]int, cols)
	for _, row := range rows {
		for i, c := range row {
			nat[i] = max(nat[i], lipgloss.Width(c))
		}
	}
	total := 0
	for _, w := range nat {
		total += w
	}
	if total <= budget {
		return nat
	}
	widths := make([]int, cols)
	sum := 0
	for i, w := range nat {
		widths[i] = max(4, w*budget/max(total, 1))
		sum += widths[i]
	}
	// Minimum 4 bisa membuat jumlah melebihi budget; kurangi kolom terlebar sampai muat.
	for sum > budget {
		wi := 0
		for i := range widths {
			if widths[i] > widths[wi] {
				wi = i
			}
		}
		if widths[wi] <= 1 {
			break
		}
		widths[wi]--
		sum--
	}
	return widths
}

func alignFor(aligns []extast.Alignment, cols int) []extast.Alignment {
	out := make([]extast.Alignment, cols)
	copy(out, aligns)
	return out
}

// renderTableRow membungkus setiap sel ke lebar kolomnya lalu menggabungkan baris per baris,
// sehingga teks panjang tampil penuh di beberapa baris, bukan dipotong.
func renderTableRow(row []string, widths []int, cols int, aligns []extast.Alignment) []string {
	cellLines := make([][]string, cols)
	height := 1
	for ci := 0; ci < cols; ci++ {
		c := ""
		if ci < len(row) {
			c = row[ci]
		}
		st := lipgloss.NewStyle().Width(widths[ci]).Align(tableAlign(aligns[ci]))
		lines := strings.Split(st.Render(c), "\n")
		cellLines[ci] = lines
		height = max(height, len(lines))
	}
	lines := make([]string, height)
	for li := 0; li < height; li++ {
		parts := make([]string, cols)
		for ci := 0; ci < cols; ci++ {
			if li < len(cellLines[ci]) {
				parts[ci] = cellLines[ci][li]
			} else {
				parts[ci] = strings.Repeat(" ", widths[ci])
			}
		}
		lines[li] = strings.Join(parts, " │ ")
	}
	return lines
}

func tableAlign(a extast.Alignment) lipgloss.Position {
	switch a {
	case extast.AlignRight:
		return lipgloss.Right
	case extast.AlignCenter:
		return lipgloss.Center
	default:
		return lipgloss.Left
	}
}

// inline merender teks inline dari anak node. Hasil belum di-wrap.
func (r *mdRenderer) inline(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			b.Write(r.unescape(c.Segment.Value(r.src)))
			if c.HardLineBreak() {
				b.WriteString("\n")
			} else if c.SoftLineBreak() {
				b.WriteString(" ")
			}
		case *ast.String:
			b.Write(c.Value)
		case *ast.Emphasis:
			st := r.st.italic
			if c.Level >= 2 {
				st = r.st.bold
			}
			b.WriteString(st.Render(r.inline(c)))
		case *extast.Strikethrough:
			b.WriteString(r.st.strike.Render(r.inline(c)))
		case *ast.CodeSpan:
			var code strings.Builder
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if tx, ok := t.(*ast.Text); ok {
					code.Write(tx.Segment.Value(r.src))
				}
			}
			b.WriteString(r.st.code.Render(code.String()))
		case *ast.Link:
			text := r.inline(c)
			dest := string(c.Destination)
			b.WriteString(r.st.link.Render(text))
			if dest != "" && dest != text {
				b.WriteString(r.st.muted.Render(" <" + dest + ">"))
			}
		case *ast.AutoLink:
			b.WriteString(r.st.link.Render(string(c.URL(r.src))))
		case *ast.Image:
			alt := r.inline(c)
			b.WriteString(r.st.muted.Render("[img: " + alt + "] " + string(c.Destination)))
		case *ast.RawHTML:
			for i := 0; i < c.Segments.Len(); i++ {
				seg := c.Segments.At(i)
				b.WriteString(r.st.muted.Render(string(seg.Value(r.src))))
			}
		case *extast.TaskCheckBox:
			if c.IsChecked {
				b.WriteString("☑ ")
			} else {
				b.WriteString("☐ ")
			}
		case *extast.FootnoteLink:
			b.WriteString(r.st.muted.Render(fmt.Sprintf("[%d]", c.Index)))
		case *extast.FootnoteBacklink:
			// tidak ditampilkan; tautan balik tidak berguna di terminal
		default:
			b.WriteString(r.inline(c))
		}
	}
	return b.String()
}

// unescape menghapus backslash escape (\*) dan men-resolve entity (&amp;).
// goldmark menyimpan teks mentah; penghapusan escape biasanya dilakukan writer HTML.
func (r *mdRenderer) unescape(b []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(b)))
}

func (r *mdRenderer) rawLines(n ast.Node) []byte {
	var out []byte
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out = append(out, seg.Value(r.src)...)
	}
	return out
}

// indentFirst memberi marker di baris pertama dan spasi di baris lanjutan.
func indentFirst(body, first, rest string) string {
	lines := strings.Split(body, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = first + lines[i]
		} else {
			lines[i] = rest + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}
