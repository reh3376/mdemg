# Clawdbot Codebase Architecture Documentation

## 1. Top-Level Directory Structure

| Directory | Purpose |
|-----------|---------|
| `/src` (69 dirs) | Main TypeScript source code (2,480 .ts files, 405K LOC) |
| `/extensions` (29) | Extension/plugin modules (channel plugins, auth, memory) |
| `/skills` (52) | Skill modules (tools, integrations, utilities) |
| `/apps` | iOS, Android, macOS native apps |
| `/ui` | Web UI components |
| `/docs` | Documentation (Mintlify hosted at docs.clawd.bot) |
| `/test` | Shared test fixtures, helpers, mocks |
| `/scripts` | Build, test, Docker scripts |
| `/Swabble` | Swift/iOS gateway bundle (separate subproject) |
| `/dist` | Compiled output (build artifacts) |

**Total: 3,135 TypeScript files, 874 test files (.test.ts), 405K lines of code**

---

## 2. Main Programming Languages & File Organization

| Language | Files | Purpose |
|----------|-------|---------|
| **TypeScript** | 2,480 | Core platform, APIs, CLI, channels, agents |
| **Swift** | ~150+ | macOS app, iOS app (in apps/), Swabble gateway |
| **Kotlin/Java** | Android app code | Android native integration |
| **Shell** | 15+ scripts | Docker, install, build automation |
| **JSON5/YAML** | Config files | User configuration |

**Primary stack:** TypeScript/Node.js v22+ (ESM), Express/Hono for HTTP, WebSocket for gateway protocol

---

## 3. Key Modules/Services and Responsibilities

### Core Services (in `/src`)

| Module | Responsibility | Key Files |
|--------|-----------------|-----------|
| **gateway** (127 files) | WebSocket/HTTP gateway, protocol handling, RPC methods | `boot.ts`, `client.ts`, `server-*.ts`, `protocol/` |
| **agents** (292 files) | AI agent orchestration, tools, auth profiles, Pi integration | `pi-embedded-*.ts`, `tool-policy.ts`, `models-config.ts` |
| **channels** (33 dirs) | Multi-channel messaging (WhatsApp, Telegram, Slack, Discord, etc.) | `registry.ts`, `dock.ts`, `plugins/` (40 sub-plugins) |
| **config** (121 files) | Configuration parsing, validation, schema | `config.ts`, `types.*.ts`, `zod-schema.ts`, `io.ts` |
| **memory** (35 files) | Vector search, embeddings, session transcripts | `manager.ts`, `embeddings.ts`, `sqlite-vec.ts` |
| **cli** (106 files) | Command-line interface, subcommands | `program.ts`, `channels-cli.ts`, `models-cli.ts` |
| **commands** (176 files) | High-level CLI/gateway commands | `agent.ts`, `send.ts`, `doctor.ts` |
| **web** (78 files) | WhatsApp Web provider (Baileys integration) | `auto-reply.ts`, `login.ts`, `inbound.ts` |
| **security** (9 files) | Audit, file permissions, ACL | `audit.ts`, `audit-fs.ts`, `windows-acl.ts` |
| **routing** (4 files) | Session/binding resolution | `resolve-route.ts`, `session-key.ts` |
| **sessions** (9 files) | Session store, transcripts, metadata | `store.ts`, `transcript.ts` |
| **logging** (16 files) | Structured logging, subsystem loggers | `subsystem.ts`, `logger.ts` |
| **plugins** (37 files) | Plugin runtime, registry, HTTP routes | `runtime/`, `http-registry.ts` |
| **plugin-sdk** (4 files) | Public SDK exports for plugin authors | `index.ts` (370 exports) |
| **infra** (148 files) | Infrastructure: env, ports, binaries, errors | `env.js`, `ports.ts`, `errors.ts` |
| **tui** (28 files) | Terminal UI (text-based interface) | Dashboard, controls |
| **media** (21 files) | Media handling, mime types, compression | `mime.ts`, `store.ts` |
| **media-understanding** (22 files) | Vision/audio providers (Anthropic, OpenAI, Gemini, Groq, Minimax, Deepgram) | `providers/` |
| **link-understanding** (9 files) | Web scraping, ReadabilityJS | `extract.ts` |
| **browser** (69 files) | Playwright-based browser automation | `routes/`, automation tools |
| **providers** (10 files) | LLM provider abstraction | Model API wrappers |
| **hooks** (30 files) | Webhook ingestion (HTTP POST handlers) | `hooks.ts`, `hooks-mapping.ts` |
| **cron** (23 files) | Scheduled job execution | `cron.ts` |
| **auto-reply** (72 files) | Auto-reply templates, chunking, reply logic | `reply.ts`, `templating.ts` |
| **pairing** (7 files) | Device/node pairing flows | Auth QR codes |
| **daemon** (33 files) | Background service management (launchd/systemd) | `install.ts` |
| **terminal** (13 files) | Terminal utilities, colors, progress | `progress-line.ts` |
| **process** (11 files) | Child process execution, PTY | `exec.ts`, `pty.ts` |
| **acp** (15 files) | Anthropic Client Protocol (tool calling) | Protocol handling |
| **tts** (4 files) | Text-to-speech orchestration | `tts.ts` |
| **utils** (15 files) | Common utilities | `utils.ts`, helpers |
| **markdown** (10 files) | Markdown parsing, table rendering | `tables.ts` |
| **types** (9 files) | TypeScript type declarations (third-party) | `.d.ts` files |

