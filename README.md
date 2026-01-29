# arbor 🌳

A fast, beautiful Git commit tree for your terminal. **arbor** renders your repository history as a color‑coded branching graph with a responsive TUI, a detail sidebar, and a curated forest palette that adapts to light and dark terminals.

<p align="center">🌲 smooth scrolling • 🔎 instant search • 🧭 branch‑aware graph • 🧩 MVU architecture</p>

---

## ✨ Highlights

- **Branching tree view** with ANSI color mapping per branch line
- **Detail sidebar** with full commit message, date, and changed files
- **Lazy loading** for huge repos (only visible rows + buffer)
- **Adaptive palette** that stays soft and readable in light or dark terminals
- **Keyboard‑first** navigation with familiar Git‑like ergonomics

---

## 🧭 Quick Start

```bash
go run .
```

Open a repo and explore:

```bash
arbor
arbor --all
arbor --limit 100
```

---

## 🧰 Usage

```
arbor [flags]

Flags:
  --all           Include all local and remote branches
  --limit int     Limit the number of commits to parse (0 = no limit)
```

---

## ⌨️ Keybindings

| Key | Action |
| --- | --- |
| `↑/↓` or `k/j` | Move selection |
| `Enter` | Toggle changed‑files view |
| `/` | Search commit messages/authors |
| `Tab` | Toggle sidebar |
| `q` | Quit |

---

## 🏗️ Build & Run

**Requirements**
- Go **1.25.6+**

**Build locally**
```bash
go build -o arbor .
```

**Run**
```bash
./arbor
```

**Install**
```bash
go install .
```

---

## 🧠 How it Works

- **Git DAG traversal** with `go-git` (no shelling out)
- **MVU architecture** (Bubble Tea) for responsive, predictable updates
- **Lip Gloss** styling for a cohesive, tree‑inspired aesthetic

---

## 🧪 Performance Notes

- Lazy loading keeps the UI responsive even for large histories.
- Scrolling loads only what you see plus a small buffer.

---

## 🤝 Contributing

PRs and issues are welcome! If you add features, keep the UI fast and the palette consistent.

---

## 📄 License

MIT — see `LICENSE`.

---

## 🙌 Acknowledgements

Built with:
- **Cobra** (CLI)
- **Bubble Tea** (TUI)
- **Lip Gloss** (styling)
- **go-git** (Git access)

