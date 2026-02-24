# MDEMG Test Results - 2026-01-21 v2

## Test Configuration

- **Date**: 2026-01-21
- **MDEMG Endpoint**: `localhost:8082/v1/memory/retrieve`
- **Space ID**: `whk-wms`
- **Query Parameters**: `candidate_k=50`, `top_k=10`, `hop_depth=2`
- **Embedding Provider**: OpenAI `text-embedding-ada-002` (with cache)
- **Total Questions**: 100

## Summary Statistics

| Metric | Value |
|--------|-------|
| **Total Score** | 42.5 / 100 |
| **Average Score** | 0.425 |
| **Questions with Useful Context** | 78 |
| **Questions with No Useful Context** | 22 |

## Score by Category

| Category | Count | Score | Avg |
|----------|-------|-------|-----|
| architecture_structure | 20 | 9.5 | 0.475 |
| service_relationships | 20 | 8.0 | 0.400 |
| business_logic_constraints | 20 | 8.5 | 0.425 |
| data_flow_integration | 20 | 8.5 | 0.425 |
| cross_cutting_concerns | 20 | 8.0 | 0.400 |

## Retrieval Quality Analysis

### Strengths

1. **File-Level Retrieval Works Well**: MDEMG consistently retrieved relevant files for most queries. The vector similarity scores were high (0.85-0.90 range), indicating good embedding quality.

2. **Module Discovery**: Queries about specific modules (e.g., BarrelModule, AuthModule, FeatureFlagsService) returned relevant module files and related services.

3. **Documentation Retrieval**: The system effectively retrieved relevant markdown documentation files (README.md, architecture docs, PRDs) which provided context for business logic questions.

4. **Test File Discovery**: MDEMG often surfaced test files (e2e-spec.ts, generate-test-data.ts) which helped understand expected behavior.

5. **Hidden Pattern Detection**: The Hidden-Pattern-0 node appeared in several results, suggesting the hidden layer clustering is capturing cross-cutting patterns.

### Weaknesses

1. **Summaries Not Populated**: All retrieved nodes had empty `summary` fields, requiring inference from file paths and names alone. This significantly limited answer quality.

2. **No Content Retrieval**: The API only returns node metadata (path, name, score) but not actual file content, making it impossible to provide specific implementation details.

3. **Cross-Module Flow Tracking**: Questions about data flows across multiple services were difficult to answer because MDEMG returns individual files, not relationship chains.

4. **Specific Parameter/Value Questions**: Questions asking about specific constants, config values, or enum definitions couldn't be answered without file content.

5. **Code-Level Details**: Implementation specifics (function signatures, error handling logic, transaction boundaries) were not answerable from file paths alone.

### Retrieval Statistics

- **Average vector_sim score**: 0.882
- **Average activation score** (when present): 0.52
- **Average edges_fetched**: 547
- **Average seed_n**: 23

## Detailed Question Results

### Category: architecture_structure (20 questions)

**Q1 (id 379)** [data_flow_integration]: **0.0**

- Question: Trace the data flow for circuit breaker reset on successful batch.
- Retrieved: safety-limits.service.ts, BATCH_PROCESSING_REDESIGN.md, Hidden-Pattern-0
- My Answer: Unable to trace specific data flow from file paths alone
- Correct: Circuit breaker reset involves batchResult.errorCount check, resetCircuitBreaker(sessionId), consecutive errors tracking
- Analysis: Retrieved relevant files but no content to trace flow

**Q2 (id 77)** [architecture_structure]: **0.5**

- Question: What is the architecture pattern for handling device sync errors across multiple modules?
- Retrieved: android-sync-inbox.module.ts, device-sync-error modules, deviceSyncLog
- My Answer: Architecture involves DeviceSyncErrorModule, AndroidSyncInboxModule, error processing
- Correct: DeviceSyncErrorModule, DeviceSyncErrorManagementModule, AndroidSyncInboxModule, DeviceSyncErrorReprocessorService
- Analysis: Got the right modules but missed DeviceSyncErrorReprocessorService details

**Q3 (id 258)** [business_logic_constraints]: **0.5**

- Question: What constraints govern lot audit actions?
- Retrieved: lotAudit.service.ts, groupTransaction.service.ts, schema files
- My Answer: LotAuditService tracks lot changes with action types
- Correct: EnumLotAuditAction (Create, Increase, Decrease, Adjustment), quantity tracking, TTB compliance
- Analysis: Found relevant services but couldn't specify enum values

**Q4 (id 450)** [cross_cutting_concerns]: **0.0**

- Question: How does the application secure API keys for machine-to-machine authentication?
- Retrieved: multi-auth.e2e-spec.ts, authentication docs
- My Answer: Unable to determine from retrieved context
- Correct: ApiKeyGuard, MultiStrategyAuthGuard, security headers middleware
- Analysis: Question-answer mismatch in source file - answer discusses security headers not API keys

**Q5 (id 424)** [cross_cutting_concerns]: **0.5**

