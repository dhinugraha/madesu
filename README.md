# madesu

A small terminal app for editing Markdown files. A file explorer sits on the left, a text editor on the right, and you can flip the right pane into a rendered Markdown preview.

Markdown simple utility.

## Features

- File explorer with directory navigation (folders first; shows all files including dotfiles; symlinks marked with `@`)
- Plain text editor for `.md` and other text files
- Rendered Markdown preview, scrollable with keyboard and mouse
- Resizable panes: drag the divider with the mouse, or use `ctrl+shift+←` / `ctrl+shift+→`
- Mouse wheel / trackpad scrolling in the explorer, editor, and preview
- Single binary, no runtime dependencies

## Requirements

- Go 1.26 or newer (see `go.mod`)
- A terminal with 256-color support and xterm mouse reporting. Terminal.app and iTerm2 on macOS are the setups I have used; other modern terminals should work too.

## Install

From source:

```sh
git clone git@github.com:dhinugraha/madesu.git
cd madesu
go build -o madesu .
```

Put the binary somewhere on your `PATH`, for example:

```sh
mv madesu ~/.local/bin/
```

Or install straight into `$(go env GOPATH)/bin`:

```sh
go install .
```

## Usage

```sh
madesu            # open the current directory
madesu ~/Notes    # open a specific directory
```

The first argument is always treated as a directory path. There are no other flags yet.

## Keyboard

### Explorer (left pane)

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move selection |
| `enter` | Open a file, or enter a directory (`..` goes up) |
| mouse wheel | Scroll the list without moving the selection, until the selection leaves the view |
| `tab` | Switch to the right pane |
| `ctrl+c` | Quit |

In the explorer, names are marked: a trailing `/` is a folder, and a trailing `@` is a symlink. Symlinks to folders can be entered like normal folders; broken symlinks are also marked `@` and can't be entered.

### Editor (right pane)

The editor has vim-style modes. It starts in INSERT mode; `esc` returns to NORMAL mode.

**INSERT mode**

| Key | Action |
| --- | --- |
| `ctrl+s` | Save |
| `esc` | Switch to NORMAL mode |
| `ctrl+p` | Switch to the preview |
| `tab` | Switch to the explorer |
| `pgup` / `pgdown` | Move the cursor one screen up or down |
| `home` / `end` | Start / end of the current line |
| `ctrl+home` / `ctrl+end` | Start / end of the document |

**NORMAL mode**

| Key | Action |
| --- | --- |
| `:w` then `enter` | Save |
| `dd` | Delete the line under the cursor |
| `u` | Undo |
| `i` / `a` | Back to INSERT (before / after the cursor) |
| `h` `j` `k` `l` / arrows | Move the cursor |
| `esc` | Cancel `d`, or stay in NORMAL |
| `ctrl+s` | Save (still available) |
| `ctrl+p` | Switch to the preview |
| `tab` | Switch to the explorer |

One insert session (everything typed between entering INSERT and pressing `esc`) is one undo step, as is each `dd`. Undo history is per open file and is cleared when another file is opened.

Editing also follows the standard `bubbles/textarea` keys while in INSERT mode, such as `ctrl+a` / `ctrl+e` for line start and end, and `ctrl+w` to delete a word.

### Preview (right pane)

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Scroll one line |
| `pgup` / `pgdown` | Scroll one screen |
| `home` / `end` | Top / bottom of the document |
| `ctrl+p` | Return to the editor |
| `tab` | Switch to the explorer |

### Opening a file

Opening a file puts the cursor on its first line, and the editor gets focus.

## Resizing panes

| Key | Action |
| --- | --- |
| `ctrl+shift+←` / `ctrl+shift+→` | Move the divider 2 columns left or right (works from either pane) |

The mouse can also drag the divider; see below.

### macOS Terminal.app setup

Terminal.app does not send `ctrl+shift+←` / `ctrl+shift+→` by default, so the shortcut does nothing until you map them. Ghostty works without setup. To map them in Terminal.app:

1. Open Terminal → Settings (`⌘,`) → Profiles → select your profile → **Keyboard**.
2. Click `+`, press `ctrl+shift+←` in the key field.
3. Set the action to **Send String to Shell** and enter `\033[1;6D`.
4. Repeat for `ctrl+shift+→` with the string `\033[1;6C`.

