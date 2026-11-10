package main

// Simplified layered layout for parsed flowcharts: break cycles, rank by
// longest path, order ranks with barycenter sweeps. Grid placement and edge
// routing live in the rest of this file (later tasks).

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type box struct {
	n       *flowNode // nil for a virtual bend point
	virtual bool
	edgeIx  int
	rank    int
	order   int
	x, w, h int
}

type lchain struct {
	e      *flowEdge
	nodes  []*box // low rank → high rank
	hiIsTo bool   // the drawn arrow points at the high end
}

type layout struct {
	buckets [][]*box // per rank, in draw order
	chains  []*lchain
	rank    map[string]int
}

type workEdge struct {
	e   flowEdge
	rev bool // reversed for ranking because it closes a cycle
}

// breakCycles marks back edges via DFS so ranking sees an acyclic graph.
// Drawn edges keep the user's direction.
func breakCycles(g *flowGraph) []workEdge {
	adj := map[string][]int{}
	for i, e := range g.edges {
		adj[e.from] = append(adj[e.from], i)
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	state := make(map[string]int, len(g.nodes))
	rev := make([]bool, len(g.edges))
	var dfs func(string)
	dfs = func(id string) {
		state[id] = gray
		for _, ei := range adj[id] {
			to := g.edges[ei].to
			switch state[to] {
			case white:
				dfs(to)
			case gray:
				rev[ei] = true
			}
		}
		state[id] = black
	}
	for _, n := range g.nodes {
		if state[n.id] == white {
			dfs(n.id)
		}
	}
	out := make([]workEdge, len(g.edges))
	for i, e := range g.edges {
		out[i] = workEdge{e: e, rev: rev[i]}
	}
	return out
}

// rankNodes assigns longest-path ranks over the acyclic working edge set.
func rankNodes(nodes []*flowNode, work []workEdge) map[string]int {
	indeg := map[string]int{}
	for _, w := range work {
		if !w.rev {
			indeg[w.e.to]++
		}
	}
	rank := map[string]int{}
	var q []string
	for _, n := range nodes {
		if indeg[n.id] == 0 {
			q = append(q, n.id)
		}
	}
	for len(q) > 0 {
		id := q[0]
		q = q[1:]
		for _, w := range work {
			if w.rev || w.e.from != id {
				continue
			}
			if r := rank[id] + 1; r > rank[w.e.to] {
				rank[w.e.to] = r
			}
			indeg[w.e.to]--
			if indeg[w.e.to] == 0 {
				q = append(q, w.e.to)
			}
		}
	}
	return rank
}

// planLayout builds boxes (one per node per rank, plus virtual bend points
// for edges spanning multiple ranks), chains, and an ordered bucket list.
func planLayout(g *flowGraph) (*layout, error) {
	seen := make(map[[2]string]bool, len(g.edges))
	for _, e := range g.edges {
		if e.from == e.to {
			return nil, errUnsupported
		}
		k := [2]string{e.from, e.to}
		// Two edges on the same pair (parallel, or opposite directions)
		// would share a routing column and overwrite each other.
		if seen[k] || seen[[2]string{e.to, e.from}] {
			return nil, errUnsupported
		}
		seen[k] = true
	}
	if len(g.nodes) == 0 {
		return nil, errUnsupported
	}
	work := breakCycles(g)
	rank := rankNodes(g.nodes, work)

	byID := map[string]*flowNode{}
	for _, n := range g.nodes {
		byID[n.id] = n
	}
	key := func(id string, r int) string { return id + "\x00" + strconv.Itoa(r) }
	boxes := map[string]*box{}
	getBox := func(id string, r int) *box {
		k := key(id, r)
		b := boxes[k]
		if b == nil {
			b = &box{n: byID[id], rank: r}
			boxes[k] = b
		}
		return b
	}

	var chains []*lchain
	for i := range g.edges {
		e := &g.edges[i]
		lo, hi := e.from, e.to
		loR, hiR := rank[lo], rank[hi]
		hiIsTo := true
		if loR > hiR {
			lo, hi = hi, lo
			loR, hiR = hiR, loR
			hiIsTo = false
		}
		c := &lchain{e: e, hiIsTo: hiIsTo}
		for r := loR; r <= hiR; r++ {
			var b *box
			switch r {
			case loR:
				b = getBox(lo, r)
			case hiR:
				b = getBox(hi, r)
			default:
				b = &box{virtual: true, edgeIx: i, rank: r}
			}
			c.nodes = append(c.nodes, b)
		}
		chains = append(chains, c)
	}
	// Standalone definitions never mentioned by an edge still get a box.
	for _, n := range g.nodes {
		getBox(n.id, rank[n.id])
	}

	maxRank := 0
	for _, n := range g.nodes {
		if r := rank[n.id]; r > maxRank {
			maxRank = r
		}
	}
	buckets := make([][]*box, maxRank+1)
	for _, n := range g.nodes {
		b := boxes[key(n.id, rank[n.id])]
		buckets[b.rank] = append(buckets[b.rank], b)
	}
	for _, c := range chains {
		for _, b := range c.nodes {
			if b.virtual {
				buckets[b.rank] = append(buckets[b.rank], b)
			}
		}
	}
	for r := range buckets {
		for i, b := range buckets[r] {
			b.order = i
		}
	}
	orderBoxes(buckets, chains)
	return &layout{buckets: buckets, chains: chains, rank: rank}, nil
}

// orderBoxes runs four barycenter sweeps to reduce edge crossings. Stable
// sorting keeps ties in place, so identical input always yields one output.
func orderBoxes(buckets [][]*box, chains []*lchain) {
	nb := map[*box][]*box{}
	for _, c := range chains {
		for i, b := range c.nodes {
			if i > 0 {
				nb[b] = append(nb[b], c.nodes[i-1])
			}
			if i+1 < len(c.nodes) {
				nb[b] = append(nb[b], c.nodes[i+1])
			}
		}
	}
	bc := func(b *box, from int) float64 {
		sum, n := 0.0, 0.0
		for _, o := range nb[b] {
			if o.rank == from {
				sum += float64(o.order)
				n++
			}
		}
		if n == 0 {
			return float64(b.order)
		}
		return sum / n
	}
	for iter := 0; iter < 4; iter++ {
		if iter%2 == 0 {
			for r := 1; r < len(buckets); r++ {
				br := buckets[r]
				sort.SliceStable(br, func(i, j int) bool {
					return bc(br[i], r-1) < bc(br[j], r-1)
				})
				reindex(br)
			}
		} else {
			for r := len(buckets) - 2; r >= 0; r-- {
				br := buckets[r]
				sort.SliceStable(br, func(i, j int) bool {
					return bc(br[i], r+1) < bc(br[j], r+1)
				})
				reindex(br)
			}
		}
	}
}

func reindex(br []*box) {
	for i, b := range br {
		b.order = i
	}
}

const (
	maxLabelCols = 24 // longest node label before truncation
	intraGap     = 3  // empty columns between boxes in one rank
)

type cell struct {
	ch     rune
	chrome bool // line/box drawing, rendered muted by styled variant
}

// layoutFlowchart renders the graph to plain text.
func layoutFlowchart(g *flowGraph) ([]string, error) {
	cells, err := buildGrid(g)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cells))
	for i, row := range cells {
		var b strings.Builder
		for _, c := range row {
			b.WriteRune(c.ch)
		}
		out[i] = strings.TrimRight(b.String(), " ")
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out, nil
}