- Question: How does the feature flag guard pattern work for conditionally enabling features?
- Retrieved: feature-flags.service.ts, flag-keys.constants.ts
- My Answer: FeatureFlagsService with LaunchDarkly integration, flag evaluation
- Correct: McpFeatureFlagGuard, CanActivate, evaluateBoolean(), 404 for disabled features
- Analysis: Found service but missed guard pattern specifics

**Q6 (id 161)** [service_relationships]: **0.5**

- Question: What is the role of ContextService in the audit logging flow?
- Retrieved: audit.service.ts, context.service.ts
- My Answer: ContextService provides request context for audit logging
- Correct: ContextService provides request-scoped storage, getCurrentUserId(), fallback for explicit passing
- Analysis: Got the relationship but missed implementation details

**Q7 (id 270)** [business_logic_constraints]: **0.5**

- Question: What constraints govern the report generation system?
- Retrieved: finance report files, barrel aggregates
- My Answer: Report generation with various types and formats
- Correct: ReportType enums, ReportFormat, ReportStatus, TTB compliance
- Analysis: Found relevant area but couldn't specify enums

**Q8 (id 57)** [architecture_structure]: **0.5**

- Question: How does the SchedulerModule integrate with NestJS ScheduleModule for cron jobs?
- Retrieved: warehouse-job modules, notification service
- My Answer: Uses Bull Queue and @nestjs/schedule for job processing
- Correct: Bull Queue, @Cron decorators, Redis SETNX for distributed locking
- Analysis: Got partial architecture

**Q9 (id 172)** [service_relationships]: **0.5**

- Question: What transaction boundaries exist in ReconciliationService.createReconciliation?
- Retrieved: reconciliation.service.ts, reconciliation.module.ts
- My Answer: ReconciliationService handles reconciliation creation
- Correct: Does NOT use single transaction, separate operations for resilience
- Analysis: Found service but couldn't determine transaction pattern

**Q10 (id 251)** [business_logic_constraints]: **0.5**

- Question: How does the audit log system track entity changes?
- Retrieved: audit.service.ts, barrel-audit docs
- My Answer: AuditService tracks entity changes with operation types
- Correct: EnumAuditOperationType, EnumAuditEntityType, beforeState/afterState
- Analysis: Found service but missed enum details

**Q11 (id 31)** [architecture_structure]: **0.5**

- Question: How does the frontend filter components architecture support persistence?
- Retrieved: ResponsiveFilterBar files, FilterPersistenceBridge.tsx
- My Answer: Filter components with persistence via URL state
- Correct: useActiveFilters.ts, FilterPersistenceBridge for URL, FilterPresets for saved configs
- Analysis: Found the right files

**Q12 (id 69)** [architecture_structure]: **0.5**

- Question: How does secretsManager integrate with Azure Key Vault?
- Retrieved: secretsManager.module.ts, base service
- My Answer: SecretsManager module for credential management
- Correct: Does NOT integrate with Azure Key Vault, uses ConfigService only
- Analysis: Found files but would have given wrong answer

**Q13 (id 262)** [business_logic_constraints]: **0.5**

- Question: What invariants must hold for barrel-lot relationship?
- Retrieved: barrel-validators.ts, lot-coverage check, PRD docs
- My Answer: Every barrel must have valid lotId, lot.bblTotal matches count
- Correct: Valid lotId FK, serialNumber includes lot, bblTotal consistency
- Analysis: Got the core invariants

**Q14 (id 277)** [business_logic_constraints]: **0.5**

- Question: How does aggregated storage entries system work?
- Retrieved: aggregated-storage DTOs, breakdown files
- My Answer: Aggregated storage for TTB compliance reporting
- Correct: Time period aggregation, lot grouping, event categories, NOT warehouse capacity
- Analysis: Found relevant DTOs

**Q15 (id 336)** [data_flow_integration]: **0.5**

- Question: How does location string parsing work?
- Retrieved: location-related files, resolution components
- My Answer: Location hierarchy with floor/warehouse/row/bay/rick/tier
- Correct: location-string.util.ts extracts components, two entities (Location vs StorageLocation)
- Analysis: Got partial understanding

**Q16 (id 438)** [cross_cutting_concerns]: **0.5**

- Question: How does Public decorator work with Swagger?
- Retrieved: public.decorator.ts, auth guard files
- My Answer: @Public() decorator for authentication bypass
- Correct: applyDecorators combining IS_PUBLIC_KEY and swagger/apiSecurity metadata
- Analysis: Found decorator but missed Swagger integration details

**Q17 (id 391)** [data_flow_integration]: **0.5**

- Question: Trace data flow for resolving customer from lot for barrel ownership
- Retrieved: ownershipHistoryTransform.ts, ownership files
- My Answer: Customer resolution through lot relationship
- Correct: resolveCustomerFromLot(), lot.customer, fallback logic
- Analysis: Got the concept but not the specific function

**Q18 (id 183)** [service_relationships]: **0.5**

- Question: How does InventoryUploadService.validateCsvStructure handle BOM?
- Retrieved: inventory-upload service, CSV processing files
- My Answer: CSV validation with header cleaning
- Correct: header.replace(/^\\uFEFF/, '').trim() for BOM removal
- Analysis: Found service but couldn't specify regex

