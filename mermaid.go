package main

// Parser for the mermaid flowchart subset (see
// docs/superpowers/specs/2026-11-06-mermaid-flowchart-design.md). Anything
// outside the subset returns errUnsupported so the preview falls back to a
// plain code block.

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errUnsupported = errors.New("mermaid: unsupported syntax")

type shapeKind int

const (
	shapeRect shapeKind = iota
	shapeRounded
	shapeStadium
	shapeDiamond
	shapeCircle
)

type edgeStyle int

const (
	edgeNormal edgeStyle = iota
	edgeDotted
	edgeThick
)

type dirKind int

const (
	dirTB dirKind = iota
	dirLR
	dirBT
	dirRL
)

type flowNode struct {
	id    string
	label string
	shape shapeKind
}

type flowEdge struct {
	from, to string
	label    string
	style    edgeStyle
	arrow    bool
	bidir    bool // arrowheads at both ends (<-->)
}

type flowGraph struct {
	dir   dirKind
	nodes []*flowNode // first-seen order
	edges []flowEdge
}

func parseFlowchart(src []byte) (*flowGraph, error) {
	lines := strings.Split(string(src), "\n")
	head := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "%%{") {
			return nil, errUnsupported
		}
		if t == "" || strings.HasPrefix(t, "%%") {
			continue
		}
		head = i
		break
	}
	if head < 0 {
		return nil, errUnsupported
	}
	f := strings.Fields(lines[head])
	if len(f) != 2 {
		return nil, errUnsupported
	}
	switch strings.ToLower(f[0]) {
	case "flowchart", "graph":
	default:
		return nil, errUnsupported
	}
	var dir dirKind
	switch strings.ToLower(f[1]) {
	case "td", "tb":
		dir = dirTB
	case "lr":
		dir = dirLR
	case "bt":
		dir = dirBT
	case "rl":
		dir = dirRL
	default:
		return nil, errUnsupported
	}
	p := &parser{g: &flowGraph{dir: dir}, byID: map[string]*flowNode{}}
	for _, l := range lines[head+1:] {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "%%{") {
			return nil, errUnsupported
		}
		for _, stmt := range strings.Split(l, ";") {
			stmt = strings.TrimSpace(stripComment(stmt))
			if stmt == "" {
				continue
			}
			if err := p.statement(stmt); err != nil {
				return nil, err
			}
		}
	}
	return p.g, nil
}

type parser struct {
	g    *flowGraph
	byID map[string]*flowNode
}

// unsupportedKWs are statements the renderer does not support (see the spec:
// classDef/class/style/linkStyle/click); hasKeyword matches a keyword only
// when it is the whole statement or followed by a delimiter, so ids like
// "endpoint" stay parseable. subgraph/end are accepted but flattened: their
// nodes and edges render, the grouping box does not.
var unsupportedKWs = []string{"classDef", "class", "style", "linkStyle", "click"}

