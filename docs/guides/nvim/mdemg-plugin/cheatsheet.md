# mdemg.nvim Cheatsheet

## Commands

### Tier 1 — Core (Implemented)

| Command | Args | Description |
|---------|------|-------------|
| `:MdemgRecall [query]` | Optional query text | Search the memory graph. No args = prompt. Visual mode = use selection. |
| `:MdemgStore` | — | Store observation. Visual mode = store selection. Normal = multiline input float. |
| `:MdemgValidate` | — | Validate current buffer changes against learned guardrail constraints. |
| `:MdemgGuide` | — | Get Jiminy guidance for code around cursor (±50 lines). |
| `:MdemgReflect [topic]` | Optional topic | Deep graph exploration. Visual mode = use selection as topic. |
| `:MdemgSymbols [query]` | Optional symbol name | Search code symbols. Select result to jump to file:line. |
| `:MdemgStatus` | — | Show instance status: endpoint, space, session, stats, layers, learning. |

### Tier 2 — Operational (Implemented)

| Command | Subcommands | Description |
|---------|-------------|-------------|
| `:MdemgIngest <sub>` | `trigger`, `status`, `cancel`, `jobs`, `files` | Ingestion management. `trigger` opens SSE progress. |
| `:MdemgConversation <sub>` | `observe`, `correct`, `recall`, `resume`, `consolidate`, `graduate`, `volatile-stats`, `session-health` | Conversation memory lifecycle. |
| `:MdemgConstraints <sub>` | `list`, `stats`, `effectiveness`, `conflicts`, `detect-conflicts` | Constraint management and conflict detection. |
| `:MdemgLearning <sub>` | `stats`, `freeze`, `unfreeze`, `freeze-status`, `prune`, `distribution`, `frontiers` | Learning system controls. `prune --execute` for real prune. |
| `:MdemgRSIC <sub>` | `cycle`, `assess`, `report`, `history`, `calibration`, `health`, `signals`, `rollback`, `reset` | RSIC self-improvement. `cycle --dry-run` for dry run. |
| `:MdemgBackup <sub>` | `trigger`, `list`, `status`, `manifest`, `restore`, `delete` | Backup and restore operations. |
| `:MdemgScraper <sub>` | `scrape`, `list`, `status`, `cancel`, `review` | Web scraper job management. |
| `:MdemgNeural <sub>` | `status` | Neural sidecar status. |
| `:MdemgGaps <sub>` | `list`, `detail`, `interviews`, `interview-detail`, `feedback` | Capability gap analysis and interviews. |
| `:MdemgSkills <sub>` | `list`, `recall`, `register` | Skill registry. Select from list to recall. |
| `:MdemgHash <sub>` | `register`, `files`, `verify`, `verify-all`, `update`, `revert`, `scan`, `lookup` | Hash verification for file integrity. |

All Tier 2 commands accept subcommands as arguments (e.g., `:MdemgRSIC cycle --dry-run`).
Running without a subcommand opens a picker.

### Tier 3 — Admin (Implemented)

| Command | Subcommands | Description |
|---------|-------------|-------------|
| `:MdemgAdmin <sub>` | `spaces`, `space-detail`, `prune`, `export-preview`, `export`, `import`, `meta-learn` | Space management, export/import, meta-learning. |
| `:MdemgLinear <sub>` | `issues`, `issue-detail`, `projects`, `project-detail`, `comments`, `create-comment` | Linear issue/project integration. |
| `:MdemgPlugins <sub>` | `list`, `install`, `validate`, `modules`, `ape-status`, `ape-trigger`, `detail`, `module-detail` | Plugin/module management and APE control. |
| `:MdemgWatcher <sub>` | `start`, `status`, `stop` | File watcher lifecycle. |
| `:MdemgWebhooks <sub>` | `trigger` | Webhook trigger by source. |
| `:MdemgHealth` | — | Aggregated dashboard: readyz, stats, RSIC, learning, neural. |

---

## Keymaps

### Normal Mode (default `<leader>M` prefix)

| Key | Command | Action |
|-----|---------|--------|
| `<leader>Mr` | `:MdemgRecall` | Recall memories (opens prompt) |
| `<leader>Ms` | `:MdemgStore` | Store observation (multiline input) |
| `<leader>Mv` | `:MdemgValidate` | Validate buffer changes |
| `<leader>Mg` | `:MdemgGuide` | Get Jiminy guidance |
| `<leader>Mf` | `:MdemgReflect` | Reflect on topic (opens prompt) |
| `<leader>My` | `:MdemgSymbols` | Search symbols |
| `<leader>Mi` | `:MdemgStatus` | Show status float |

> **Note:** The default keymaps in `mdemg.nvim` use `<leader>m`, but `<leader>M` is
> recommended to avoid conflicts with markdown plugins. See [Remap Keymaps](#remap-keymaps).

### Visual Mode

| Key | Action |
|-----|--------|
| `<leader>Mr` | Recall using selected text as query |
| `<leader>Ms` | Store selected text as observation |

### Float Window Keymaps

| Key | Action |
|-----|--------|
| `q` | Close float |
| `<Esc>` | Close float |
| `y` | Yank current line to clipboard |
| `<CR>` | Select / confirm |
| `?` | Show keymap help |

### Guide Float (`MdemgGuide`) Keymaps

| Key | Action |
|-----|--------|
| `+` | Mark guidance as helpful |
| `-` | Mark guidance as unhelpful |

### Store Float (`MdemgStore`) Keymaps

| Key | Action |
|-----|--------|
| `<C-s>` | Save and submit observation |
| `q` | Cancel |

---

## Disable a Keymap

Set any keymap to `""` in setup:

```lua
require("mdemg").setup({
  keymaps = {
    guide = "",    -- disables <leader>mg
    validate = "", -- disables <leader>mv
  },
})
```

## Remap Keymaps

If `<leader>m` conflicts with another plugin (e.g., markdown preview), remap the entire prefix:

```lua
require("mdemg").setup({
  keymaps = {
    recall   = "<leader>Mr",
    store    = "<leader>Ms",
    validate = "<leader>Mv",
    guide    = "<leader>Mg",
    reflect  = "<leader>Mf",
    symbols  = "<leader>My",
    status   = "<leader>Mi",
  },
})
```

---

## Health Check

```vim
:checkhealth mdemg
```

Checks: Neovim ≥ 0.10, curl, `.mdemg/` dir, `.mdemg.port`, endpoint reachability, space ID, optional deps.

---

## Statusline (lualine)

```lua
require("lualine").setup({
  sections = {
    lualine_x = {
      {
        require("mdemg.ui.statusline").component,
        color = require("mdemg.ui.statusline").color,
      },
    },
  },
})
```

Format: `🧠 space-name 15234` (short) or `🧠 space-name 15234n 87%` (long)

Colors: green = connected, red = disconnected, yellow = stale.