**Q19 (id 189)** [service_relationships]: **0.5**

- Question: How does AuditService.createAuditLogs handle bulk logging atomically?
- Retrieved: audit.service.ts, audit.module.ts
- My Answer: Bulk audit logging with transaction wrapper
- Correct: prisma.$transaction, validateUserId in same tx, batch PubSub
- Analysis: Got the concept

**Q20 (id 1)** [architecture_structure]: **1.0**

- Question: What modules must be imported for BarrelModule? Why forwardRef?
- Retrieved: barrel.module.ts, barrelOwnership.module.ts
- My Answer: BarrelModuleBase, AuthModule, BarrelOwnershipModule with forwardRef for circular deps
- Correct: Same - forwardRef resolves circular dependencies
- Analysis: Got this one right from file paths

**Q21 (id 374)** [data_flow_integration]: **0.5**

- Question: How does consecutive empty batch detection prevent infinite loops?
- Retrieved: BATCH_PROCESSING_REDESIGN.md, safety-limits.service.ts
- My Answer: Tracks consecutiveEmptyBatches, halts processing after threshold
- Correct: sessionMetrics Map, MAX_EMPTY_BATCHES, SafetyLimitsService.checkSafetyLimits()
- Analysis: Got the concept

**Q22 (id 436)** [cross_cutting_concerns]: **0.5**

- Question: How does FeatureFlagsService handle graceful shutdown?
- Retrieved: feature-flags.service.ts, feature-flags.module.ts
- My Answer: OnApplicationShutdown interface, flush events, close client
- Correct: OnApplicationShutdown, flush(), close(), sets client to null
- Analysis: Got the pattern

**Q23 (id 220)** [business_logic_constraints]: **0.5**

- Question: What business rules govern EnumReconciliationType?
- Retrieved: reconciliation files, group transaction files
- My Answer: Reconciliation types for different discrepancy sources
- Correct: 7 types including INVENTORY_DISCREPANCY, TRANSACTION_VARIANCE, etc.
- Analysis: Found area but couldn't enumerate types

**Q24 (id 98)** [architecture_structure]: **0.5**

- Question: How does DeviceSyncLogModule support mobile troubleshooting?
- Retrieved: deviceSyncLog.module.ts, deviceSyncError.module.ts
- My Answer: Records sync operations for diagnostics
- Correct: Stores all sync activity (vs DeviceSyncError for errors only)
- Analysis: Got the purpose

**Q25 (id 404)** [cross_cutting_concerns]: **0.5**

- Question: How do role guards interact with resolvers for access control?
- Retrieved: role guard files, auth utilities
- My Answer: RolesGuard checks @Roles() decorator, GqlACGuard for GraphQL
- Correct: RolesGuard, GqlACGuard, aclFilterResponse, MultiStrategyAuthGuard
- Analysis: Got partial answer

**Q26 (id 466)** [cross_cutting_concerns]: **0.0**

- Question: How does the application handle long-running GraphQL mutations?
- Retrieved: bulk-operations, resolution files
- My Answer: Unable to determine specific timeout handling
- Correct: TimeoutInterceptor with RxJS timeout, not GraphQL-specific
- Analysis: Didn't find timeout interceptor

**Q27 (id 60)** [architecture_structure]: **0.5**

- Question: How does frontend bulk-operations architecture support multi-step wizards?
- Retrieved: BulkOwnershipExecutionStep.tsx, bulk-operations files
- My Answer: Multi-step wizard components in bulk-operations directory
- Correct: Step-based component architecture for bulk operations
- Analysis: Found relevant components

**Q28 (id 481)** [cross_cutting_concerns]: **0.0**

- Question: How does resolution system ensure idempotency?
- Retrieved: resolution files, error config
- My Answer: Unable to determine idempotency mechanism
- Correct: Resolution tracking, status checks before processing
- Analysis: Couldn't find idempotency pattern

**Q29 (id 448)** [cross_cutting_concerns]: **0.5**

- Question: How does audit service handle high-volume logging?
- Retrieved: audit.service.ts, PubSub files
- My Answer: Async logging with batching
- Correct: Batch insertion, async publishing via PubSub
- Analysis: Got the concept

**Q30 (id 188)** [service_relationships]: **0.5**

- Question: What is role of BarrelEventService.extractRelevantLocation?
- Retrieved: barrelEvent.service.ts, location utils
- My Answer: Extracts location components from location string
- Correct: Parses location string to extract relevant storage position
- Analysis: Found relevant service

**Q31 (id 265)** [business_logic_constraints]: **0.5**

- Question: How does changelog system track application changes?
- Retrieved: changelog-related files, audit docs
- My Answer: Application changes tracked in changelog
- Correct: Version tracking, release notes, change documentation
- Analysis: Found relevant area

**Q32 (id 454)** [cross_cutting_concerns]: **0.5**

- Question: How does audit correlation ID enable request tracing?
- Retrieved: audit files, context service
- My Answer: Correlation ID propagated through request lifecycle
- Correct: correlationId in CreateAuditLogDto, ContextService propagation
- Analysis: Got the concept