func hasKeyword(s, kw string) bool {
	if !strings.HasPrefix(s, kw) {
		return false
	}
	rest := s[len(kw):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}

func (p *parser) statement(s string) error {
	for _, kw := range unsupportedKWs {
		if hasKeyword(s, kw) {
			return errUnsupported
		}
	}
	// Flatten subgraphs: the header and its "end" carry no nodes or edges.
	if hasKeyword(s, "subgraph") || s == "end" {
		return nil
	}
	sc := &scan{s: s}
	from, err := sc.nodeDef(p)
	if err != nil {
		return err
	}
	for {
		sc.spaces()
		if sc.done() {
			return nil
		}
		e, err := sc.edge()
		if err != nil {
			return err
		}
		sc.spaces()
		to, err := sc.nodeDef(p)
		if err != nil {
			return err
		}
		e.from, e.to = from.id, to.id
		p.g.edges = append(p.g.edges, e)
		from = to
	}
}

func (sc *scan) edge() (flowEdge, error) {
	var e flowEdge
	rest := sc.s[sc.pos:]
	switch {
	case strings.HasPrefix(rest, "<-"), strings.HasPrefix(rest, "<="):
		sc.pos++
		inner, err := sc.edge()
		if err != nil {
			return e, err
		}
		inner.bidir = true
		e = inner
	case strings.HasPrefix(rest, "-."):
		sc.pos += 2
		if strings.HasPrefix(sc.s[sc.pos:], "->") {
			sc.pos += 2
			e = flowEdge{style: edgeDotted, arrow: true}
		} else if lbl, arrow, ok := sc.edgeText(".->", ".-"); ok {
			e = flowEdge{style: edgeDotted, arrow: arrow, label: lbl}
		} else {
			return e, errUnsupported
		}
	case strings.HasPrefix(rest, "=="):
		sc.pos += 2
		if sc.peek() == '>' {
			sc.pos++
			e = flowEdge{style: edgeThick, arrow: true}
		} else if lbl, _, ok := sc.edgeText("==>", ""); ok {
			e = flowEdge{style: edgeThick, arrow: true, label: lbl}
		} else {
			return e, errUnsupported
		}
	case strings.HasPrefix(rest, "--"):
		sc.pos += 2
		switch sc.peek() {
		case '>':
			sc.pos++
			e = flowEdge{style: edgeNormal, arrow: true}
		case '-':
			sc.pos++
			e = flowEdge{style: edgeNormal, arrow: false}
		default:
			lbl, arrow, ok := sc.edgeText("-->", "---")
			if !ok {
				return e, errUnsupported
			}
			e = flowEdge{style: edgeNormal, arrow: arrow, label: lbl}
		}
	default:
		return e, errUnsupported
	}
	sc.spaces()
	if sc.peek() == '|' {
		sc.pos++
		lbl, ok := sc.scanTo("|")
		if !ok {
			return e, errUnsupported
		}
		e.label = lbl
	}
	return e, nil
}

// edgeText scans " label " up to the arrow terminator, else the open
// terminator, and consumes through the one it matched.
func (sc *scan) edgeText(arrowTerm, openTerm string) (label string, arrow, ok bool) {
	rest := sc.s[sc.pos:]
	ia := indexUnquoted(rest, arrowTerm)
	io := -1
	if openTerm != "" {
		io = indexUnquoted(rest, openTerm)
	}
	switch {
	case ia >= 0 && (io < 0 || ia <= io):
		sc.pos += ia + len(arrowTerm)
		return strings.TrimSpace(unquote(strings.TrimSpace(rest[:ia]))), true, true
	case io >= 0:
		sc.pos += io + len(openTerm)
		return strings.TrimSpace(unquote(strings.TrimSpace(rest[:io]))), false, true
	}
	return "", false, false
}

func indexUnquoted(s, sub string) int {
	inQ := false
	for i := 0; i+len(sub) <= len(s); i++ {
		switch {
		case s[i] == '"':
			inQ = !inQ
		case !inQ && strings.HasPrefix(s[i:], sub):
			return i
		}
	}
	return -1
}

type scan struct {
	s   string
	pos int
}

func (sc *scan) done() bool { return sc.pos >= len(sc.s) }

func (sc *scan) peek() byte {
	if sc.pos < len(sc.s) {
		return sc.s[sc.pos]
	}
	return 0
}

func (sc *scan) spaces() {
	for sc.pos < len(sc.s) && (sc.s[sc.pos] == ' ' || sc.s[sc.pos] == '\t') {
		sc.pos++
	}
}

func (sc *scan) nodeDef(p *parser) (*flowNode, error) {
	start := sc.pos
	for sc.pos < len(sc.s) {
		r, size := utf8.DecodeRuneInString(sc.s[sc.pos:])
		if !isIDRune(r) {
			break
		}
		sc.pos += size
	}
	if sc.pos == start {
		return nil, errUnsupported
	}
	id := sc.s[start:sc.pos]
	shape, label, ok, err := sc.shape()
	if err != nil {
		return nil, err
	}
	n := p.byID[id]
	if n == nil {
		n = &flowNode{id: id}
		p.byID[id] = n
		p.g.nodes = append(p.g.nodes, n)
	}
	if ok {
		n.shape, n.label = shape, label
	}
	if n.label == "" {
		n.label = id
	}
	return n, nil
}

// isIDRune keeps letters, digits, and underscore in ids. Anything else (an
// arrow, say) ends the id so the statement fails to parse and falls back,
// instead of being swallowed into a node name.
func isIDRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

var shapeDelims = []struct {
	open, close string
	kind        shapeKind
}{
	{"([", "])", shapeStadium},
	{"((", "))", shapeCircle},
	{"[(", ")]", shapeStadium}, // cylinder: drawn as a rounded box
	{"[", "]", shapeRect},
	{"(", ")", shapeRounded},
	{"{", "}", shapeDiamond},
}

func (sc *scan) shape() (shapeKind, string, bool, error) {
	for _, d := range shapeDelims {
		if !strings.HasPrefix(sc.s[sc.pos:], d.open) {
			continue
		}
		sc.pos += len(d.open)
		// Cylinder, parallelogram, subroutine, flag, and hexagon shapes all
		// start with '[' too; reject them rather than misreading a rect.
		if d.open == "[" && sc.pos < len(sc.s) {
			switch sc.s[sc.pos] {
			case '(', '[', '/', '\\', '>', '{':
				return 0, "", false, errUnsupported
			}
		}
		label, ok := sc.scanTo(d.close)
		if !ok {
			return 0, "", false, errUnsupported
		}
		return d.kind, label, true, nil
	}
	return 0, "", false, nil
}

// scanTo consumes up to a closing delimiter outside double quotes.
func (sc *scan) scanTo(close string) (string, bool) {
	rest := sc.s[sc.pos:]
	inQ := false
	for i := 0; i+len(close) <= len(rest); i++ {
		switch {
		case rest[i] == '"':
			inQ = !inQ
		case !inQ && strings.HasPrefix(rest[i:], close):
			sc.pos += i + len(close)
			return unquote(strings.TrimSpace(rest[:i])), true
		}
	}
	return "", false
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// stripComment cuts a `%%` comment that starts outside double quotes.
func stripComment(s string) string {
	inQ := false
	for i := 0; i+1 < len(s); i++ {
		switch s[i] {
		case '"':
			inQ = !inQ
		case '%':
			if !inQ && s[i+1] == '%' {
				return s[:i]
			}
		}
	}
	return s
}
