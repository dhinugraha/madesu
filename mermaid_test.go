package main

import (
	"errors"
	"testing"
)

func mustParse(t *testing.T, src string) *flowGraph {
	t.Helper()
	g, err := parseFlowchart([]byte(src))
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return g
}

func TestParseHeader(t *testing.T) {
	cases := []struct {
		src  string
		want dirKind
	}{
		{"flowchart TD", dirTB},
		{"flowchart tb", dirTB},
		{"graph LR", dirLR},
		{"flowchart BT", dirBT},
		{"graph rl", dirRL},
	}
	for _, tc := range cases {
		g := mustParse(t, tc.src)
		if g.dir != tc.want {
			t.Fatalf("%q: dir %v, want %v", tc.src, g.dir, tc.want)
		}
	}
}

func TestParseHeaderRejects(t *testing.T) {
	for _, src := range []string{
		"",
		"\n\n",
		"sequenceDiagram\nA->>B: hi",
		"flowchart\nA --> B",
		"flowchart XX\nA --> B",
		"graph",
	} {
		if _, err := parseFlowchart([]byte(src)); !errors.Is(err, errUnsupported) {
			t.Fatalf("%q: err %v, want errUnsupported", src, err)
		}
	}
}

func TestParseNodeShapes(t *testing.T) {
	g := mustParse(t, "flowchart TD\nA[Box]\nB(Round)\nC([Stad])\nD{Diamond}\nE((Dot))\nF")
	want := []struct {
		id, label string
		shape     shapeKind
	}{
		{"A", "Box", shapeRect},
		{"B", "Round", shapeRounded},
		{"C", "Stad", shapeStadium},
		{"D", "Diamond", shapeDiamond},
		{"E", "Dot", shapeCircle},
		{"F", "F", shapeRect}, // bare id: label defaults to id
	}
	if len(g.nodes) != len(want) {
		t.Fatalf("nodes %d, want %d", len(g.nodes), len(want))
	}
	for i, w := range want {
		n := g.nodes[i]
		if n.id != w.id || n.label != w.label || n.shape != w.shape {
			t.Fatalf("node %d = %+v, want %+v", i, n, w)
		}
	}
	if len(g.edges) != 0 {
		t.Fatalf("edges %d, want 0", len(g.edges))
	}
}

func TestParseQuotedLabel(t *testing.T) {
	g := mustParse(t, `flowchart TD
A["Label with ) parens"]`)
	if g.nodes[0].label != "Label with ) parens" {
		t.Fatalf("label %q", g.nodes[0].label)
	}
}

func TestParseNodeReferenceKeepsDefinition(t *testing.T) {
	// First definition wins; a later bare reference keeps label and shape.
	g := mustParse(t, "flowchart TD\nA[Box]\nA")
	if g.nodes[0].label != "Box" || g.nodes[0].shape != shapeRect {
		t.Fatalf("node = %+v", g.nodes[0])
	}
	if len(g.nodes) != 1 {
		t.Fatalf("duplicate node created: %d", len(g.nodes))
	}
}

func TestParseCommentsAndSemis(t *testing.T) {
	g := mustParse(t, "flowchart TD\n%% a comment\nA[Box]; B(Round) %% trailing\n%% another")
	if len(g.nodes) != 2 {
		t.Fatalf("nodes %d, want 2", len(g.nodes))
	}
}

func TestParseEdges(t *testing.T) {
	cases := []struct {
		name  string
		stmt  string
		style edgeStyle
		arrow bool
		label string
	}{
		{"arrow", "A --> B", edgeNormal, true, ""},
		{"open", "A --- B", edgeNormal, false, ""},
		{"dotted", "A -.-> B", edgeDotted, true, ""},
		{"thick", "A ==> B", edgeThick, true, ""},
		{"pipe", "A -->|Yes| B", edgeNormal, true, "Yes"},
		{"text arrow", "A -- No --> B", edgeNormal, true, "No"},
		{"text open", "A -- maybe --- B", edgeNormal, false, "maybe"},
		{"text dotted", "A -. wait .-> B", edgeDotted, true, "wait"},
		{"text thick", "A == sure ==> B", edgeThick, true, "sure"},
	}
	for _, tc := range cases {
		g := mustParse(t, "flowchart TD\n"+tc.stmt)
		if len(g.edges) != 1 {
			t.Fatalf("%s: edges %d, want 1", tc.name, len(g.edges))
		}
		e := g.edges[0]
		if e.from != "A" || e.to != "B" || e.style != tc.style || e.arrow != tc.arrow || e.label != tc.label {
			t.Fatalf("%s: edge %+v", tc.name, e)
		}
	}
}