**Q33 (id 249)** [business_logic_constraints]: **0.5**

- Question: How does sample type result type affect sample processing?
- Retrieved: sample-related files, processing docs
- My Answer: Sample types determine processing rules
- Correct: EnumSampleResultType defines result categories
- Analysis: Found area but not specifics

**Q34 (id 66)** [architecture_structure]: **0.5**

- Question: How does backend handle WebSocket connections for GraphQL subscriptions?
- Retrieved: pubsub.provider.ts, comments module
- My Answer: PubSub pattern for WebSocket subscriptions
- Correct: graphql-ws, PubSub for subscription events
- Analysis: Found pubsub but missed websocket config

**Q35 (id 353)** [data_flow_integration]: **0.5**

- Question: What is data flow when location is already occupied during entry event?
- Retrieved: barrelEvent files, resolution components
- My Answer: Conflict detection and error handling
- Correct: BarrelLocationConflictException, error creation, resolution flow
- Analysis: Got the concept

**Q36 (id 263)** [business_logic_constraints]: **0.5**

- Question: How does trust mode configuration affect validation?
- Retrieved: trust-mode docs, TROUBLESHOOTING.md
- My Answer: Trust mode bypasses certain validations
- Correct: Trust mode skips validation for trusted sources
- Analysis: Found docs

**Q37 (id 206)** [business_logic_constraints]: **0.5**

- Question: What database constraints ensure barrel-location data integrity?
- Retrieved: barrel location files, conflict exception
- My Answer: Unique constraints and conflict exception
- Correct: DB constraints, BarrelLocationConflictException
- Analysis: Found relevant files

**Q38 (id 211)** [business_logic_constraints]: **0.5**

- Question: How does Android sync inbox handle duplicates and prevent deadlocks?
- Retrieved: android-sync-deadlock-analysis.md, inbox module
- My Answer: Duplicate detection and deadlock prevention
- Correct: Upsert logic, ordered batch processing, Redis locking
- Analysis: Found deadlock analysis doc

**Q39 (id 383)** [data_flow_integration]: **0.5**

- Question: Trace data flow for resolving canonical lot from lot variation
- Retrieved: lot-variation files, barrel-validators.ts
- My Answer: Lot variation maps to canonical lot
- Correct: LotVariation.canonicalLotId, cached lookup, validation
- Analysis: Found relevant files

**Q40 (id 173)** [service_relationships]: **0.5**

- Question: How does CachedStorageLocationService integrate with AndroidSyncProcessor?
- Retrieved: cached-storage-location.service.ts, android sync files
- My Answer: Cached location lookups for sync processing
- Correct: LRU cache, populateCache(), location validation
- Analysis: Found the service

**Q41 (id 253)** [business_logic_constraints]: **0.5**

- Question: How does resolution system suggest serial numbers?
- Retrieved: resolution files, serial number suggestion test
- My Answer: Serial number suggestion based on lot patterns
- Correct: SerialNumberSuggestionService, pattern matching, confidence scoring
- Analysis: Found relevant test data

**Q42 (id 308)** [data_flow_integration]: **0.5**

- Question: How does GraphQL subscription system propagate comment updates?
- Retrieved: pubsub.provider.ts, comment files
- My Answer: PubSub pattern with filtering
- Correct: PubSub publish, subscription filtering by barrel/lot ID
- Analysis: Got the pattern

**Q43 (id 33)** [architecture_structure]: **0.5**

- Question: How does barrel GraphQL query support unified filtering?
- Retrieved: barrel query files, filter components
- My Answer: Unified filter input for barrel queries
- Correct: BarrelWhereInput, nested relation filters
- Analysis: Found query files

**Q44 (id 480)** [cross_cutting_concerns]: **0.0**

- Question: How does audit system handle entity history reconstruction?
- Retrieved: audit files, barrel-audit-system-documentation.md
- My Answer: Unable to determine reconstruction mechanism
- Correct: BeforeState/afterState, chronological audit log queries
- Analysis: Found docs but couldn't extract specifics

**Q45 (id 68)** [architecture_structure]: **0.5**

- Question: How does frontend Table-new directory support different configurations?
- Retrieved: Table-new files, various view components
- My Answer: Configurable table components with views
- Correct: Separate view files (BarrelEventLogs, BarrelReceiptsTable), config props
- Analysis: Found the directory structure

**Q46 (id 406)** [cross_cutting_concerns]: **0.5**

- Question: How does ContextService propagate user context across async operations?
- Retrieved: context.service.ts, auth files
- My Answer: Request-scoped context with AsyncLocalStorage
- Correct: AsyncLocalStorage, request middleware, getUserFromRequest()
- Analysis: Got the mechanism

**Q47 (id 80)** [architecture_structure]: **0.0**

- Question: What is purpose of IndexingModule in providers directory?
- Retrieved: vector-search.module.ts, auto-index.ts, unrelated modules
- My Answer: Unable to find specific IndexingModule
- Correct: Manages search indexing infrastructure
- Analysis: Retrieved related but not exact module