## Mouse

- **Drag the divider** between the explorer and the right pane to resize them. Press on the border or the gap (±1 column), drag, and release. The explorer keeps at least 16 columns and the right pane at least 30.
- **Wheel / trackpad** scrolls the pane under the pointer, 3 lines per notch:
  - Explorer: the list scrolls and the selection stays put until it leaves the view.
  - Editor: the cursor moves with the view so the text under the pointer keeps scrolling.
  - Preview: scrolls the rendered document.

Because the app captures mouse events, your terminal's native text selection is disabled while it runs. To select text anyway, hold the terminal's bypass modifier while dragging: `option` in iTerm2, and usually `fn` in Terminal.app.

## Markdown support

The preview renders CommonMark plus these GitHub-flavored extensions:

- Headings (levels 1–6, with the `#` markers kept visible)
- Emphasis, bold, bold-italic, strikethrough, inline code
- Links (text followed by the URL) and autolinks; images show as `[img: alt] url`
- Block quotes, including nested quotes
- Bullet and ordered lists, nested up to any depth, tight and loose
- Task lists (`☑` / `☐`)
- Fenced and indented code blocks, with the language label
- Mermaid flowcharts (`graph` / `flowchart` with `TB`/`TD`, `LR`, `BT`, `RL` directions), drawn with box-drawing characters: rectangle, rounded, stadium, cylinder, circle, and diamond nodes; normal, dotted, thick, and bidirectional edges; edge labels (`-->|text|` and `-- text -->` forms). `subgraph` blocks are flattened: their nodes and edges render, the grouping box is not drawn. `<br/>` inside labels becomes a space.
- Tables with left, center, and right alignment; long cells wrap inside their column
- Footnotes (`[^1]`), rendered as `[1]` with the notes listed at the end
- Definition lists
- Backslash escapes and HTML entities
- Horizontal rules

Not rendered yet:

- **Math** (`$...$`, `$$...$$`) is shown as plain text.
- **Raw HTML** is shown as plain text, since a terminal cannot render it.
- **Syntax highlighting** in code blocks is not implemented. Code blocks only get a background color.
- **Mermaid** diagrams other than flowcharts (sequence, state, ER, gantt, ...) are shown as plain code blocks. Flowcharts using unsupported syntax (`classDef`/`style`, theme directives, self-loops, parallel or opposite-direction edges, exotic shapes such as parallelograms and flags) also fall back to plain code blocks.
- **Other diagram languages** are shown as plain code blocks.
- **Images** are not displayed; the alt text and URL are shown.

## Configuration

| Variable | Values | Default | Purpose |
| --- | --- | --- | --- |
| `MADESU_THEME` | `dark`, `light` | `dark` | Color palette for the UI and the Markdown preview. Use `light` for a light terminal background. |

Terminal background detection is not automatic. The default is dark, so set `MADESU_THEME=light` if your background is light.

## Development

```sh
go vet ./...
go test -race -count=1 ./...
```

The code is a single Go package:

| File | Contents |
| --- | --- |
| `main.go` | Bubble Tea model, key and mouse handling, layout, theme |
| `explorer.go` | Directory listing (folders first, all entries including dotfiles and symlinks) |
| `mdrender.go` | Markdown to ANSI renderer on top of goldmark |
| `mermaid.go` | Mermaid flowchart parser (supported subset) |
| `mermaid_layout.go` | Layered layout, box/edge drawing for flowcharts |
| `*_test.go` | Tests for layout geometry, keys, mouse drag and wheel, and the renderer |
| `testdata/` | Sample Markdown used by the layout tests |

Dependencies: [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), [Lip Gloss](https://github.com/charmbracelet/lipgloss), and [goldmark](https://github.com/yuin/goldmark).

## Known limitations

- The editor's wheel scrolling moves the cursor along with the view, because `bubbles/textarea` does not expose its scroll offset.
- The whole file is loaded into memory. Very large files will be slow to render in the preview (roughly 100 ms per 600 KB).
- Undo is available in NORMAL mode (`u`, one step per insert session or `dd`); redo is not implemented.
- Mermaid flowcharts render at their natural size; a diagram wider than the preview pane is clipped on the right. Dense graphs may draw crossing edges.
- There is no prompt for unsaved changes when opening another file.

## License

No license has been chosen yet.