func buildGrid(g *flowGraph) ([][]cell, error) {
	l, err := planLayout(g)
	if err != nil {
		return nil, err
	}
	// Column placement within each rank.
	for _, bs := range l.buckets {
		x := 0
		for _, b := range bs {
			if b.virtual {
				b.w, b.h = 1, 1
			} else {
				_, b.w, b.h = nodeBox(b.n)
			}
			b.x = x
			x += b.w + intraGap
		}
	}
	// Gutter rows between consecutive ranks: 1 bare, 2 with a label,
	// 3 when an edge must bend.
	maxRank := len(l.buckets) - 1
	horiz := g.dir == dirLR || g.dir == dirRL
	gh := make([]int, maxRank)
	for r := range gh {
		h := 1
		for _, c := range l.chains {
			for i := 0; i+1 < len(c.nodes); i++ {
				if c.nodes[i].rank != r || c.nodes[i+1].rank != r+1 {
					continue
				}
				if crossCenter(c.nodes[i], horiz) != crossCenter(c.nodes[i+1], horiz) {
					h = 3
				} else if h < 2 && (c.e.label != "" || c.e.style != edgeNormal || c.e.bidir) {
					// A label, a dotted/thick segment, or a second arrowhead
					// needs a second row.
					h = 2
				}
			}
		}
		gh[r] = h
	}
	// Along-axis offsets. A rank spans its tallest box — or its widest when
	// the flow runs horizontally.
	yTop := make([]int, len(l.buckets))
	y := 0
	for r := range l.buckets {
		yTop[r] = y
		y += rankExtent(l.buckets[r], horiz)
		if r < len(gh) {
			y += gh[r]
		}
	}
	crossLen := 0
	for _, bs := range l.buckets {
		for _, b := range bs {
			crossLen = max(crossLen, b.x+crossExtent(b, horiz))
		}
	}
	// +8: headroom so edge labels never clip; trailing space is trimmed.
	cv := newCanvas(horiz, y+8, crossLen+8)

	// Boxes.
	for r, bs := range l.buckets {
		re := rankExtent(bs, horiz)
		for _, b := range bs {
			if b.virtual {
				continue
			}
			lines, _, h := nodeBox(b.n)
			// Center along the flow axis using the dimension that axis sees,
			// so horizontal layouts do not offset boxes by their height.
			al := h
			if horiz {
				al = b.w
			}
			bx := yTop[r] + (re-al)/2
			for i, line := range lines {
				j := 0 // column index: range over a string yields byte offsets
				for _, ch := range line {
					a, c := bx+i, b.x+j
					if cv.horiz { // keep text horizontal on the final grid
						a, c = bx+j, b.x+i
					}
					cv.put(a, c, cell{ch: ch, chrome: true})
					j++
				}
			}
		}
	}
	// Edges. (horiz is already in scope from the canvas setup.)
	flip := g.dir == dirBT || g.dir == dirRL
	for _, c := range l.chains {
		for i := 0; i+1 < len(c.nodes); i++ {
			b1, b2 := c.nodes[i], c.nodes[i+1]
			r := b1.rank
			y0 := yTop[r] + rankExtent(l.buckets[r], horiz)
			gt := gh[r]
			x1, x2 := crossCenter(b1, horiz), crossCenter(b2, horiz)
			last := i+2 == len(c.nodes)
			seg := segGlyph(horiz, c.e.style, true)
			if x1 == x2 {
				for k := 0; k < gt; k++ {
					cv.put(y0+k, x1, cell{ch: seg, chrome: true})
				}
				if last && (c.hiIsTo || c.e.bidir) {
					cv.put(y0+gt-1, x1, cell{ch: arrowGlyph(horiz, true, flip), chrome: true})
				}
				if i == 0 && (!c.hiIsTo || c.e.bidir) {
					cv.put(y0, x1, cell{ch: arrowGlyph(horiz, false, flip), chrome: true})
				}
				continue
			}
			// Orthogonal bend through the middle gutter row.
			cv.put(y0, x1, cell{ch: seg, chrome: true})
			cv.put(y0+1, x1, cell{ch: cornerGlyph(horiz, true, x2 > x1), chrome: true})
			for x := min(x1, x2) + 1; x < max(x1, x2); x++ {
				cv.put(y0+1, x, cell{ch: segGlyph(horiz, c.e.style, false), chrome: true})
			}
			cv.put(y0+1, x2, cell{ch: cornerGlyph(horiz, false, x2 > x1), chrome: true})
			cv.put(y0+2, x2, cell{ch: seg, chrome: true})
			if last && (c.hiIsTo || c.e.bidir) {
				cv.put(y0+2, x2, cell{ch: arrowGlyph(horiz, true, flip), chrome: true})
			}
			if i == 0 && (!c.hiIsTo || c.e.bidir) {
				cv.put(y0, x1, cell{ch: arrowGlyph(horiz, false, flip), chrome: true})
			}
		}
		// Label on the first gutter row of the edge's first segment,
		// written into empty cells only.
		if c.e.label != "" {
			b1, b2 := c.nodes[0], c.nodes[1]
			r := b1.rank
			y0 := yTop[r] + rankExtent(l.buckets[r], horiz)
			x1, x2 := crossCenter(b1, horiz), crossCenter(b2, horiz)
			lx := x1 + 1
			if x1 != x2 {
				lx = min(x1, x2) + 1
			}
			putLabel(cv, y0, lx, c.e.label)
		}
	}
	return cv.cells, nil
}