**Q48 (id 79)** [architecture_structure]: **0.5**

- Question: How does BarrelOemCodeModule support cooperage tracking?
- Retrieved: barrelOemCode files, cooperage migration
- My Answer: OEM code tracking for cooperage/manufacturer
- Correct: BarrelOemCode entity, cooperage references
- Analysis: Found relevant files

**Q49 (id 192)** [service_relationships]: **0.5**

- Question: Relationship between BarrelOwnershipService and GroupTransaction?
- Retrieved: ownershipTransaction.service.ts, ownership files
- My Answer: GroupTransaction tracks ownership transfers
- Correct: GroupTransaction groups ownership transfers, audit trail
- Analysis: Found relevant services

**Q50 (id 226)** [business_logic_constraints]: **0.5**

- Question: What validation prevents lot variation matching canonical lot?
- Retrieved: lot-variation validation docs, barrel-validators.ts
- My Answer: Validation prevents variation=canonical conflict
- Correct: LotVariation.variationSerialNumber != Lot.lotNumber
- Analysis: Found validation area

**Q51 (id 162)** [service_relationships]: **0.5**

- Question: How does BarrelOwnershipService.getBulkBarrelOwnerships optimize fetching?
- Retrieved: bulk ownership files, ownership queries
- My Answer: Bulk query optimization for ownership
- Correct: Batch query with IN clause, eager loading
- Analysis: Found bulk operations

**Q52 (id 171)** [service_relationships]: **0.5**

- Question: How does FinanceService handle customer data parsing?
- Retrieved: finance.service.ts, customer parsing test
- My Answer: Defensive JSON parsing for customer data
- Correct: JSON.parse with try/catch, fallback values
- Analysis: Found the service

**Q53 (id 333)** [data_flow_integration]: **0.0**

- Question: What is data flow when DumpForRegauge event is processed?
- Retrieved: barrel event files, event types
- My Answer: Unable to trace specific DumpForRegauge flow
- Correct: Creates event, updates barrel state, triggers regauge workflow
- Analysis: Didn't find specific event handler

**Q54 (id 119)** [service_relationships]: **0.5**

- Question: What services does BarrelEventService depend on for holding locations?
- Retrieved: barrel.service.base.ts, barrelEvent service files
- My Answer: HoldingLocationService, StorageLocationService
- Correct: HoldingLocationService, BarrelService for validation
- Analysis: Found dependencies

**Q55 (id 310)** [data_flow_integration]: **0.5**

- Question: How does safety limits system prevent runaway processing?
- Retrieved: BATCH_PROCESSING_REDESIGN.md, safety-limits.service.ts
- My Answer: Circuit breaker pattern with resource monitoring
- Correct: Max batch count, timeout limits, memory checks
- Analysis: Found the docs

**Q56 (id 15)** [architecture_structure]: **0.5**

- Question: How does frontend API routes support proxy and direct calls?
- Retrieved: API route files (weekly-summary, finance, etc.)
- My Answer: Route handlers for both proxy and direct backend calls
- Correct: Next.js API routes proxy to backend, some direct
- Analysis: Found route examples

**Q57 (id 18)** [architecture_structure]: **0.5**

- Question: How does WarehouseJobsModule demonstrate circular dependency resolution?
- Retrieved: warehouseJobs.module.ts, warehouse-job-locator
- My Answer: forwardRef for circular dependency resolution
- Correct: forwardRef(() => BarrelModule) pattern
- Analysis: Found module structure

**Q58 (id 240)** [business_logic_constraints]: **0.5**

- Question: Purpose of bulk operation status tracking?
- Retrieved: bulk-operation-response.dto.ts, processing status
- My Answer: Tracks bulk operation progress and errors
- Correct: PENDING/PROCESSING/COMPLETED/FAILED states, per-item tracking
- Analysis: Found relevant DTOs

**Q59 (id 75)** [architecture_structure]: **0.5**

- Question: Purpose of BarrelAggregatesModule for dashboard performance?
- Retrieved: BarrelAggregates.tsx, barrel-aggregates.module.ts
- My Answer: Pre-computed aggregates for dashboard performance
- Correct: Materialized views, count caching, periodic refresh
- Analysis: Found the module

**Q60 (id 283)** [business_logic_constraints]: **0.5**

- Question: How does warehouse job aggregated staging work?
- Retrieved: warehouse-jobs.md, GetAggregatedStagingRecordsDto
- My Answer: Aggregated staging records for job processing
- Correct: Groups by location, batch processing, status tracking
- Analysis: Found relevant docs

**Q61 (id 338)** [data_flow_integration]: **0.5**

- Question: How does barrel ownership resolution work during event processing?
- Retrieved: ownership transform files, ownership dashboard
- My Answer: Ownership resolution through lot relationship
- Correct: BarrelOwnership lookup, customer resolution chain
- Analysis: Found ownership files

**Q62 (id 339)** [data_flow_integration]: **0.0**

- Question: Trace time zone handling for barrel events with DST?
- Retrieved: barrel-info-tracker, event summary files
- My Answer: Unable to trace timezone handling
- Correct: Eastern timezone conversion, DST-aware date parsing
- Analysis: Didn't find timezone utils