### Channel Implementations (in `/src`)

- **imessage** (15 files) — iMessage/BlueBubbles integration
- **telegram** (78 files) — Telegram Bot API integration
- **slack** (36 files) — Slack Bolt SDK integration
- **discord** (42 files) — Discord.js integration
- **signal** (24 files) — Signal CLI integration
- **line** (36 files) — LINE Bot SDK integration
- **web** (78 files) — WhatsApp Web (Baileys)

### Built-in + Extension Channels

Built-in: WhatsApp, Telegram, Slack, Discord, Signal, iMessage, LINE
Extensions: Microsoft Teams, Matrix, Zalo, Zalo Personal, Nostr, BlueBubbles

---

## 4. Core Data Models & Entities

### Protocol/Gateway Schema (`src/gateway/protocol/schema/`)

```typescript
AgentEventSchema         // Agent event stream (runId, seq, stream, data)
SendParamsSchema         // Message sending (to, message, mediaUrl, channel, accountId)
AgentParamsSchema        // Agent invocation (message, agentId, sessionKey, thinking, deliver)
ChannelAccountSnapshotSchema  // Channel account state
ChannelsStatusResultSchema    // Multi-channel status
ConfigSchemaResponseSchema    // Dynamic config schema
```

### Session Model (`src/config/sessions/types.ts`)

```typescript
SessionEntry {
  sessionId: string
  updatedAt: number
  chatType?: SessionChatType
  thinkingLevel?: string
  modelOverride?: string
  authProfileOverride?: string
  ttsAuto?: TtsAutoMode
  groupActivation?: "mention" | "always"
  queueMode?: "steer" | "followup" | "collect" | "interrupt"
  label?: string
  origin?: SessionOrigin
  skillsSnapshot?: SessionSkillSnapshot
  systemPromptReport?: SessionSystemPromptReport
}
```

### Configuration Model (`src/config/types.clawdbot.ts`)

```typescript
ClawdbotConfig {
  auth?: AuthConfig
  env?: { shellEnv?, vars? }
  wizard?: { lastRunAt, lastRunVersion }
  diagnostics?: DiagnosticsConfig
  logging?: LoggingConfig
  models?: ModelsConfig
  agents?: AgentsConfig
  channels?: ChannelsConfig
  skills?: SkillsConfig
  plugins?: PluginsConfig
  hooks?: HooksConfig
  cron?: CronConfig
  gateway?: GatewayConfig
  approvals?: ApprovalsConfig
  commands?: CommandsConfig
}
```

### Channel Account State

```typescript
ChannelAccountSnapshot {
  accountId: string
  name?: string
  enabled?: boolean
  configured?: boolean
  running?: boolean
  connected?: boolean
  lastConnectedAt?: number
  lastError?: string
  mode?: string
  dmPolicy?: string
  allowFrom?: string[]
}
```