type canvas struct {
	cells [][]cell
	horiz bool
}

// newCanvas allocates a canvas of final dimensions. Rank-space coordinates
// (along a, cross c) map to (row, col) = (a, c), or (c, a) when horizontal.
func newCanvas(horiz bool, alongLen, crossLen int) *canvas {
	rows, cols := alongLen, crossLen
	if horiz {
		rows, cols = crossLen, alongLen
	}
	c := &canvas{horiz: horiz, cells: make([][]cell, rows)}
	for i := range c.cells {
		c.cells[i] = make([]cell, cols)
		for j := range c.cells[i] {
			c.cells[i][j].ch = ' '
		}
	}
	return c
}

func (cv *canvas) put(a, c int, cell cell) {
	if cv.horiz {
		if c < 0 || c >= len(cv.cells) || a < 0 || a >= len(cv.cells[c]) {
			return
		}
		cv.cells[c][a] = cell
		return
	}
	if a < 0 || a >= len(cv.cells) || c < 0 || c >= len(cv.cells[a]) {
		return
	}
	cv.cells[a][c] = cell
}

func (cv *canvas) inBounds(a, c int) bool {
	if cv.horiz {
		return c >= 0 && c < len(cv.cells) && a >= 0 && a < len(cv.cells[c])
	}
	return a >= 0 && a < len(cv.cells) && c >= 0 && c < len(cv.cells[a])
}