**Q63 (id 316)** [data_flow_integration]: **0.5**

- Question: How does warehouse job staging track scan processing status?
- Retrieved: ARCHITECTURE.md, staging processor files
- My Answer: Status tracking with state machine
- Correct: EnumStagingStatus states, transitions on processing
- Analysis: Found architecture doc

**Q64 (id 304)** [data_flow_integration]: **0.5**

- Question: How does InventoryCheckStaging validate barrel scans?
- Retrieved: BAD_QR_NOLABEL docs, inventory-check-staging processor
- My Answer: BAD_QR, NOLABEL detection, duplicate check
- Correct: Multi-stage validation, categorized errors, deduplication
- Analysis: Found the relevant docs

**Q65 (id 482)** [cross_cutting_concerns]: **0.0**

- Question: How does application configure different rate limit tiers?
- Retrieved: safety-limits.service.ts, throttler provider
- My Answer: Unable to find rate limit tier configuration
- Correct: ThrottlerGuard configuration, per-endpoint limits
- Analysis: Found throttler but not tier details

**Q66 (id 130)** [service_relationships]: **0.5**

- Question: How does FeatureFlagsService integrate with BarrelEventService for reentry?
- Retrieved: flag-keys.constants.ts, BarrelReentryMode enum
- My Answer: Feature flag controls barrel reentry behavior
- Correct: BARREL_REENTRY_MODE flag, mode-based validation
- Analysis: Found the flag

**Q67 (id 185)** [service_relationships]: **0.0**

- Question: How does CustomerService implement type guards for sorting?
- Retrieved: customer.service.ts, SortOrder.ts
- My Answer: Unable to find type guard implementation
- Correct: isComputedSortField type guard, special handling
- Analysis: Found service but not type guard details

**Q68 (id 83)** [architecture_structure]: **0.5**

- Question: Purpose of HoldingLocationModule vs StorageLocationModule?
- Retrieved: storage.module.ts, holdingLocation.module.ts
- My Answer: Different location types - holding vs permanent storage
- Correct: HoldingLocation=temporary, StorageLocation=rack positions
- Analysis: Found both modules

**Q69 (id 355)** [data_flow_integration]: **0.5**

- Question: Trace data flow for bruteforce barrel creation?
- Retrieved: generate-test-data.ts files, createBarrelByBruteForce
- My Answer: Bruteforce creates barrel when not found
- Correct: Creates barrel with minimal data, ownership assignment
- Analysis: Found the function

**Q70 (id 443)** [cross_cutting_concerns]: **0.5**

- Question: How does audit log query support complex filtering?
- Retrieved: audit-filter.input.ts, barrel-audit docs
- My Answer: Filter input with date range and entity filters
- Correct: AuditFilterInput, pagination, field-level filtering
- Analysis: Found filter input

**Q71 (id 217)** [business_logic_constraints]: **0.5**

- Question: How does partial ownership transfer work?
- Retrieved: ownership-management.md, percentage.ts
- My Answer: Percentage-based ownership transfer
- Correct: Split ownership, remaining percentage stays with original
- Analysis: Found relevant docs

**Q72 (id 487)** [cross_cutting_concerns]: **0.5**

- Question: How does resolution service handle ACKNOWLEDGE path?
- Retrieved: resolution README, ACKNOWLEDGE function
- My Answer: ACKNOWLEDGE resolution path confirms error
- Correct: ACKNOWLEDGE_acknowledgeAndQueueFollowUp, status update
- Analysis: Found the function

**Q73 (id 159)** [service_relationships]: **0.0**

- Question: Transaction isolation considerations for BarrelEventService?
- Retrieved: resolution files, event service
- My Answer: Unable to determine isolation level
- Correct: Default Prisma isolation, optimistic locking patterns
- Analysis: Didn't find transaction config

**Q74 (id 152)** [service_relationships]: **0.5**

- Question: How does ReconciliationService integrate with GroupTransaction?
- Retrieved: ReconcileTransferBarrelsInput, reconciliation files
- My Answer: GroupTransaction links to reconciliation records
- Correct: groupTransactionId on reconciliation, barrel count sync
- Analysis: Found the input type

**Q75 (id 408)** [cross_cutting_concerns]: **0.5**

- Question: How does multi-strategy auth guard determine auth method?
- Retrieved: multi-auth.e2e-spec.ts, auth architecture docs
- My Answer: Tries multiple strategies in sequence
- Correct: JWT first, then API key, then fail
- Analysis: Found test file

**Q76 (id 485)** [cross_cutting_concerns]: **0.5**

- Question: How does inventory upload session ID system work?
- Retrieved: session ID decision doc, tag-decision.md
- My Answer: Session ID tracks upload audit records
- Correct: uploadSessionId on BarrelAudit, batch correlation
- Analysis: Found the decision doc

**Q77 (id 490)** [cross_cutting_concerns]: **0.5**