### Memory Search Result

```typescript
MemorySearchResult {
  path: string
  startLine: number
  endLine: number
  score: number
  snippet: string
  source: "memory" | "sessions"
}
```

---

## 5. Important Interfaces/Protocols

### Gateway RPC Protocol

- **WebSocket-based** (`ws://` or `wss://`)
- **JSON-RPC 2.0** with custom extensions
- **Methods**: agent, send, channels.status, chat.send, config.get, config.set, logs.tail, etc.
- **Streaming**: Agent events, chat history, logs via server-sent events
- **Authentication**: Client handshake with role-based authorization (operator, node)

### Channel Plugin Interface (`src/plugin-sdk/index.ts` exports 370+ types)

```typescript
ChannelPlugin {
  id: string
  meta: ChannelMeta
  account: (config) => ChannelAccountState
  setup: ChannelSetupAdapter
  outbound: ChannelOutboundAdapter
  messaging: ChannelMessagingAdapter
  threading?: ChannelThreadingAdapter
  resolver?: ChannelResolverAdapter
  security?: ChannelSecurityAdapter
  group?: ChannelGroupAdapter
  mention?: ChannelMentionAdapter
}
```

### Agent Tool Interface

```typescript
ChannelAgentTool {
  name: string
  description: string
  input?: Record<string, unknown>
  execute: (params, context) => Promise<unknown>
}
```

### Embedding Provider Interface

```typescript
EmbeddingProvider {
  embed: (texts: string[]) => Promise<number[][]>
  getDimensions: () => number
  getModel: () => string
  getProvider: () => string
}
```

---

## 6. Plugin/Extension System

### Loading Mechanism

- **jiti**: JIT runtime TypeScript loader (no build required)
- **Discovery**: `extensions/` and `plugins/entries` directories scanned at boot
- **Registration**: Plugin manifest + optional JSON Schema validation
- **Execution**: In-process (trusted code only)

### Plugin Types

1. **Channel plugins** — Enable/disable messaging channels (40+ in extensions/)
2. **Provider auth plugins** — OAuth flows (Google, Qwen, Copilot)
3. **Memory plugins** — Core (default) or LanceDB (long-term)
4. **Skill plugins** — Custom tools/commands
5. **Service plugins** — Background services

### Manifest Format (`clawdbot.plugin.json`)

```json
{
  "id": "plugin-id",
  "version": "1.0.0",
  "name": "Plugin Name",
  "entry": "./dist/index.js",
  "slots": ["memory"],
  "skills": ["./skills"],
  "config": { ... }
}
```

---

## 7. Authentication/Security Mechanisms

### API Authentication

- **Gateway roles**: operator (full), node (device RPC only), anonymous
- **Scopes**: operator.admin, operator.read, operator.write, operator.approvals, operator.pairing
- **Handshake**: TLS + role verification
- **Timeout**: 10 seconds (configurable via CLAWDBOT_TEST_HANDSHAKE_TIMEOUT_MS)

### Tool Authorization

```typescript
TOOL_PROFILES {
  minimal: ["session_status"]
  coding: ["group:fs", "group:runtime", "group:sessions", "group:memory"]
  messaging: ["group:messaging", sessions tools]
  full: {}
}
```

### File System Security

- **Audit**: Verifies ~/.clawdbot permissions (no world-writable)
- **Windows ACL**: Validates ACLs on Windows
- **Path normalization**: Prevents directory traversal

### External Content

- **Threat detection**: Blocks scripts in fetched HTML
- **MIME type validation**: Verifies file types
- **Media sandboxing**: Vision APIs run server-side

### DM Policy / Group Policy

- **allowFrom**: Allowlist of sender JIDs/IDs per channel
- **dmPolicy**: "all" | "allowlist" | "none"
- **groupPolicy**: Tool access controls per group
- **requireMention**: Enforce @ mention requirement in groups

---

## 8. Database/Storage Layers

### SQLite

- **Main store**: `~/.clawdbot/gateway.db`
- **Schema**: Chat history, sessions, logs, execution approvals
- **Locking**: `proper-lockfile` for concurrent access
- **Backup**: Automatic rotation, compaction settings

