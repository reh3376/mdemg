# Installing mdemg.nvim

This guide walks you through adding the mdemg.nvim plugin to your Neovim configuration so it loads automatically on startup.

## Prerequisites

| Requirement | Check | Notes |
|-------------|-------|-------|
| Neovim >= 0.10 | `nvim --version` | Required for `vim.system()` and `vim.uv` APIs |
| `curl` on PATH | `which curl` | Used for all HTTP communication with MDEMG |
| Running MDEMG instance | `curl -s http://localhost:9999/readyz` | Start with `mdemg start --auto-migrate` |

## Installation

### lazy.nvim (Recommended)

Add to your `~/.config/nvim/lua/plugins/` directory or wherever your lazy.nvim plugin specs live:

```lua
-- ~/.config/nvim/lua/plugins/mdemg.lua
return {
  "reh3376/mdemg.nvim",
  version = "*",  -- pin to latest stable release
  config = function()
    require("mdemg").setup()
  end,
}
```

To pin to a specific minor version:

```lua
return {
  "reh3376/mdemg.nvim",
  version = "^0.1",  -- pin to 0.1.x releases
  config = function()
    require("mdemg").setup()
  end,
}
```

If you use a single `plugins.lua` file instead of a directory, add the entry to your plugin table:

```lua
-- ~/.config/nvim/lua/plugins.lua
return {
  -- ... your other plugins ...

  {
    "reh3376/mdemg.nvim",
    version = "*",
    config = function()
      require("mdemg").setup()
    end,
  },
}
```

After saving, restart Neovim. lazy.nvim will clone the repo and load the plugin automatically.

### rocks.nvim