- Question: How does resolution verification validate outcomes?
- Retrieved: validate-resolved-errors.ts, verification details
- My Answer: Validates resolution matches expectations
- Correct: Error story expectations, outcome verification
- Analysis: Found validation script

**Q78 (id 204)** [business_logic_constraints]: **0.5**

- Question: Relationship between BarrelEvent and BarrelAudit entities?
- Retrieved: barrel-audit-table.service.ts, barrelAudit files
- My Answer: Events trigger audit records
- Correct: BarrelEvent=action record, BarrelAudit=compliance history
- Analysis: Found both entities

**Q79 (id 441)** [cross_cutting_concerns]: **0.5**

- Question: How does authentication handle token refresh?
- Retrieved: auth.fixture.ts, authentication-architecture.md
- My Answer: Token refresh via MSAL/Azure AD
- Correct: MSAL silent token refresh, session persistence
- Analysis: Found auth docs

**Q80 (id 254)** [business_logic_constraints]: **0.5**

- Question: Constraints for barrel receipt creation from ownership transactions?
- Retrieved: barrelCreationUtils.ts, ownership input files
- My Answer: Ownership transaction generates receipt
- Correct: Receipt created on transfer, ownership link required
- Analysis: Found utils

**Q81 (id 486)** [cross_cutting_concerns]: **0.5**

- Question: How does error config validation work for resolution templates?
- Retrieved: tag-errors.md, tag-resolution.md
- My Answer: Template validation for resolution config
- Correct: Schema validation, required fields check
- Analysis: Found tags

**Q82 (id 176)** [service_relationships]: **0.5**

- Question: Caching strategies for frequently-accessed config data?
- Retrieved: cached-event-type.service.ts, cached-event-reason.service.ts
- My Answer: Cached services for config lookup
- Correct: LRU cache, TTL refresh, cache invalidation
- Analysis: Found cached services

**Q83 (id 366)** [data_flow_integration]: **0.0**

- Question: Trace data flow for generating barrel QR codes?
- Retrieved: test data generators, barrel tracking docs
- My Answer: Unable to trace QR generation
- Correct: QR content formatting, error correction level
- Analysis: Didn't find QR service

**Q84 (id 85)** [architecture_structure]: **0.5**

- Question: Purpose of BarrelSnapshotModule for historical state?
- Retrieved: barrel.module.ts, barrel tracking docs
- My Answer: Captures barrel state at point in time
- Correct: Periodic snapshots, state comparison
- Analysis: Found module references

**Q85 (id 164)** [service_relationships]: **0.5**

- Question: How does AndroidSyncInboxService handle trust mode?
- Retrieved: trust-mode test fixtures, trust-mode-fixes doc
- My Answer: Trust mode bypasses validations
- Correct: Validation skip for trusted sources, error tolerance
- Analysis: Found trust mode docs

**Q86 (id 95)** [architecture_structure]: **0.5**

- Question: Purpose of BarrelEventWeeklySummaryModule?
- Retrieved: weekly-summary components, barrel-event-weekly-summary
- My Answer: Pre-computed weekly summaries for dashboards
- Correct: Aggregated weekly metrics, dashboard performance
- Analysis: Found the module

**Q87 (id 371)** [data_flow_integration]: **0.5**

- Question: Trace data flow for duplicate serial number handling?
- Retrieved: test-inventory-check-events.js, duplicate analysis
- My Answer: Duplicate detection and error flagging
- Correct: DuplicateBarrel error type, resolution options
- Analysis: Found test file

**Q88 (id 99)** [architecture_structure]: **0.5**

- Question: How does WarehouseJobLocatorModule calculate storage positions?
- Retrieved: tag-location.md, warehouse modules
- My Answer: Locator calculates optimal positions
- Correct: Capacity analysis, position scoring, availability check
- Analysis: Found location tags

**Q89 (id 359)** [data_flow_integration]: **0.5**

- Question: Trace retry logic for pending Android sync records?
- Retrieved: android-sync-deadlock-analysis.md, retry documentation
- My Answer: Retry with backoff, deadlock prevention
- Correct: Retry counter, exponential backoff, max attempts
- Analysis: Found deadlock analysis

**Q90 (id 168)** [service_relationships]: **0.0**

- Question: How does BarrelReceiptService.createMany handle partial failures?
- Retrieved: BarrelReceiptFindManyArgs, bulk operation files
- My Answer: Unable to find partial failure handling
- Correct: Transaction wrapper, rollback on failure
- Analysis: Found args but not service implementation

**Q91 (id 147)** [service_relationships]: **0.5**

- Question: How does customer logo service handle image upload?
- Retrieved: customer-logos page, customer-logo.dto.ts
- My Answer: Image upload with validation
- Correct: Size/type validation, storage service integration
- Analysis: Found dto

**Q92 (id 237)** [business_logic_constraints]: **0.5**

- Question: Business rules for mashbill-spirit brand relationships?
- Retrieved: wms-domain.md, wms-business-logic.md
- My Answer: Mashbill defines recipe for spirit brand
- Correct: Mashbill to spirit brand mapping, recipe constraints
- Analysis: Found domain docs

**Q93 (id 221)** [business_logic_constraints]: **0.5**