### Session Transcripts

- **Location**: `~/.clawdbot/sessions/<sessionKey>/transcript.jsonl`
- **Format**: Newline-delimited JSON (streaming)
- **Tracking**: File watcher (`chokidar`) for changes
- **Encoding**: UTF-8

### Vector Store

- **sqlite-vec**: Vector embedding storage
- **Provider**: OpenAI, Gemini, node-llama-cpp (local), Ollama
- **Search**: Hybrid (vector + BM25 keyword)
- **Chunking**: Markdown-aware, configurable overlap

### File Store

- **Media**: `~/.clawdbot/media/` (images, documents)
- **Config**: `~/.clawdbot/config.json5` (JSON5 format)
- **Logs**: `~/.clawdbot/logs/` (daily rotated, tslog format)
- **Credentials**: `~/.clawdbot/credentials/` (encrypted, provider tokens)

---

## 9. API Structure

### Gateway HTTP Endpoints

- **WebSocket RPC**: `ws://localhost:18789` (default)
- **REST**: `/api/*` (plugin-registered routes, Hono-based)
- **Hooks**: `/hooks/*` (webhook ingestion, configurable base path)
- **OpenAI compat**: `/v1/*` (LLM proxy, chat completions)
- **Control UI**: `/ui/*` (web dashboard)
- **Slack**: `/slack/*` (slash commands, interactions)
- **Canvas**: `/canvas/*` (A2UI WebGL frames)

### RPC Methods (Typebox schemas)

**Read methods** (50+ routes):

```
health, logs.tail, channels.status, models.list, agents.list,
chat.history, sessions.list, cron.list, usage.status, tts.status
```

**Write methods** (15+ routes):

```
send, agent, agent.wait, talk.mode, chat.send, chat.abort,
config.set, config.apply, config.patch, cron.create, cron.delete
```

**Admin/Approvals** (exec.approvals.*):

```
exec.approval.request, exec.approval.resolve, exec.approvals.list
```

**Node/Device** (pairing, registration):

```
node.pair.request, node.pair.approve, device.token.rotate
```

### Protocol Constants (`src/gateway/server-constants.ts`)

```typescript
MAX_PAYLOAD_BYTES = 512 * 1024      // Frame size limit
MAX_BUFFERED_BYTES = 1.5 * 1024 * 1024  // Send buffer
DEFAULT_HANDSHAKE_TIMEOUT_MS = 10_000
TICK_INTERVAL_MS = 30_000
HEALTH_REFRESH_INTERVAL_MS = 60_000
DEDUPE_TTL_MS = 5 * 60_000
DEDUPE_MAX = 1000
```

---

## 10. Cross-Cutting Concerns

### Logging System (`src/logging/`)

- **Subsystem loggers**: Per-module logging with colored output
- **Levels**: trace, debug, info, warn, error, fatal, silent
- **Transports**: Console (pretty/compact/JSON), file (tslog), custom via registerLogTransport
- **Filtering**: Per-subsystem console visibility, verbose mode
- **Metadata**: Structured metadata, consoleMessage override
- **Color coding**: Deterministic per-subsystem
- **TTY detection**: Auto-color detection, NO_COLOR/FORCE_COLOR support

### Error Handling

- **Global handlers**: Unhandled rejection + uncaught exception traps
- **Error formatting**: Detailed context, stack traces
- **Port conflicts**: Helpful OS process info, EADDRINUSE resolution
- **Config validation**: Detailed error messages with paths
- **Custom ErrorCodes**: INVALID_REQUEST, UNAUTHORIZED, NOT_FOUND, INTERNAL_ERROR, etc.

### Caching & Deduplication

- **Dedupe**: Message deduplication with TTL (5 min), max 1000 entries
- **Config snapshot hash**: Prevents re-sends of unchanged config
- **Auth profile cooldowns**: Prevent rapid re-auth attempts
- **Session label caching**: In-memory session metadata cache

### Message Queueing & Rate Limiting

- **Queue modes**: steer (priority), followup, collect, interrupt, queue
- **Debounce**: Configurable per-session (queueDebounceMs)
- **Dequeue lanes**: Priority routing (cron, agent, user)
- **Cap & drop policies**: old (FIFO drop), new (LIFO drop), summarize