func (cv *canvas) empty(a, c int) bool {
	if cv.horiz {
		if c < 0 || c >= len(cv.cells) || a < 0 || a >= len(cv.cells[c]) {
			return false
		}
		return cv.cells[c][a].ch == ' '
	}
	if a < 0 || a >= len(cv.cells) || c < 0 || c >= len(cv.cells[a]) {
		return false
	}
	return cv.cells[a][c].ch == ' '
}

// crossExtent is a box's size across the flow: its width, or its height
// when the flow runs horizontally.
func crossExtent(b *box, horiz bool) int {
	if horiz {
		return b.h
	}
	return b.w
}

// crossCenter is where an edge attaches: the middle of the cross extent.
func crossCenter(b *box, horiz bool) int {
	return b.x + crossExtent(b, horiz)/2
}

// rankExtent is how much along-axis space a rank needs: its tallest box, or
// its widest when the flow runs horizontally.
func rankExtent(bs []*box, horiz bool) int {
	e := 1
	for _, b := range bs {
		if horiz {
			e = max(e, b.w)
		} else {
			e = max(e, b.h)
		}
	}
	return e
}

// labelBreaks turns mermaid's line-break tags into spaces: diagrams are
// single-line per node.
var labelBreaks = strings.NewReplacer("<br/>", " ", "<br />", " ", "<br>", " ")