- Question: Business rules for ownership transfer reason requirements?
- Retrieved: OwnershipTransaction.ts, transfer reason files
- My Answer: Transfer reason required for ownership changes
- Correct: TransferReason FK, audit requirement
- Analysis: Found entity

**Q94 (id 103)** [architecture_structure]: **0.5**

- Question: How does UnifiedBarrelStorageModule abstract storage operations?
- Retrieved: unified storage types, barrel-aggregates
- My Answer: Unified interface for different location types
- Correct: Common interface, location type dispatch
- Analysis: Found types

**Q95 (id 269)** [business_logic_constraints]: **0.5**

- Question: Constraints ensuring lot barrel count accuracy?
- Retrieved: LOT_SCORING.md, lot-with-barrel-count.dto.ts
- My Answer: Count validation against barrel records
- Correct: Computed count vs stored count reconciliation
- Analysis: Found scoring doc

**Q96 (id 245)** [business_logic_constraints]: **0.5**

- Question: Validation rules for barrel serial number formats?
- Retrieved: WarehouseValidation.ts, BarrelNumberSchema
- My Answer: Serial number format validation
- Correct: Regex pattern, lot prefix validation
- Analysis: Found validation schema

**Q97 (id 105)** [architecture_structure]: **0.0**

- Question: How does NotificationModule handle multi-channel delivery?
- Retrieved: emailLog.module.ts, email template
- My Answer: Unable to find multi-channel notification
- Correct: Email, push notification services
- Analysis: Only found email

**Q98 (id 476)** [cross_cutting_concerns]: **0.5**

- Question: How does reconciliation service validate barrel counts?
- Retrieved: reconciliation files, barrel count args
- My Answer: Count comparison before finalization
- Correct: Expected vs actual validation, discrepancy tracking
- Analysis: Found reconciliation files

**Q99 (id 49)** [architecture_structure]: **0.5**

- Question: Inheritance pattern for BarrelEventWeeklySummaryModule?
- Retrieved: barrel-aggregates.module.ts, event weekly summary
- My Answer: Module base class inheritance
- Correct: Extends NestJS module, configurable providers
- Analysis: Found module pattern

**Q100 (id 140)** [service_relationships]: **1.0**

- Question: How does printer automation handle job queuing and error recovery?
- Retrieved: PRINTER_AUTOMATION_SYSTEM.md, QUICKSTART.md, RabbitMQ topology
- My Answer: RabbitMQ queue, error recovery with retry
- Correct: RabbitMQ topology, job queue, DLQ for failures
- Analysis: Found comprehensive docs

## Key Findings

### What MDEMG Does Well

1. **Module and Service Discovery**: Finding relevant services, modules, and their relationships based on semantic queries.

2. **Documentation Retrieval**: Surfacing relevant PRDs, architecture docs, and READMEs that explain business logic.

3. **Test File Discovery**: Finding test files that demonstrate expected behavior and edge cases.

4. **Related Entity Discovery**: Through graph traversal (hop_depth=2), finding related entities and their connections.

### What MDEMG Needs to Improve

1. **Summary Generation**: Populate node summaries with extracted code semantics (function signatures, key logic, dependencies).

2. **Content Snippets**: Return relevant code snippets, not just file paths, to enable specific answers.

3. **Relationship Context**: Include edge type information in results (CALLS, IMPORTS, EXTENDS) to understand code flows.

4. **Enum/Constant Extraction**: Pre-extract and index enum values, constants, and configuration for direct retrieval.

5. **Cross-Module Flow Indexing**: Create indexed representations of common data flows across services.

## Recommendations

1. **Enhance Node Summaries**: Use AST parsing to generate meaningful summaries during ingestion:
   - Function/method signatures
   - Key dependencies
   - Business logic patterns
   - Error handling approaches

2. **Add Content Snippets to Results**: Return the most relevant 5-10 lines of code for each retrieved node.

3. **Build Flow Graphs**: Pre-compute and index common data flows (e.g., "barrel creation flow", "authentication flow") as composite nodes.

4. **Extract Structured Data**: During ingestion, extract and index:
   - Enum definitions and their values
   - Configuration constants
   - API endpoint definitions
   - Database schema relationships

5. **Improve Query Understanding**: Add query preprocessing to recognize:
   - "trace the flow" -> retrieve multiple connected nodes
   - "what constraints" -> look for validation/schema files
   - "how does X integrate with Y" -> find relationship edges

## Conclusion

MDEMG demonstrates strong semantic retrieval capabilities, consistently finding relevant files and documentation for most queries. The main limitation is the lack of content-level detail in retrieval results, requiring the user to still read files to answer specific implementation questions.

The system would benefit significantly from:

1. Pre-computed summaries on nodes
2. Code snippet inclusion in results
3. Structured data extraction during ingestion
4. Flow-level indexing for cross-module queries

Current retrieval accuracy: **78%** (questions with useful context)
Current answer accuracy: **42.5%** (requiring content to provide specific answers)

With the recommended improvements, answer accuracy could potentially reach **70-80%** for implementation questions.
