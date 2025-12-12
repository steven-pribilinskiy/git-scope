# git-scope

A **fast TUI dashboard** to view the git status of **all your repositories** in one place — no more `cd` → `git status` loops.

![demo](docs/git-scope-demo-1.webp)

<p align="left">
  <a href="https://github.com/Bharath-code/git-scope">
    <img src="https://img.shields.io/github/stars/Bharath-code/git-scope?style=flat-square" />
  </a>
  <a href="https://github.com/Bharath-code/git-scope/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/Bharath-code/git-scope?style=flat-square" />
  </a>
  <a href="https://goreportcard.com/report/github.com/Bharath-code/git-scope">
    <img src="https://goreportcard.com/badge/github.com/Bharath-code/git-scope?style=flat-square" />
  </a>
  <a href="https://pkg.go.dev/github.com/Bharath-code/git-scope">
    <img src="https://pkg.go.dev/badge/github.com/Bharath-code/git-scope.svg" />
  </a>
</p>

---

## 🚀 Overview

`git-scope` helps you manage *many* git repositories from a single terminal UI.

It auto-discovers repos, shows which ones are dirty/ahead/behind, and lets you jump into your editor instantly — ideal for multi-repo workflows, microservices, dotfiles, OSS contributions, and experimentation folders.

🌐 **Landing Page:** https://bharath-code.github.io/git-scope/

---

## ✨ Features

- 🔍 **Fuzzy Search** — find any repo by name, path, or branch (`/`)
- 🛡️ **Dirty Filter** — show only repos with uncommitted changes (`f`)
- ⚡ **Fast Startup** — JSON caching → ~10ms launch time
- 📊 **Dashboard** — branch, staged/unstaged counts, last commit time
- ⌨️ **Keyboard-Driven** — Vim navigation (`j/k`), sorting (`s`, `1–4`)
- 🚀 **Editor Jump** — open in VSCode, nvim, vim, helix (`Enter`)
- 🌿 **Contribution Graph** — GitHub-style local heatmap (`g`)
- 💾 **Disk Usage View** — `.git` + `node_modules` sizes (`d`)
- ⏰ **Timeline View** — see recent repo activity (`t`)
- 🔄 **Rescan Anytime** (`r`)

---

## 💡 Why I Built This

I work across many small repositories — experiments, configs, microservices, side projects — and I kept forgetting which repos had uncommitted changes.

Every morning started like this:

```bash
cd repo-1 && git status
cd repo-2 && git status
cd repo-3 && git status
# ...repeat for 20+ repos
```

It was slow, repetitive, and easy to miss dirty repos.

I wanted a **single screen** that showed:

- which repos were dirty  
- which were ahead/behind  
- which had recent changes  
- which needed attention  

No existing tool solved this well, especially for *many* repos.  
So I built `git-scope` to reduce friction and keep everything visible at a glance.

---

## 🆚 Comparison: git-scope vs lazygit

| Feature | git-scope | lazygit |
|---------|-----------|---------|
| **Scope** | Many repos at once | One repo at a time |
| **Purpose** | Workspace overview | Deep repo interaction |
| **Dirty status across repos** | ✔ Yes | ❌ No |
| **Fuzzy repo search** | ✔ Yes | ❌ No |
| **Jump to repo/editor** | ✔ Yes | ❌ No |
| **Commit graph / diffs** | ❌ No | ✔ Yes |
| **Disk usage** | ✔ Yes | ❌ No |
| **Activity timeline** | ✔ Yes | ❌ No |
| **Ideal for** | Multi-repo devs, microservices, config folders | Single-repo workflows |

**Summary:**  
`git-scope` = overview of all repos  
`lazygit` = powerful UI for one repo  
Most developers use both.

---

## 📦 Installation

### **Homebrew (macOS/Linux)**

```sh
brew tap Bharath-code/tap
brew install git-scope
```

**Upgrade:**

```sh
brew upgrade git-scope
```

### **From Source**

```sh
go install github.com/Bharath-code/git-scope/cmd/git-scope@latest
```

Upgrade by running the install command again.

---

## 🖥️ Usage

```sh
git-scope
```

---

## ⚙️ Configuration

Edit `~/.config/git-scope/config.yml`:

```yaml
roots:
  - ~/code
  - ~/work

ignore:
  - node_modules
  - .venv

editor: code   # or nvim, vim, helix
```

---

## ⌨️ Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `/` | Search repos |
| `f` | Filter (All / Dirty / Clean) |
| `s` | Cycle sort mode |
| `1–4` | Sort by Dirty / Name / Branch / Recent |
| `Enter` | Open repo in editor |
| `c` | Clear search & filters |
| `r` | Rescan directories |
| `g` | Toggle contribution graph |
| `d` | Toggle disk usage view |
| `t` | Toggle timeline |
| `Esc` | Close panel |
| `q` | Quit |

---

## 🗺️ Roadmap

- [ ] Background file watcher
- [ ] Quick actions (pull/push)
- [ ] Repo grouping (service/team/workspace)
- [ ] Team dashboards

---

## 🔎 Related Keywords (SEO)

git workspace manager, git dashboard, git repository monitor, git repo viewer, multi-repo git tool, git status all repos, git TUI, terminal UI git manager, TUI dashboard for git, Bubble Tea TUI example, Go TUI application, Go CLI tool, git productivity tools, developer workflow tools, microservices repo management, local git analytics, contribution graph terminal, disk usage analyzer TUI, git repo activity timeline, fast TUI for git, Go project for beginners, open source Go tools, devops git utilities, multi-folder git scanner, git monorepo alternative

---

## 📄 License

MIT

---

If you find this useful, a ⭐ star means a lot!