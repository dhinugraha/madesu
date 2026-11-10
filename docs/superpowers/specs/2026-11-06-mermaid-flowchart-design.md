# Mermaid flowchart rendering — design

Date: 2026-11-06
Status: approved
Branch: `feature/mermaid-flowchart-renderer`

## Goal

Render `mermaid` fenced code blocks in madesu's preview pane as ASCII/Unicode
diagrams instead of plain code. Keeps the project promise: single binary, no
new runtime dependencies, no new Go dependencies.

## Decisions

- **What mermaid support means:** draw actual diagrams (boxes, arrows, labels)
  in the terminal — not syntax highlighting, not shelling out to mermaid-cli or
  graphviz.
- **Diagram types in v1:** `graph` / `flowchart` only. Every other mermaid
  diagram type keeps rendering as a plain code block. Later types slot in
  behind the same parse → layout → render interface.
- **Layout engine:** simplified layered (rank → order → orthogonal routing),
  not full Sugiyama. Note-sized diagrams (5–20 nodes) are the target; dense
  graphs may show extra crossings but stay readable.
- **Failure behavior:** silent fallback. Anything unparseable or unsupported
  renders exactly like today (language label + plain lines). No warnings, no
  layout shift.

## Parser (`mermaid.go`, new file)

Types independent of goldmark and lipgloss so parser tests are pure:

```go
type shapeKind int // shapeRect, shapeRounded, shapeStadium, shapeDiamond, shapeCircle
type edgeStyle int // edgeNormal, edgeDotted, edgeThick
type flowNode struct { id, label string; shape shapeKind }
type flowEdge struct { from, to, label string; style edgeStyle; arrow bool }
type flowGraph struct { dir int; nodes []*flowNode; edges []flowEdge }
// dir is one of: dirTB, dirLR, dirBT, dirRL
```

`parseFlowchart(src []byte) (*flowGraph, error)` — hand-written scanner, no
regexp (quoted labels and unicode must not trip it):

- First line: `flowchart <dir>` or `graph <dir>`; direction `td|tb|lr|bt|rl`.
- Statements separated by newlines or `;`; `%%` comments stripped.
- Edge operators: `-->`, `-.->`, `==>`, `---`, with labels in both forms
  (`C -->|Yes| D`, `C -- No --> D`) and chaining (`A --> B --> C`).
- Node shapes: `id[...]` rect, `id(...)` rounded, `id([...])` stadium,
  `id{...}` diamond, `id((...))` circle. Nodes created in first-seen order,
  which drives deterministic layout. Bare `id[label]` statements define nodes.
- Unknown constructs — `subgraph`/`end`, `classDef`, `style`, `linkStyle`,
  `click`, `%%{...}%%` directives, exotic shapes (`id>flag]`, double circles),
  unparseable statements — return the sentinel `errUnsupported`.
- Cycles parse fine; cycle-breaking belongs to the layout stage.

## Layout (`mermaid_layout.go`, new file)

`layoutFlowchart(g *flowGraph) ([]string, error)` returns finished diagram
lines:

1. **Cycle breaking:** DFS; back edges reversed for ranking only — arrows are
   still drawn in the user's direction.
2. **Ranking:** longest-path layering over the acyclic graph. TB/BT → rank =
   row; LR/RL → rank = column. Sources at rank 0.
3. **Ordering:** first-seen order within a rank, then 4 barycenter sweeps
   (down-up-down-up) to reduce crossings. Ties never reorder; identical input
   produces identical output.
4. **Sizing:** single-line labels truncated with `…` beyond ~24 display
   columns, measured with `lipgloss.Width` (display width, not rune count).
   Boxes are 1 row tall except diamonds (3 rows). Gaps sized to fit routed
   edges plus their labels.
5. **Grid assembly:** rune per cell + style tag; muted chrome, default label
   color. Shape glyphs: rect `┌ ┐ └ ┘`, rounded/stadium `╭ ╮ ╰ ╯`, diamond
   `╱ ╲`, circle arc caps.
6. **Edge routing:** straight `│`/`─` when endpoints align; otherwise
   orthogonal routes through gutter columns (`┐ └ ┬ ┴`), one gutter column per
   edge so parallel edges never overlap. Arrowheads `▼ ▶ ◀ ▲`; dotted edges
   use `╌`/`┄`, thick edges `║ ═`. Edge labels sit in the gutter on the straight
   segment, truncated when they do not fit.
7. **Directions:** `BT`/`RL` reuse the TB/LR layout with arrowheads flipped.

## Integration (`mdrender.go`)

- In `codeBlock`, when the fence language is `mermaid` (case-insensitive):
  parse → layout → return styled diagram lines.
- On `errUnsupported` or any parse error: fall back to today's plain code
  block rendering.
- Diagrams render at natural width; the preview viewport does not wrap, so
  wider diagrams clip on the right (same as over-long table cells today).
  Documented, not solved, in v1.
- Editor pane untouched; live re-render uses the existing `refreshPreview`
  path.

## Testing

Matches the repo's behavior-driven style (`go test -race -count=1 ./...`,
`go vet ./...` must pass):

- `mermaid_test.go`: parser table tests — first-line variants, bad direction
  error, all five shapes, both label forms, chaining, `;` separators, `%%`
  comments, quoted labels, cycles accepted, and every `errUnsupported` case.
- `mermaid_layout_test.go`: golden grid tests — 3-node chain TB, LR transpose,
  diamond branch with `Yes`/`No` labels, dotted/thick glyphs, BT arrow flip,
  determinism (identical input → byte-identical output).
- `mdrender_test.go` additions: a mermaid fence produces box-drawing output;
  a fence containing `subgraph` falls back to plain code-block rendering.

## Documentation (README.md)

Mermaid moves from "Not rendered yet" to the rendered list, stating the
supported subset (flowchart/graph, TB/LR/BT/RL, five shapes, four edge styles,
labels) and limitations (subgraphs and styling fall back to a plain block,
dense graphs may cross edges, wide diagrams clip).

## Out of scope

- Sequence, state, ER, and other mermaid diagram types.
- `subgraph`, `classDef`, `style`, `linkStyle`, `click`, theme directives.
- Width scaling or horizontal scrolling for over-wide diagrams.
- Any dependency beyond the existing go.mod set.

## Estimated size

~200 lines parser + ~300 lines layout + ~350 lines tests — same order as
`mdrender.go` itself.
