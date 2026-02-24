# Service Relationships Category - Clawdbot Benchmark Questions

## Overview

Created 20 questions testing understanding of service communication patterns, dependency injection, initialization order, and inter-module protocols in the clawdbot codebase.

## Question Distribution

### Communication Patterns (8 questions)

- Q21: Gateway RPC → Agent Command invocation
- Q23: Agent lifecycle event listening and job status retrieval
- Q24: Agent run context registration and event emission
- Q28: Gateway → Plugin logout handler flow
- Q31: CronService → Agent command execution
- Q37: Gateway RPC deduplication state sharing
- Q40: Gateway server service coordination (channels, cron, events)

### Service Dependencies & Injection (5 questions)

- Q22: ChannelDock registry → Plugin configuration resolution
- Q26: Core tools → Plugin tools integration
- Q27: Memory service → Agent execution integration
- Q34: Runtime environment dependency injection in channel lifecycle
- Q35: Plugin registry channel lifecycle hook management

### Initialization & Lifecycle (4 questions)

- Q25: Channel plugin initialization order via ChannelManager
- Q29: Session state persistence across agent command execution
- Q33: Agent delivery target resolution from session state
- Q36: Agent execution → Model fallback service integration

### Data Aggregation (3 questions)

- Q30: Gateway RPC handler registry assembly
- Q32: Channel status probing and account snapshot building
- Q38: Multi-agent session store merging
- Q39: Channel account snapshot construction from runtime state

## Key Service Relationships Tested

### Gateway Server Hub

- RPC method registration and routing (Q30, Q37)
- Channel management orchestration (Q25, Q28, Q32, Q39)
- Agent command invocation (Q21, Q33)
- Service initialization coordination (Q40)

### Plugin System

- Channel plugin lifecycle (Q22, Q25, Q28, Q35)
- Tool plugin integration (Q26)
- Runtime dependency injection (Q34)

### Agent Execution Pipeline

- Command service entry point (Q21, Q31)
- Event emission and tracking (Q23, Q24)
- Model fallback handling (Q36)
- Memory integration (Q27)

### State Management

- Session persistence (Q29)
- Multi-agent store aggregation (Q38)
- Delivery target resolution (Q33)
- Runtime snapshot merging (Q32, Q39)

## Complexity Distribution

- **Multi-file**: 8 questions (requires reading 2 files)
- **Cross-module**: 9 questions (requires understanding 2-3 modules)
- **System-wide**: 3 questions (requires tracing through 4+ files)

## File Coverage

Most frequently referenced files:

1. `src/gateway/server-methods/agent.ts` (5 questions)
2. `src/gateway/server-channels.ts` (4 questions)
3. `src/infra/agent-events.ts` (4 questions)
4. `src/channels/plugins/index.ts` (4 questions)
5. `src/commands/agent.ts` (3 questions)
6. `src/gateway/session-utils.ts` (3 questions)
7. `src/plugins/runtime.ts` (3 questions)

## Answer Verification

All answers include:

- Specific file paths (absolute, starting with `src/`)
- Line number references where key calls occur
- Function/method names involved in the communication
- Complete explanation of the service interaction flow

## Question Quality Assurance

✅ Each question requires reading 2+ files
✅ All answers verified against actual source code
✅ Unambiguous - only one correct answer
✅ Specific line numbers and function names provided
✅ Tests understanding of actual service contracts, not just file structure