// nodeBox renders one node's glyphs: lines, display width, height.
func nodeBox(n *flowNode) ([]string, int, int) {
	label := labelBreaks.Replace(n.label)
	if lipgloss.Width(label) > maxLabelCols {
		label = truncateCols(label, maxLabelCols)
	}
	switch n.shape {
	case shapeDiamond:
		lw := lipgloss.Width(label)
		w := lw + 4
		pad := (w - 2) / 2 // center the 2-glyph cap over the full width
		return []string{
			strings.Repeat(" ", pad) + "╱╲",
			"╱ " + label + " ╲",
			strings.Repeat(" ", pad) + "╲╱",
		}, w, 3
	case shapeCircle:
		return []string{"((" + label + "))"}, lipgloss.Width(label) + 4, 1
	default: // rect, rounded, and stadium look alike at one row
		open, close := '[', ']'
		if n.shape == shapeRounded || n.shape == shapeStadium {
			open, close = '(', ')'
		}
		return []string{string(open) + label + string(close)}, lipgloss.Width(label) + 2, 1
	}
}

// truncateCols cuts s to at most n display columns, ending with an ellipsis.
func truncateCols(s string, n int) string {
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if w+rw > n-1 {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteString("…")
	return b.String()
}

// segGlyph picks the line glyph. along is rank-space orientation; the final
// orientation is horizontal when along == horiz.
func segGlyph(horiz bool, st edgeStyle, along bool) rune {
	h := along == horiz
	switch st {
	case edgeDotted:
		if h {
			return '┄'
		}
		return '┆'
	case edgeThick:
		if h {
			return '═'
		}
		return '║'
	default:
		if h {
			return '─'
		}
		return '│'
	}
}

func arrowGlyph(horiz, intoHi, flip bool) rune {
	g := '▲'
	switch {
	case horiz && intoHi:
		g = '▶'
	case horiz:
		g = '◀'
	case intoHi:
		g = '▼'
	}
	if flip {
		switch g {
		case '▼':
			g = '▲'
		case '▲':
			g = '▼'
		case '▶':
			g = '◀'
		case '◀':
			g = '▶'
		}
	}
	return g
}

// cornerGlyph picks the bend corner. atStart is the source-side corner;
// rightward is true when the horizontal run moves toward larger columns.
func cornerGlyph(horiz, atStart, rightward bool) rune {
	switch {
	case !horiz && atStart: // coming down, turning
		if rightward {
			return '└'
		}
		return '┐'
	case !horiz: // coming from the side, turning down
		if rightward {
			return '┐'
		}
		return '└'
	case atStart: // coming right, turning up/down
		if rightward {
			return '┐'
		}
		return '┘'
	default:
		if rightward {
			return '└'
		}
		return '┌'
	}
}

// putLabel writes text left-to-right (in final orientation) into the first
// run of empty cells at or after the anchor, so an edge glyph between the
// anchor and open space does not truncate the label.
func putLabel(cv *canvas, a, c int, text string) {
	r := []rune(text)
	for {
		fits := true
		for i := range r {
			p, q := a, c+i
			if cv.horiz {
				p, q = a+i, c
			}
			if !cv.empty(p, q) {
				fits = false
				break
			}
		}
		if fits {
			for i, ch := range r {
				p, q := a, c+i
				if cv.horiz {
					p, q = a+i, c
				}
				cv.put(p, q, cell{ch: ch})
			}
			return
		}
		if cv.horiz {
			a++
		} else {
			c++
		}
		if !cv.inBounds(a, c) {
			return
		}
	}
}

// layoutFlowchartStyled renders the graph with line-drawing chrome muted.
func layoutFlowchartStyled(g *flowGraph, muted lipgloss.Style) ([]string, error) {
	cells, err := buildGrid(g)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(cells))
	for i, row := range cells {
		var b strings.Builder
		j := 0
		for j < len(row) {
			k := j
			for k < len(row) && row[k].chrome == row[j].chrome {
				k++
			}
			buf := make([]rune, 0, k-j)
			for _, c := range row[j:k] {
				buf = append(buf, c.ch)
			}
			s := string(buf)
			if row[j].chrome {
				s = muted.Render(s)
			}
			b.WriteString(s)
			j = k
		}
		out[i] = strings.TrimRight(b.String(), " ")
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out, nil
}