> **Note:** This section is only for users who use [rocks.nvim](https://github.com/nvim-neorocks/rocks.nvim)
> as their plugin manager. If you use lazy.nvim or packer, skip this section.

```vim
:Rocks install mdemg.nvim
```

Then add the setup call to your `~/.config/nvim/init.lua`:

```lua
require("mdemg").setup()
```

### packer.nvim

Add to your packer config (usually `~/.config/nvim/lua/plugins.lua`):

```lua
use {
  "reh3376/mdemg.nvim",
  tag = "v0.1.*",
  config = function()
    require("mdemg").setup()
  end,
}
```

Then run `:PackerSync` to install.

### Manual (no plugin manager)

Clone directly into Neovim's pack path:

```bash
git clone https://github.com/reh3376/mdemg.nvim \
  ~/.config/nvim/pack/plugins/start/mdemg.nvim
```

Then add the setup call to your `~/.config/nvim/init.lua`:

```lua
require("mdemg").setup()
```

The `pack/plugins/start/` path tells Neovim to load the plugin automatically on startup. No plugin manager required.

## Verify Installation

After restarting Neovim:

```vim
:checkhealth mdemg
```

You should see checks for Neovim version, curl, `.mdemg/` directory, endpoint reachability, and optional dependencies. All required items should show green checkmarks.

Quick smoke test:

```vim
:MdemgStatus
```

This opens a floating window with your MDEMG instance details (endpoint, space ID, session, stats). If it shows data, the plugin is working.

## Configuration

All options are optional. The defaults work out of the box if MDEMG is running on `localhost:9999`.

### Minimal (all defaults)

```lua
require("mdemg").setup()
```

### Full Configuration Reference

```lua
require("mdemg").setup({
  -- Connection
  endpoint = "http://localhost:9999",
  space_id = nil,                         -- auto-resolved from project directory name
  timeout = 30,                           -- seconds

  -- Keymaps (set any to "" to disable it)
  keymaps = {
    recall   = "<leader>mr",
    store    = "<leader>ms",
    validate = "<leader>mv",
    guide    = "<leader>mg",
    reflect  = "<leader>mf",
    symbols  = "<leader>my",
    status   = "<leader>mi",
  },

  -- Session lifecycle
  session = {
    auto_create = true,                   -- generate session ID on VimEnter
    auto_consolidate = true,              -- consolidate session on VimLeavePre
  },

  -- Automatic behaviors
  auto = {
    ingest_on_save = true,                -- auto-ingest saved files
    ingest_debounce_ms = 2000,            -- debounce rapid saves (ms)
    ingest_extensions = {                 -- which file types to ingest
      "go", "py", "lua", "js", "ts", "tsx", "jsx",
      "rs", "java", "rb", "c", "cpp", "h", "hpp",
    },
    health_poll_interval = 30,            -- readyz check interval (seconds)
    stats_refresh_interval = 120,         -- full stats refresh interval (seconds)
  },

  -- UI
  ui = {
    border = "rounded",                   -- float border style
    width = 0.8,                          -- float width (fraction of screen)
    height = 0.8,                         -- float height (fraction of screen)
    use_telescope = true,                 -- use telescope.nvim if available
    use_notify = true,                    -- use nvim-notify if available
    float_timeout = 10,                   -- auto-dismiss timeout (seconds)
  },

  -- Statusline
  statusline = {
    format = "short",                     -- "short" or "long"
    icons = true,                         -- show emoji icons
  },

  -- Guardrail
  guardrail = {
    auto_validate = false,                -- auto-validate on buffer changes
  },

  -- Logging
  log_level = "warn",                     -- debug, info, warn, error
})
```

### Example: Custom Endpoint and Keymaps

```lua
require("mdemg").setup({
  endpoint = "http://127.0.0.1:8888",
  keymaps = {
    recall   = "<leader>mr",
    store    = "<leader>ms",
    validate = "",                        -- disabled
    guide    = "<leader>mg",
    reflect  = "",                        -- disabled
    symbols  = "<leader>my",
    status   = "<leader>mi",
  },
})
```

### Example: Minimal Auto-Behaviors

```lua
require("mdemg").setup({
  session = {
    auto_create = false,
    auto_consolidate = false,
  },
  auto = {
    ingest_on_save = false,
    health_poll_interval = 0,             -- disables health polling
  },
})
```

## Statusline Integration

Add the MDEMG component to lualine:

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

The component shows connection state, space ID, and node count. Colors indicate health: green (connected), yellow (stale), red (disconnected).

## Optional Dependencies

These plugins are not required but enhance the experience:

| Plugin | What it adds |
|--------|-------------|
| [telescope.nvim](https://github.com/nvim-telescope/telescope.nvim) | Fuzzy finder for memory results, symbol search, and all picker-based commands |
| [nvim-notify](https://github.com/rcarriga/nvim-notify) | Styled notification popups instead of command-line messages |
| [lualine.nvim](https://github.com/nvim-lualine/lualine.nvim) | Statusline component showing MDEMG connection state |
| [nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) | Context-aware auto-tagging (function name, class, language) when storing observations |

Without these, the plugin falls back to `vim.ui.select()` for pickers and `vim.notify()` for notifications.

## How It Works on Startup

When Neovim starts with the plugin installed, the following happens automatically:

1. **`plugin/mdemg.lua`** runs — registers all 24 `:Mdemg*` commands
2. **`setup()`** is called by your config — merges your options with defaults
3. **Session auto-create** — generates a unique session ID (`vim.g.mdemg_session_id`)
4. **Health polling starts** — checks MDEMG reachability every 30 seconds
5. **BufEnter autocmd** — on each buffer enter, walks up to find `.mdemg/` and resolves the endpoint and space ID for that project
6. **BufWritePost autocmd** — on file save, auto-ingests the file into MDEMG (debounced, extension-filtered)
7. **VimLeavePre autocmd** — on exit, consolidates the session to MDEMG

## Multi-Project Support

The plugin auto-detects which MDEMG instance to connect to per buffer. If you have multiple projects each with their own `.mdemg/` directory and running instance, the plugin resolves the correct endpoint for each buffer independently.

Resolution priority:
1. Walk up from buffer path → find `.mdemg.port` file → read port number
2. `.mdemg/config.yaml` `server.port` field
3. `MDEMG_ENDPOINT` environment variable
4. `setup()` `endpoint` option
5. Default: `http://localhost:9999`

## Troubleshooting

**`:checkhealth mdemg` shows "MDEMG instance not reachable":**

```bash
# Start the instance
cd /path/to/your/project
mdemg start --auto-migrate
```

**No space ID resolved:**

```bash
# Initialize MDEMG in your project
cd /path/to/your/project
mdemg init
```

Or set it explicitly:

```lua
require("mdemg").setup({ space_id = "my-project" })
```

**Commands not available:**

Ensure `setup()` is being called. Check `:messages` for any load errors. The plugin requires Neovim 0.10+ — older versions will show an error and skip loading.

**Keymaps conflict with other plugins:**

The default `<leader>m` prefix may conflict with markdown or other plugins. Remap the entire prefix:

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

Or disable individual keymaps:

```lua
require("mdemg").setup({
  keymaps = {
    guide = "",                           -- disable entirely
  },
})
```
