# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Configurable keyboard actions on the selected repo.
  - New `actions:` block in `config.yml`. Each entry has `key`, `label`, and exactly one of `run` (shell command) or `clipboard` (text to copy). `{path}` is substituted with the selected repo's path.
  - `Enter` runs the action with key `enter` (default: opens the configured editor — backward compatible).
  - `Alt+<letter>` shortcuts dispatch their action directly from the table.
  - `?` opens an in-app menu listing every action with its shortcut. From the menu: arrow keys to navigate, `Enter` to run, `e` edit, `a` add, `d` delete (with one-key confirm).
  - Edits write back to `config.yml` via `yaml.Node` so header and inline comments are preserved.
  - Built-in defaults when no actions are configured: `enter` → editor, `alt+c` → copy path, `alt+g` → `gitui -d {path}`.
  - WSL-aware clipboard: writes go through `iconv -t utf-16le | clip.exe` on WSL so unicode survives the Win32 clipboard format; everywhere else, the atotto/clipboard backend handles it.
- Opt-in support for linked git worktrees ([#24](https://github.com/Bharath-code/git-scope/issues/24)).
  - Config: `includeWorktrees: false` (default) in `~/.config/git-scope/config.yml`.
  - CLI flag: `--worktrees` for one-shot enable.
  - TUI: press `W` to toggle live; the toggle controls both visibility and totals.
  - Scan/JSON: each repo carries an `is_worktree` field; worktrees show a `⎇` marker in the TUI table.
  - Submodules are excluded — only `gitdir:` pointers under `.git/worktrees/` are recognised.