func TestParseChainedEdges(t *testing.T) {
	g := mustParse(t, "flowchart TD\nA --> B --> C")
	if len(g.edges) != 2 {
		t.Fatalf("edges %d, want 2", len(g.edges))
	}
	if g.edges[0].from != "A" || g.edges[0].to != "B" || g.edges[1].from != "B" || g.edges[1].to != "C" {
		t.Fatalf("edges %+v", g.edges)
	}
}

func TestParseEdgesRejectJunk(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\nA --> B trailing",
		"flowchart TD\nA -.- B", // bare dotted open not in subset
		"flowchart TD\nA === B", // bare thick open not in subset
		"flowchart TD\nA --o B", // circle arrowheads not in subset
		"flowchart TD\nA -->",   // dangling edge
	} {
		if _, err := parseFlowchart([]byte(src)); !errors.Is(err, errUnsupported) {
			t.Fatalf("%q: err %v, want errUnsupported", src, err)
		}
	}
}

func TestParseUnsupportedConstructs(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\nclassDef green fill:#9f6",
		"flowchart TD\nclass A green",
		"flowchart TD\nstyle A fill:#f9f",
		"flowchart TD\nlinkStyle 0 stroke:#ff3",
		"flowchart TD\nclick A call someFn()",
		"flowchart TD\nA --> B\n%%{init: {'theme':'dark'}}%%",
		"flowchart TD\nA>Flag] --> B",
	} {
		if _, err := parseFlowchart([]byte(src)); !errors.Is(err, errUnsupported) {
			t.Fatalf("%q: err %v, want errUnsupported", src, err)
		}
	}
}

func TestParseDirectiveFirstLine(t *testing.T) {
	if _, err := parseFlowchart([]byte("%%{init: {'theme':'dark'}}%%\nflowchart TD\nA --> B")); !errors.Is(err, errUnsupported) {
		t.Fatalf("err %v, want errUnsupported", err)
	}
}

func TestParseRejectsExoticBracketShapes(t *testing.T) {
	for _, src := range []string{
		"flowchart TD\nA[/parallelogram/]",
		"flowchart TD\nA[[sub]]",
		"flowchart TD\nA>flag]",
	} {
		if _, err := parseFlowchart([]byte(src)); !errors.Is(err, errUnsupported) {
			t.Fatalf("%q: err %v, want errUnsupported", src, err)
		}
	}
}

func TestParseUnicodeArrowIsNotAnID(t *testing.T) {
	if _, err := parseFlowchart([]byte("flowchart TD\nA→B")); !errors.Is(err, errUnsupported) {
		t.Fatalf("A→B: err %v, want errUnsupported", err)
	}
}

func TestParseUnicodeLetterID(t *testing.T) {
	g := mustParse(t, "flowchart TD\n节点[Node]")
	if len(g.nodes) != 1 || g.nodes[0].id != "节点" || g.nodes[0].label != "Node" {
		t.Fatalf("nodes %+v", g.nodes)
	}
}

func TestParseSubgraphFlattened(t *testing.T) {
	g := mustParse(t, "flowchart TD\nsubgraph one[\"Group\"]\nA --> B\nend\nsubgraph docker-compose\nC --> D\nend")
	if len(g.nodes) != 4 || len(g.edges) != 2 {
		t.Fatalf("nodes %d edges %d, want 4 and 2", len(g.nodes), len(g.edges))
	}
}

func TestParseCylinderShape(t *testing.T) {
	g := mustParse(t, "flowchart TD\nDB[(\"Cloud SQL\")]")
	if len(g.nodes) != 1 || g.nodes[0].label != "Cloud SQL" {
		t.Fatalf("nodes %+v", g.nodes)
	}
	if g.nodes[0].shape == shapeRect {
		t.Fatalf("cylinder rendered as rect")
	}
}

func TestParseBidirectionalEdges(t *testing.T) {
	cases := []struct {
		stmt  string
		style edgeStyle
	}{
		{"A <--> B", edgeNormal},
		{"A <-.-> B", edgeDotted},
		{"A <==> B", edgeThick},
	}
	for _, tc := range cases {
		g := mustParse(t, "flowchart TD\n"+tc.stmt)
		if len(g.edges) != 1 {
			t.Fatalf("%q: edges %d", tc.stmt, len(g.edges))
		}
		e := g.edges[0]
		if !e.bidir || !e.arrow || e.style != tc.style {
			t.Fatalf("%q: edge %+v", tc.stmt, e)
		}
	}
}