### Concurrency Control

- **Session write lock**: Prevents concurrent session edits
- **Lane-based scheduling**: Work distribution across processing lanes
- **Broadcast deduplication**: Per-connection frame dedup

### Terminal UI Components (`src/terminal/`)

- **Progress line**: Overwritable single-line progress
- **Color helpers**: Cross-platform color output
- **Link formatting**: `https://docs.clawd.bot/...` formatting
- **CLI highlighting**: Syntax highlighting for inline code

### Diagnostics

- **Diagnostic events**: Opt-in event stream for telemetry
- **Event types**: session-state, lane-enqueue/dequeue, message-queued/processed, webhook-*, usage
- **Conditional**: Controlled via CLAWDBOT_DIAGNOSTICS env var

---

## 11. Key Constants & Configuration Values

### Tool Groups (`src/agents/tool-policy.ts`)

```typescript
"group:fs" = ["read", "write", "edit", "apply_patch"]
"group:runtime" = ["exec", "process"]
"group:memory" = ["memory_search", "memory_get"]
"group:web" = ["web_search", "web_fetch"]
"group:messaging" = ["message"]
"group:sessions" = [sessions_*, session_status]
"group:ui" = ["browser", "canvas"]
"group:automation" = ["cron", "gateway"]
"group:clawdbot" = [all native tools]
```

### Memory Limits (`src/gateway/server-constants.ts`)

```typescript
DEFAULT_MAX_CHAT_HISTORY_MESSAGES_BYTES = 6 * 1024 * 1024
```

### Session Defaults (`src/config/sessions/types.ts`)

```typescript
DEFAULT_RESET_TRIGGER = "/new"
DEFAULT_RESET_TRIGGERS = ["/new", "/reset"]
DEFAULT_IDLE_MINUTES = 60
```

---

## 12. Important File Paths (for benchmarking)

**Configuration:**

- `/src/config/types.clawdbot.ts` — Main config type (81+ properties)
- `/src/config/zod-schema.ts` — Zod validation schema
- `/src/config/config.ts` — Config I/O and validation

**Gateway Protocol:**

- `/src/gateway/protocol/schema/` — TypeBox schemas (14 files)
- `/src/gateway/server-methods/` — RPC handlers (25+ files)
- `/src/gateway/server-constants.ts` — Constants

**Channels:**

- `/src/channels/dock.ts` — 15K+ LOC multi-channel orchestration
- `/src/channels/plugins/` — 40 channel implementations
- `/src/channels/registry.ts` — Channel discovery

**Sessions:**

- `/src/config/sessions/types.ts` — Session model with 40+ properties
- `/src/config/sessions/store.ts` — Session storage
- `/src/config/sessions/transcript.ts` — Transcript handling

**Security:**

- `/src/channels/plugins/allowlist-match.ts` — Allowlist matching
- `/src/agents/tool-policy.ts` — Tool authorization
- `/src/security/audit.ts` — File system audit

**Memory:**

- `/src/memory/manager.ts` — Vector search orchestration
- `/src/memory/embeddings.ts` — Embedding provider interface
- `/src/memory/hybrid.ts` — BM25 + vector merging

**Logging:**

- `/src/logging/subsystem.ts` — 285 LOC subsystem logger
- `/src/logging/logger.ts` — Root logger setup

---

## Summary Statistics

| Metric | Value |
|--------|-------|
| **Total TypeScript files** | 2,480 |
| **Test files** | 874 |
| **Total LOC** | ~405,000 |
| **Src directories** | 69 |
| **Extensions** | 29 |
| **Skills** | 52 |
| **Built-in channels** | 7 |
| **Extension channels** | 6 |
| **Node.js runtime** | v22+ (ESM) |
| **Config format** | JSON5 with Zod validation |
| **Default gateway port** | 18789 |
| **Default WebSocket path** | `/` |
| **Max payload** | 512 KB |
| **Chat history limit** | 6 MB |
| **Handshake timeout** | 10 seconds |
| **RPC method count** | 50+ |
