# PHASE 2: TEST QUESTIONS (100 total)

## INSTRUCTIONS

You have completed ingestion. Now answer these 100 questions.

### RULES

- You MAY access /Users/reh3376/whk-wms/ to verify/lookup information
- Do NOT access /Users/reh3376/mdemg/ (contains test answers - invalidates test)
- You SHOULD first attempt to answer from your ingested knowledge
- Note whether each answer came from: [MEMORY], [LOOKUP], or [BOTH]

### Scoring

- 1.0 = Completely correct
- 0.5 = Partially correct (right concept, wrong details)
- 0.0 = Unable to answer
- -1.0 = Confidently wrong

### Output Format

```
Q[number]: [question]
Source: [MEMORY/LOOKUP/BOTH]
Answer: [your answer]
Score: [self-assessed score]
Reasoning: [brief explanation]
```

---

## QUESTIONS

### Question 1 (Category: data_flow_integration)

What is the complete data flow integration test strategy for the WMS?

Expected Answer: Integration test strategy: 1) Test database: separate PostgreSQL instance or test schema, 2) Test Redis: separate instance or mock, 3) BullMQ processors: instantiate in test, process jobs synchronously, 4) GraphQL: use supertest to call endpoints with test auth tokens, 5) Mock Azure AD: stub validateTokenAndGetUser to return test user, 6) WebSocket tests: use Socket.io client to connect and verify events, 7) Data setup: seed test data via Prisma, clean up after tests, 8) Assertions: verify database state, event emissions, error handling, 9) Coverage: happy path, error scenarios, edge cases (empty batches, duplicates), 10) CI integration: run in isolated environment with test configs.

---

### Question 2 (Category: architecture_structure)

What is the purpose of having both DeltaSyncModule and SyncModule?

Expected Answer: DeltaSyncModule handles mobile device synchronization - fetching barrel audit data changes since a given timestamp with cursor-based pagination for efficient mobile updates. It uses DeviceSyncLogService for logging sync operations and supports both BarrelAudit and Barrel table sources. SyncModule handles entity synchronization FROM external systems (MES/ERP) - providing generic upsert/delete operations for items, customers, vendors, lots, recipes, purchase orders, and sales orders. It parses Prisma schema relations and transforms incoming data with validation. The two serve completely different purposes: DeltaSyncModule pulls data OUT to mobile devices, while SyncModule pushes data IN from external systems.

---

### Question 3 (Category: architecture_structure)

Why does AppModule configure ThrottlerModule with both in-memory and Redis storage options?

Expected Answer: AppModule uses ThrottlerModule.forRootAsync with ThrottlerStorageRedisProvider that returns Redis storage in production/staging but undefined (in-memory) for development. This is crucial for multi-instance deployments where rate limits must be shared across servers. The configuration defines three throttle tiers (short: 10/sec, medium: 50/10sec, long: 200/min) providing layered protection against abuse. GqlThrottlerGuard as APP_GUARD applies these limits to both REST and GraphQL.

---

### Question 4 (Category: cross_cutting_concerns)

How does the application handle feature flag changes in real-time?

Expected Answer: On the backend, FeatureFlagsService initializes the LaunchDarkly SDK with LaunchDarkly.init() and waits for initialization via waitForInitialization(). There is no explicit streaming connection handling or flag change subscription implemented in the service - it simply evaluates flags on-demand using client.variation(). On the frontend, the mock client (used when LAUNCHDARKLY_CLIENT_ID is not set) supports 'change' event subscriptions via an eventListeners Map, emitting events when identify() is called. The identifyFeatureFlagUser() function dispatches a 'featureflag:identified' CustomEvent via window.dispatchEvent(). Cache clearing in development clears localStorage keys starting with 'ld:' on initialization. There is no periodic polling implemented in the code.

---

### Question 5 (Category: service_relationships)

What is the complete error logging flow when AndroidSyncProcessor encounters a processing failure?

Expected Answer: Error flow: 1) process() catches error, logs via this.logger.error() with job ID and context; 2) Throws error for BullMQ retry handling; 3) @OnWorkerEvent('failed') handler logs detailed failure with job data; 4) DeviceSyncErrorService persists errors to database; 5) For specific errors (DuplicateBarrelException, LocationAlreadyOccupiedException), AndroidSyncErrorLoggerService logs with appropriate severity level. BullMQ handles: retry with backoff based on processor options, moves to failed queue after maxStalledCount. This multi-layer logging ensures: immediate console visibility, persistent database record, automated retry, failed job visibility for investigation.

---

### Question 6 (Category: service_relationships)

How does the FeatureFlagsService integrate with BarrelEventService to control barrel reentry behavior?

Expected Answer: BarrelEventService injects FeatureFlagsService. The FEATURE_FLAG_KEYS constant defines flags like BarrelReentryMode. During barrel event processing, the service can check featureFlags.isEnabled() or getValue() to determine behavior. For example, BarrelReentryMode may control whether: a barrel can be re-entered at a different location without explicit withdrawal, validation strictness for duplicate entries, automatic conflict resolution strategies. Feature flags enable: runtime behavior changes without deployment, A/B testing of new features, gradual rollout of changes. The service reads from database or external configuration.

---

### Question 7 (Category: service_relationships)

What services does BarrelEventService depend on for handling barrel entry events at holding locations?

Expected Answer: BarrelEventService constructor injects: 1) PrismaService - database operations; 2) AndroidSyncErrorLoggerService (errorLogger) - logs warnings/errors to DeviceSyncError table; 3) LotVariationService - normalizes serial numbers using lot variations; 4) FeatureFlagsService - controls feature behavior like BarrelReentryMode; 5) CachedEventTypeService - avoids in-transaction EventType queries. In createHoldingLocationEntry(): finds holding location, normalizes serial via lotVariationService.findLotByNumberOrVariation(), finds/creates barrel via createBarrelByBruteforce(), updates barrel location, creates event. Error logging happens outside transaction to prevent deadlocks.

---

### Question 8 (Category: architecture_structure)

What is the purpose of the BarrelAggregatesModule and how does it optimize dashboard performance?

Expected Answer: BarrelAggregatesModule provides server-side aggregation queries that execute ON-DEMAND, not pre-computed statistics. The module uses efficient PostgreSQL GROUP BY queries with direct SQL and chunked processing (ID_CHUNK_SIZE = 5000) to prevent memory issues. Endpoints include: barrelCountBySize, barrelCountByPeriod, barrelCountByOem, barrelCountByRecipe, inventoryByRecipe, and inventoryAggregates. Results are computed at query time with proper authorization (non-privileged users must provide customer scoping). This improves dashboard performance by pushing aggregation to the database rather than loading millions of records client-side.

---

### Question 9 (Category: cross_cutting_concerns)

How does the resolution service integrate with printer automation for label printing?

Expected Answer: The integrity tracking system monitors data quality and consistency metrics, NOT cryptographic tamper detection. It tracks: missing required fields, orphaned records, referential integrity violations, and data anomalies. There are no cryptographic hashes, Merkle trees, or blockchain-style integrity verification. The 'integrity' refers to database referential integrity and data quality.

---

### Question 10 (Category: architecture_structure)

How does the trust-mode module enable high-throughput inventory operations?

Expected Answer: The trust-mode module does NOT enable high-throughput inventory operations through authorization checks. Instead, it implements a cutoff-date-based filtering mechanism for Android scan records. Purpose: Allows operators to set a cutoff date and only process AndroidSyncInbox records created before that date. Implementation: Uses Redis for distributed caching (24-hour TTL) with in-memory fallback. Key Methods: shouldProcessAndroidSyncRecord() checks if a record's creation date is before the cutoff. High-throughput enablement: By skipping records after the cutoff date, it reduces processing volume for faster inventory syncing during specific time windows. Not authorization-based: No caller/permission checking - it's purely temporal filtering.

---

### Question 11 (Category: cross_cutting_concerns)

How does the feature flag guard pattern work for conditionally enabling features?

Expected Answer: The McpFeatureFlagGuard demonstrates the pattern for feature-flag-controlled features. It implements CanActivate and injects FeatureFlagsService. In canActivate(), it first checks for local development override (MCP_SERVER_ENABLED env var in non-production). Then it evaluates the LaunchDarkly flag using evaluateBoolean() with a system context. If the flag is disabled, it throws HttpException with 404 status, making the feature appear non-existent. This pattern allows features to be toggled without code deployment. The guard runs BEFORE authentication, ensuring clean 404 responses even for unauthenticated requests. Multiple feature guards can be composed using @UseGuards().

---

### Question 12 (Category: cross_cutting_concerns)

How does the resolution system coordinate with the serial registry service?

Expected Answer: ResolutionFunctionParams includes serialRegistryService as an optional injected dependency for serial number management. However, the answer incorrectly describes the specific functionality. The actual SerialRegistryService provides: 1) register() - to register new serial numbers with conflict detection, 2) checkExists() - O(1) hash-based existence check, 3) linkToBarrel() - to link registered serials to barrels, 4) registerWithOverride() - for exceptional duplicate cases. The BarrelMatchingService.ensureUniqueSerialNumber() checks if a suggested serial exists in the Barrel table (not serial registry) and generates a new semantic serial if it does. It does NOT coordinate with SerialRegistryService - it uses the Barrel table directly via tx.barrel.findFirst(). The registry does NOT track serial usage patterns for analytics and gap detection.

---

### Question 13 (Category: data_flow_integration)

How does the GraphQL subscription system propagate comment updates to connected clients? Describe the PubSub pattern and filtering mechanism.

Expected Answer: Comment subscription flow: 1) PubSubService wraps graphql-subscriptions PubSub, provides publish() and asyncIterableIterator(), 2) CommentResolver defines @Subscription with filter function checking entityType and entityId match, 3) When comment created/updated, service calls pubSub.publish('commentAdded', payload), 4) Subscription filter evaluates: payload.entityType === variables.entityType && payload.entityId === variables.entityId, 5) Only matching subscribers receive the update, 6) commentUpdated, commentDeleted follow same pattern, 7) mentionedInComment subscription filters by payload.mentionedUserIds?.includes(variables.userId), 8) resolve function transforms payload before sending to client.

---

### Question 14 (Category: architecture_structure)

How does the ACLModule (Access Control List) work with the RolesModule for authorization?

Expected Answer: ACLModule (acl.module.ts) is a simple module that uses AccessControlModule.forRoles() from 'nest-access-control' library, initializing a RolesBuilder with grants from grants.json. The grants.json file defines permissions as {role, resource, action, attributes} entries (e.g., {role:'administrator', resource:'User', action:'read:own', attributes:'*'}). RolesModule (roles.module.ts) simply provides and exports the RolesBuilder class - it does NOT manage user-role assignments. Resolvers use @nestAccessControl.UseRoles decorator with {resource, action, possession} to declare required permissions. GqlACGuard extends ACGuard from nest-access-control and overrides getUser() to extract the user from GraphQL context. The separation is minimal: ACLModule configures permissions, RolesModule makes RolesBuilder injectable, and GqlACGuard validates permissions against user roles.

---

### Question 15 (Category: data_flow_integration)

Trace the data flow for Redis diagnostics during inventory processing. How are connection health issues detected and handled?

Expected Answer: Redis diagnostics flow: 1) RedisDiagnosticsService performs periodic health checks, 2) getLastHealthCheck() returns: connectionStatus, lastError, latency metrics, 3) In processInventorySession(), at job start: check redisHealth, 4) If connectionStatus !== 'connected': log WARN via databaseProgressService.logProcessingEvent(), 5) Health check details included in log: status, lastError, timestamp, 6) Processing continues (non-blocking) but issue tracked, 7) BullMQ reconnectOnError handles: READONLY, ECONNRESET, ETIMEDOUT with auto-reconnect, 8) At job end: finalHealth logged for correlation, 9) Used for post-mortem analysis of processing issues, 10) Separate from BullMQ's internal connection management.

---

### Question 16 (Category: business_logic_constraints)

How does the system track lot-level vs barrel-level transactions?

Expected Answer: GroupTransaction supports two tracking modes via TransactionTrackingMode enum: 1) BARREL_LEVEL (default) - affectedBarrelIds array populated with individual barrel IDs. BarrelEvent created for each barrel. Used for barrel-specific operations. 2) LOT_LEVEL - affectedBarrelIds is empty ([]). Uses CreateLotLevelTransferInput with quantity and unitType from FinancialUnitType enum (PROOF_GALLONS, WINE_GALLONS, BARRELS, COUNT, OTHER). LotLevelTransactionMetadata (interface in service, not a DB model) stores trackingMode, lotNumber, quantityChange, unitType, beforeQuantities, afterQuantities. The GroupTransaction model stores lotQuantityUnits and lotQuantityUnitType directly. Both modes create FinancialTransaction records for accounting.

---

### Question 17 (Category: architecture_structure)

How does the frontend lib/graphql directory organize queries and support code generation?

Expected Answer: The lib/graphql directory contains hand-written query files (barrel-queries.ts, reconciliation.ts, bulkBarrelOperations.ts, etc.) alongside .generated.ts files produced by graphql-codegen. The codegen.yml configuration scans .ts files in lib/graphql/, hooks/, components/, and app/ directories for GraphQL operations (excluding .generated.ts files). It generates: 1) A central generated.ts with TypeScript types, operation types, and React Apollo hooks, and 2) Individual .generated.ts files next to source files using the near-operation-file preset. The configuration uses typescript, typescript-operations, and typescript-react-apollo plugins. This ensures type safety between frontend queries and the backend schema.graphql file.

---

### Question 18 (Category: architecture_structure)

How does the WarehouseJobsModule demonstrate circular dependency resolution with BarrelModule?

Expected Answer: WarehouseJobsModule uses forwardRef(() => BarrelModule) to defer module resolution, allowing warehouse jobs to query/update barrels. However, BarrelModule does NOT have a reverse forwardRef to WarehouseJobsModule - the circular dependency mentioned in the original answer is not bidirectional. BarrelModule uses forwardRef for AuthModule and BarrelOwnershipModule instead. The enum import pattern is correct: './base/warehouse-jobs.enums' is imported separately to ensure GraphQL enum types are registered before resolvers.

---

### Question 19 (Category: architecture_structure)

How does the barrel aggregate calculation in the resolver ensure memory efficiency for large datasets?

Expected Answer: BarrelResolver.calculateBarrelAggregates uses Prisma aggregation queries (findMany with distinct, aggregate) instead of loading all records. For unique counts, it selects only the distinct field values. For age statistics, it uses raw SQL (EXTRACT/FLOOR/AVG) to calculate average age at the database level. This avoids loading thousands of barrels into memory just to count unique values or calculate averages, making it scale to datasets of any size.

---

### Question 20 (Category: service_relationships)

What queue infrastructure does AndroidSyncProcessor use to route InventoryCheck events to the staging queue?

Expected Answer: AndroidSyncProcessor injects the staging queue via @InjectQueue('inventory-check-staging') private readonly stagingQueue: Queue<StagingJobData>. When processing a record, it checks if the associated warehouseJob.requiresPreReconciliation is true AND the event type is 'InventoryCheck'. If both conditions are met, instead of direct processing, it creates a staging job: stagingQueue.add('process-to-staging', {jobId, deviceName, inboxId}). The InventoryCheckStagingProcessor then handles these jobs separately. This routing enables: pre-reconciliation workflow for inventory counts, separation of direct events (Entry, Withdrawal) from staging events, and dedicated concurrency/error handling for staging operations.

---

### Question 21 (Category: service_relationships)

What services subscribe to barrel events and how do they respond in the event-driven architecture?

Expected Answer: The codebase uses BullMQ queues rather than NestJS EventEmitter for event distribution. Key event consumers: 1) AndroidSyncProcessor (android-sync-processing queue) - processes barrel scan events, creates barrel events; 2) InventoryProcessingProcessor (inventory-processing queue) - processes CSV upload rows; 3) InventoryCheckStagingProcessor (inventory-check-staging queue) - handles pre-reconciliation staging. Each processor uses @Processor decorator with options (concurrency, lockDuration, stalledInterval). When BarrelEventService creates events, downstream effects are handled synchronously in the transaction. For async effects like MES sync, LotService publishes to RabbitMQ. The PubSubService handles GraphQL subscriptions for UI updates.

---

### Question 22 (Category: business_logic_constraints)

How does the bulk barrel operations system work?

Expected Answer: bulk-update-barrel.dto.ts defines fields for bulk barrel operations: barrelIds (array of barrel IDs to update), locationId (optional new location), status (optional new EnumBarrelStatus), dispositionId (optional new disposition), notes (optional update notes), and performedBy (user performing the update). The DTO uses class-validator decorators for validation: @IsArray(), @IsUUID() for barrelIds, @IsOptional() for optional fields. Bulk updates are processed in a transaction to ensure atomicity.

---

### Question 23 (Category: data_flow_integration)

How does the serial number format validation work? What patterns are considered valid?

Expected Answer: Serial validation flow: 1) validateSerialNumberFormat() in InventoryCheckStagingService, 2) Returns {valid: boolean, error?: string}, 3) Expected format: {lotNumber}.{barrelNumber} e.g., '25G29AB.001', 4) Split by '.', validate parts count, 5) Lot number validation: alphanumeric, specific patterns, 6) Barrel number validation: numeric, within expected range, 7) Special patterns (EMPTY_POSITION, BAD_QR, NOLABEL) handled separately, 8) If invalid AND resolveLotFromSerialNumber fails: create ERROR issue 'ValidationError', 9) requiresManualReview=true for invalid format, 10) Prevents garbage data from creating invalid barrels.

---

### Question 24 (Category: architecture_structure)

How does the frontend API routes structure support both proxy and direct backend calls?

Expected Answer: Frontend API routes in app/api/ serve two purposes: graphql-proxy/route.ts forwards authenticated GraphQL requests to the backend with token handling, while other routes (barrel-inventory/, ownership/, transfers/) make direct REST calls to backend endpoints. This hybrid approach uses GraphQL for complex queries needing selective field loading, and REST routes for simpler operations or file uploads. API routes also handle rate limiting and origin validation before forwarding.

---

### Question 25 (Category: data_flow_integration)

How does the warehouse job staging system track scan processing status? Describe the state machine and transitions.

Expected Answer: Staging state machine: 1) Initial state: record created in InventoryCheckStaging with status PROCESSING, processingStartedAt set, 2) States: PROCESSING (validation in progress), READY (validation complete, awaiting review), APPROVED (human approved), COMMITTED (changes applied), REJECTED, 3) Transitions: PROCESSING->READY on successful validation, 4) hasIssues flag set based on detected issues, 5) WarehouseJob stats updated: totalScans, pendingReview, issueCount, criticalIssueCount, 6) Job auto-transitions PENDING->IN_PROGRESS on first scan, 7) Stuck detection via processingStartedAt, lastHeartbeat, processingAttempts, 8) Commit flow: READY->APPROVED->COMMITTED, creates actual BarrelEvents, 9) Issues with requiresManualReview=true block approval.

---

### Question 26 (Category: service_relationships)

Explain the transaction boundary and error propagation when GroupTransactionService creates a lot entry with barrel verification.

Expected Answer: In createLotEntry(), the entire operation is wrapped in prisma.$transaction(). The transaction boundary includes: 1) Lot existence verification (tx.lot.findUnique); 2) Barrel existence verification (tx.barrel.findMany); 3) GroupTransaction creation (tx.groupTransaction.create). Error propagation: If lot not found, BadRequestException is thrown, causing transaction rollback. If barrels are missing, the code identifies missing IDs and throws BadRequestException with details. Any database error during create causes automatic rollback. The transaction uses Prisma's default isolation level (ReadCommitted). Services involved: GroupTransactionService coordinates with PrismaService for all DB operations. This ensures atomic operations - either all verifications pass and the group transaction is created, or nothing is persisted.

---

### Question 27 (Category: cross_cutting_concerns)

How does the application handle authentication token refresh and expiration?

Expected Answer: The application does NOT implement server-side token refresh. Azure AD token refresh is handled entirely client-side by MSAL.js. The backend only validates incoming tokens - if a token is expired, the request fails with 401 and the client must handle refresh. There is no token refresh endpoint or server-side refresh logic.

---

### Question 28 (Category: cross_cutting_concerns)

How are metrics, logs, and traces collected and what can be monitored?

Expected Answer: The application does NOT implement distributed tracing. There is no OpenTelemetry, Jaeger, or similar tracing infrastructure. Request correlation is handled via correlationId generated per-request in ContextMiddleware and passed through services via AsyncLocalStorage. AuditService logs include correlationId for linking related operations, but this is not distributed tracing - it's request-scoped correlation within a single service instance.

---

### Question 29 (Category: cross_cutting_concerns)

How does the audit log query system support complex filtering and pagination?

Expected Answer: The AuditService provides AuditLogQueryOptions supporting filters: operationType (single or array), entityType (single or array), entityId (single or array), userId (single or array), sessionId, uploadSessionId, correlationId, dateFrom, dateTo, and operationResult. Pagination is handled via skip/take with orderBy options. The queryAuditLogs() method builds Prisma where clauses dynamically based on provided options. Array values become 'in' clauses. Date ranges use 'gte'/'lte'. The service also supports getAuditLogsByCorrelationId() to trace related operations and getEntityHistory() to see all changes to a specific entity over time.

---

### Question 30 (Category: cross_cutting_concerns)

How does the resolution system ensure idempotency for resolution operations?

Expected Answer: Idempotency is ensured through: 1) Pre-flight validation checking if errors are already RESOLVED - duplicate resolution attempts throw clear errors listing already-resolved IDs, 2) Database constraints preventing duplicate resolution records, 3) Prisma transactions ensuring atomic updates - partial failures rollback completely, 4) Status transitions are one-way (UNRESOLVED -> RESOLVED), preventing state oscillation. The resolution record stores the unique combination of error IDs and resolution context. Re-submitting the same resolution request fails with 'already resolved' error rather than creating duplicates. Post-resolution verification confirms status changes persisted.

---

### Question 31 (Category: business_logic_constraints)

What is the purpose of validFrom/validTo in barrel ownership and how does it enable historical queries?

Expected Answer: BarrelOwnership uses validFrom/validTo for temporal modeling: validFrom = when ownership became active (default: creation time), validTo = when ownership ended (null = current). This enables: 1) Current ownership query: WHERE validTo IS NULL. 2) Historical ownership at date: WHERE validFrom <= date AND (validTo IS NULL OR validTo > date). 3) Ownership audit: full history preserved, never deleted. When transferOwnership() occurs, current record gets validTo = now, new record created with validFrom = now. This temporal pattern supports regulatory requirements for ownership history and enables point-in-time reporting.

---

### Question 32 (Category: service_relationships)

How does OwnershipTransactionService ensure ledger entries are created atomically with ownership transactions?

Expected Answer: In createOwnershipTransaction(), the entire operation is wrapped in prisma.$transaction(async (tx) => {...}). Within this transaction: 1) Creates the OwnershipTransaction record via tx.ownershipTransaction.create(); 2) Immediately calls createOwnershipLedgerEntries(tx, ownershipTransaction, {...}) passing the transaction client. The createOwnershipLedgerEntries utility function uses the same transaction client (tx) to create corresponding ledger entries. If either operation fails, the entire transaction rolls back. If a select clause was provided, a final findUnique is done to return only requested fields. This ensures: ledger entries always match transactions, no orphaned records on failure, and accounting consistency.

---

### Question 33 (Category: business_logic_constraints)

What constraints prevent overlapping barrel ownerships?

Expected Answer: The ownership system prevents overlapping active ownerships through: 1) Single owner check: When creating sole ownership, system verifies no existing active ownership (validTo IS NULL) exists. 2) Joint ownership: All joint owners must be created atomically with single jointOwnershipGroupId, percentages summing to 100%. 3) Transfer operations: Old ownership's validTo is set before new ownership created. 4) Aggregate validation: createOwnership() checks currentOwnershipTotal + newPercentage <= 100 within transaction. This ensures at any point in time, total ownership percentages for a barrel equal exactly 100%.

---

### Question 34 (Category: data_flow_integration)

How does the Verified event type work? Describe the data flow and payload validation.

Expected Answer: Verified event flow in handleVerifiedEvent(): 1) Extract barrelSN, locationString, operator, jsonEntityPayload from data, 2) Find barrel by serialNumber (with lot variation normalization), 3) Validate jsonEntityPayload as VerifiedPayload: @IsOptional() @IsString() verificationPurpose, 4) Lookup 'Verified' eventType, 5) Barrel location unchanged (verification is observational), 6) Create BarrelEvent with: barrelId, eventTypeId, eventTime, createdBy, notes with verificationPurpose if provided, 7) Use case: confirming barrel is at expected location, quality check completion, 8) Returns barrelEvent with current location info, 9) Lightweight event - no state change, just audit trail.

---

### Question 35 (Category: service_relationships)

What services are involved when BarrelOwnershipService creates ownership with inheritance tracking?

Expected Answer: In establishInheritance(parentCustomerId, childCustomerId, userId, notes, transferReasonId): 1) Calls getCustomerOwnerships(parentCustomerId) to get parent's active ownerships; 2) Within prisma.$transaction(): for each parent ownership, creates new BarrelOwnership with: ownershipType=InheritedChild, inheritedFromCustomerId=parentCustomerId, inheritanceDate=now; 3) Calls createOwnershipTransaction() with type=INHERITANCE for each barrel. Services: BarrelOwnershipService (main), PrismaService (transaction), implicitly OwnershipTransaction table via createOwnershipTransaction. The inheritance creates a copy of parent's portfolio under child while preserving original ownership history.

---

### Question 36 (Category: architecture_structure)

Why is PrismaModule marked as @Global() and what modules does it export?

Expected Answer: PrismaModule is marked @Global() so PrismaService is automatically available in all modules without explicit imports, reducing boilerplate. It exports PrismaService (database access) and ContextModule (request context propagation). This global pattern is essential because virtually every feature module needs database access, and having to import PrismaModule everywhere would create excessive import statements and make the dependency graph harder to manage.

---

### Question 37 (Category: architecture_structure)

How does the frontend configuration page architecture support different setting categories?

Expected Answer: The app/dashboard/configuration directory contains subdirectories for different configuration areas: barrel-oem-codes/, customer-logos/, holding-locations/, manhour-by-day/, printer-automation/, and rickhouse/. Each has page.tsx and potentially edit/new subdirectories for CRUD operations. This organization groups related settings while the page-client.tsx pattern separates server and client components for Next.js. The modular structure allows adding new configuration areas without affecting existing ones.

---

### Question 38 (Category: cross_cutting_concerns)

How does the application validate that device sync errors are not already resolved before resolution?

Expected Answer: In ResolutionService.createResolution(), after fetching deviceSyncErrors by ID, it filters for errors where status === 'RESOLVED'. If alreadyResolvedErrors.length > 0, it throws BadRequestException listing the already-resolved error IDs. This prevents duplicate resolutions and maintains data integrity. The service logs warnings with error details for debugging. After successful resolution, it updates all device sync errors with status: 'RESOLVED' and resolutionId. It then verifies the updates by re-fetching and logging any errors that weren't properly marked. This verification catches edge cases where the update may silently fail.

---

### Question 39 (Category: cross_cutting_concerns)

How does the application handle sensitive data in error messages and logs?

Expected Answer: Several patterns protect sensitive data: 1) Error categorization in business-logic-error-handler.ts categorizes errors into types (RESOURCE_NOT_FOUND, RESOURCE_CONFLICT, PERMISSION_ERROR, VALIDATION_ERROR, DATABASE_CONSTRAINT_ERROR, EXTERNAL_SERVICE_ERROR, UNKNOWN_ERROR) but still includes detailed error messages in output. 2) AuditService.sanitizeForAudit() removes sensitive fields: password, passwordHash, salt, token, secret, apiKey, privateKey from beforeState/afterState. 3) Frontend sanitizeContextForLogging() removes PII from LaunchDarkly context by only exposing kind, key, anonymous, and environment fields. 4) The error handler uses structured logging with context objects containing errorId, errorType, resolutionOption, userId, warehouseJobId.

---

### Question 40 (Category: service_relationships)

How does BarrelService.buildUnifiedDirectWhere implement default filtering for removed barrels?

Expected Answer: buildUnifiedDirectWhere(filters): 1) If !filters.includeRemovedBarrels: where.AND = [{ OR: [{ systemStatus: { not: PendingDeletion } }, { systemStatus: null }] }, { OR: [{ disposition: { notIn: [Removed, Dumped, TransferredOffsite] } }, { disposition: null }] }]; 2) Applies serial number, recipe, and other filters on top. This default behavior: excludes soft-deleted barrels (PendingDeletion), excludes removed/dumped/transferred barrels, handles null values explicitly (null is valid for non-removed). Setting includeRemovedBarrels=true skips this filter for audit/historical queries.

---

### Question 41 (Category: cross_cutting_concerns)

How does the resolution system handle partial batch failures?

Expected Answer: When resolving multiple device sync errors in a batch, the ResolutionService tracks success/failure per error. For each errorId, it executes the resolution function in a try-catch. Successes are added to succeededErrorIds array. Failures are captured with categorized error information in failedErrors array. After processing all errors, if failedErrors.length > 0, the entire transaction is rolled back - no errors are marked resolved. The exception message includes counts (failed X of Y errors, Z succeeded) and detailed failure information per error including errorId, errorMessage, and errorCategory. This all-or-nothing approach ensures data consistency but requires the user to address failures and retry the entire batch.

---

### Question 42 (Category: architecture_structure)

What is the purpose of having both HoldingLocationModule and StorageLocationModule?

Expected Answer: StorageLocationModule handles physical warehouse locations (specific positions in ricks). HoldingLocationModule manages temporary holding areas (staging areas, shipping docks, quality inspection areas) that aren't permanent storage. A barrel may move from StorageLocation to HoldingLocation during picking, then back to a different StorageLocation. This separation reflects physical warehouse workflow where barrels transition between permanent storage and temporary holds.

---

### Question 43 (Category: service_relationships)

How does the codebase prevent circular dependencies between CustomerService and TransferService?

Expected Answer: Both services inject each other using NestJS's forwardRef() pattern: In CustomerService: @Inject(forwardRef(() => TransferService)) private readonly transferService: TransferService. In LotService (similar pattern): @Inject(forwardRef(() => TransferService)) private readonly transferService?: TransferService. The forwardRef() defers the resolution of the dependency until the IoC container has fully initialized both services. Additionally, in LotService, the TransferService is marked as optional (?) with null checks before usage. This allows: nested resolvers to delegate sorting to the appropriate service, avoiding runtime circular reference errors during dependency injection.

---

### Question 44 (Category: service_relationships)

How does BarrelOwnershipService use createId from @paralleldrive/cuid2 for identifier generation?

Expected Answer: BarrelOwnershipService imports createId from @paralleldrive/cuid2. Used in: 1) createOwnershipTransaction() - generates unique transaction ID; 2) establishJointOwnership() - generates jointOwnershipGroupId; 3) bulkUpdateOwnership() - generates operationId; 4) executeBarrelTransfer() - generates ownership record ID. CUID2 provides: collision-resistant IDs, sortable by creation time, URL-safe characters, suitable for distributed systems without coordination. This replaces UUID in some cases for better sorting/indexing characteristics.

---

### Question 45 (Category: cross_cutting_concerns)

How does the error configuration support custom field requirements per resolution option?

Expected Answer: ERROR_CONFIGURATION defines requiredFields for each resolution option. ResolutionValidationUtil.getRequiredFields(errorType, resolutionOption) retrieves the specific fields needed. For example, 'RESOLVE:create9xxxBarrel' may require lotId and locationString, while 'RESOLVE:moveToCorrectLocation' requires barrelId, oldLocationId, and locationId. extractedRequiredFields are auto-populated from error data; userRequiredFields must be provided by the caller. Validation checks both categories. The configuration also includes optional fields with default values. This declarative approach allows new resolution options to be added with their requirements without code changes.

---

### Question 46 (Category: cross_cutting_concerns)

How does the resolution system handle the ESCALATE resolution path?

Expected Answer: The ERROR_CONFIGURATION does NOT have an ESCALATE resolution path. The valid allowedResolutions are: ACCEPT, RESOLVE, ACKNOWLEDGE, and REJECT. There is no ESCALATED status defined. For errors requiring team review, the configuration uses options like 'RESOLVE:elevateToEntTeam' (which calls notifyEntTeam function) or 'ACKNOWLEDGE:acknowledgeAndQueueFollowUp' (which calls createOrConnectAFollowUpJob and updateStatusInResolutionToReview functions). The subsequentJob parameter in ResolutionFunctionParams supports newJobData.assignedUserNames for team assignment and newJobData.jobType for job categorization. The status change is typically to 'REVIEW' via updateStatusInResolutionToReview, not 'ESCALATED'.

---

### Question 47 (Category: business_logic_constraints)

What validation rules prevent invalid barrel disposition transitions, and how does the BarrelDispositionLocationStateException enforce the relationship between disposition and location?

Expected Answer: The system enforces strict rules between barrel disposition and location state: 1) If a barrel has a storage location (locationId), disposition MUST be 'InStorage'. 2) If a barrel has a holding location (holdingLocationId), disposition MUST be 'InHolding' or 'TransferredOffsite'. 3) If neither location is set, disposition CANNOT be 'InStorage' or 'InHolding'. The BarrelDispositionLocationStateException in barrel.exceptions.ts throws a BAD_REQUEST error with detailed context including the attempted disposition, current location state, and allowed dispositions. This prevents barrels from having inconsistent states like being marked 'InStorage' without an actual storage location.

---

### Question 48 (Category: architecture_structure)

How does the frontend support different authentication modes (Azure AD vs demo mode)?

Expected Answer: Frontend uses MSAL_Provider.tsx which checks isDemoMode() (NEXT_PUBLIC_AUTH_MODE === 'demo'). SECURITY: isDemoMode() explicitly returns false in production environments (NODE_ENV === 'production'), blocking demo mode activation. In demo mode, it creates a mock AccountInfo and stores 'test_e2e_token' in sessionStorage. The graphql-proxy route.ts ALWAYS requires bearer tokens - it returns 401 'Authentication required' if no token is provided (line 230-235). The proxy does NOT allow passthrough - it forwards whatever token is provided (demo or real). There is no AuthContext wrapper; MSAL_Provider directly wraps children with MsalProvider. The proxy also validates token claims and can enforce Azure AD v2.0 tokens when APP_ENFORCE_AAD_V2='true'.

---

### Question 49 (Category: service_relationships)

What services are involved when TransferService performs reconciliation status calculation?

Expected Answer: In getTransfersForReconciliation(): 1) PrismaService fetches transfers with primaryGroupTransactions include; 2) For each transfer: extracts groupTransaction.affectedBarrelIds.length as currentBarrelCount; 3) Gets expectedBarrelCount from transfer.quantity; 4) Calculates reconciliationStatus: NOT_RECONCILED (currentBarrelCount === 0, regardless of expected), PARTIALLY_RECONCILED (currentBarrelCount > 0 AND currentBarrelCount < expectedBarrelCount), FULLY_RECONCILED (all other cases including when currentBarrelCount >= expectedBarrelCount); 5) needsReconciliation = currentBarrelCount === 0 && expectedBarrelCount > 0. No additional services involved - calculation is self-contained using data already fetched. This enables: reconciliation dashboard filtering, prioritization of unreconciled transfers.

---

### Question 50 (Category: service_relationships)

How does InventoryUploadService.validateCsvStructure handle header validation with BOM and encoding issues?

Expected Answer: validateCsvStructure(): 1) Sets validationTimeout (5000ms) with cleanup; 2) Converts buffer to UTF-8 string; 3) Creates stream from csv-parser; 4) On first row (isFirstRow): extracts headers via Object.keys(data); 5) Cleans headers: header.replace(/^\uFEFF/, '').trim() - removes BOM (Byte Order Mark); 6) Case-insensitive comparison: header.toLowerCase() === required.toLowerCase(); 7) Checks for missing required headers; 8) Rejects with descriptive error if missing. This handles: Excel-exported CSVs with BOM, case variations in headers, early termination after validation (maxRowsToCheck=10), timeout prevention for large files.

---

### Question 51 (Category: data_flow_integration)

What is the data flow when a location is already occupied during entry event processing?

Expected Answer: Location occupied handling: 1) In storage location processing, after location lookup, 2) checkLocationOccupied(locationId, incomingBarrelId) queries barrel with that locationId, 3) If found and id !== incomingBarrelId: location occupied, 4) For staging validation: creates WARN issue 'LocationAlreadyOccupied', context includes displacedBarrelId, displacedBarrelSN, targetLocationId, resolutionAction='DISPLACE', 5) For direct processing: creates displacement BarrelEvent with 'Displaced' type for occupying barrel, 6) Clears occupyingBarrel.locationId, 7) Then assigns location to incoming barrel, 8) Ensures location-barrel relationship is 1:1.

---

### Question 52 (Category: service_relationships)

How does GroupTransactionService validate barrel existence before creating transactions, and what error information does it provide?

Expected Answer: In createLotEntry(), createTransfer(), createInternalMove(): 1) Fetches barrels: tx.barrel.findMany({ where: { id: { in: input.barrelIds } }, select: { id, serialNumber } }); 2) Compares found count vs requested: if (barrels.length !== input.barrelIds.length); 3) Identifies missing: foundIds = barrels.map(b => b.id); missingIds = input.barrelIds.filter(id => !foundIds.includes(id)); 4) Throws: BadRequestException(`Barrels not found: ${missingIds.join(', ')}`). This provides: specific list of missing IDs for debugging, prevents partial transaction creation, fails fast before any writes. The error message enables operators to identify and resolve data issues.

---

### Question 53 (Category: architecture_structure)

Why does the BarrelOwnershipModule not use forwardRef for its imports while BarrelModule does?

Expected Answer: BarrelOwnershipModule imports PrismaModule, ACLModule, and LotVariationModule - none of which depend back on BarrelOwnership, so no circular dependency exists. However, BarrelModule imports BarrelOwnershipModule via forwardRef because there's bidirectional reference: barrels have ownership records, and ownership records reference barrels. The asymmetry exists because ownership is a supporting module without direct barrel imports, while the barrel module actively consumes ownership services.

---

### Question 54 (Category: business_logic_constraints)

What business rules govern the feedback collection system?

Expected Answer: EnumFeedbackType defines: FEATURE_REQUEST, BUG_REPORT, IMPROVEMENT (not 'Bug', 'Feature', 'Improvement'). feedback.dto.ts (CreateFeedbackRequestDto) defines feedback submission with type, title (5-200 chars), description, authorId, authorEmail, authorName, changelogEntryId, and structured fields (FeatureRequestFieldsDto, BugReportFieldsDto, ImprovementFieldsDto). Feedback collected from web interface with author identification. Feedback may trigger: changelog entries (via changelogEntryId link), linear issue creation (linearIssueId field). Anonymous feedback is NOT explicitly supported - all author fields are required.

---

### Question 55 (Category: data_flow_integration)

How does the inventory upload processing system handle large CSV files? Describe the complete data transformation pipeline from file upload to database persistence.

Expected Answer: The flow is: 1) Frontend uploads CSV to inventory-upload endpoint, 2) File is parsed and rows are inserted into InventoryUploadStaging table with status 'pending', 3) A Bull job is queued to 'inventory-processing' queue via InventoryProcessingProcessor, 4) Processor processes in batches (configurable batchSize), calling inventoryProcessingService.processBatch(), 5) SQL function processes staging records: validates data, resolves lots via LotVariationService, creates/updates barrels, 6) Progress is tracked via ProgressTrackingService and broadcast via WebSocket ProgressGateway, 7) SafetyLimitsService monitors for resource limits and circuit breaker patterns, 8) DatabaseProgressService logs batch progress and events, 9) On completion, TrustModeProcessingService handles cutoff date processing if enabled.

---

### Question 56 (Category: architecture_structure)

What is the purpose of the AggregatedStorageEntriesModule?

Expected Answer: AggregatedStorageEntriesModule provides server-side aggregation of barrel storage entries specifically for TTB compliance reporting. It solves the problem of client-side aggregation missing data from paginated barrel events. Features include: aggregation by lot number, event category (entry, withdrawal, transfer_in, dump), and configurable time period (daily, weekly, monthly); optional grouping by customer and/or whiskey type; efficient pagination at the aggregated entry level; and event IDs for drill-down functionality. The resolver provides GraphQL access via the 'aggregatedStorageEntries' query with filter, pagination, and sort arguments.

---

### Question 57 (Category: service_relationships)

What is the complete flow of services when a barrel is created through inventory upload processing?

Expected Answer: Complete flow: 1) InventoryUploadService.createUploadSession() creates session; 2) Controller saves file, calls queueProcessing() to add job to 'inventory-processing' queue; 3) InventoryProcessingProcessor dequeues and calls processInventorySession(); 4) DataValidationService.validateSession() performs comprehensive validation; 5) processBatch() calls PostgreSQL function process_inventory_batch() which handles barrel creation/updates internally in the database; 6) BarrelProcessingService.processBarrelsForSession() orchestrates via SQL function process_barrel_operations(); 7) ProgressTrackingService tracks session progress; 8) AuditService logs operations at each phase; 9) TrustModeProcessingService handles trust mode if enabled; 10) BarrelEventHelperService creates BarrelEvents for operations. Note: Barrel creation happens via SQL functions, not direct BarrelService.createBarrelWithOwnership() calls. The MicroBatchOrchestrator is a separate processing pathway.

---

### Question 58 (Category: architecture_structure)

How does the frontend chatbot component architecture support AI-powered warehouse assistance?

Expected Answer: The components/chatbot directory has an index.ts barrel file and shared/ subdirectory for common components. The dashboard/chat page contains ChatMainPanel.tsx (conversation UI) and ConversationSidebar.tsx (history). Hooks useChatSession.ts and useChatHistory.ts manage state. This connects to backend ChatHistoryModule for persistence and AgentModule for AI processing. The architecture supports streaming responses, tool use, and conversation context management for warehouse-specific queries.

---

### Question 59 (Category: data_flow_integration)

Trace the data flow when a frontend user queries barrels with pagination. How does the GraphQL resolver interact with Prisma to return paginated results?

Expected Answer: Pagination flow: 1) Frontend sends GraphQL query with first/after or last/before cursor arguments, 2) BarrelResolver receives args via @Args(), 3) Resolver calls BarrelService method which builds Prisma findMany query, 4) Prisma query includes: where clause from filters, orderBy, take (first+1 to check hasNextPage), cursor (decoded from after/before), skip (1 if cursor provided), 5) Service transforms results to Connection type with edges (node + cursor) and pageInfo (hasNextPage, hasPreviousPage, startCursor, endCursor), 6) Cursor is typically base64-encoded ID, 7) Total count fetched separately if needed via count query, 8) DataLoader may batch related entity fetches to prevent N+1.

---

### Question 60 (Category: service_relationships)

How does the PubSubService enable real-time subscriptions across the application?

Expected Answer: PubSubService wraps graphql-subscriptions PubSub: 1) publish(trigger, payload) - broadcasts event to all subscribers of trigger; 2) asyncIterableIterator(triggers) - returns async iterator for GraphQL subscriptions. Services like AuditService call pubSubService.publish('auditLogAdded', {...}) after creating records. GraphQL resolvers use @Subscription decorator with pubSubService.asyncIterableIterator('auditLogAdded'). Pattern: service creates entity -> publishes event -> subscription resolver receives -> pushes to connected clients. This enables: live dashboard updates, real-time notifications, event-driven UI without polling.

---

### Question 61 (Category: data_flow_integration)

Trace the data flow for resolving canonical lot from a lot variation serial number.

Expected Answer: Canonical lot resolution: 1) resolveLotFromSerialNumber() in staging service, 2) Parse serial: extract potential lot code from first parts, 3) Call LotVariationService.findLotByNumberOrVariation(potentialLotCode), 4) Service queries: Lot where lotNumber matches OR LotVariation where variationNumber matches, 5) If found via variation: return canonical Lot (the parent), 6) If found directly: return Lot, 7) If not found: return {success: false, error: 'Lot not found'}, 8) Canonical lot has: id, lotNumber (canonical), customer relation, 9) Used for: barrel creation, duplicate detection, ownership resolution.

---

### Question 62 (Category: data_flow_integration)

How does the session validation work before processing a batch?

Expected Answer: Session validation flow: 1) inventoryProcessingService.validateSessionExists(sessionId), 2) Queries InventoryUploadSession by id, 3) Validates: session exists, status is appropriate for processing, 4) Returns session object with metadata: totalRows, uploadedBy, trustModeEnabled, etc., 5) If not found or invalid status: throws error, job fails, 6) Called at start of each batch (continuation jobs revalidate), 7) Prevents processing of: deleted sessions, already completed sessions, cancelled sessions, 8) Session object used for Trust Mode check at completion.

---

### Question 63 (Category: service_relationships)

What is the relationship between BarrelOwnershipService and GroupTransaction for ownership transfer tracking?

Expected Answer: In bulk transfer methods (bulkTransferOwnership, bulkUpdateOwnership, bulkEstablishJointOwnership): 1) Creates GroupTransaction via createTransactionGroup() with type OWNERSHIP_TRANSFER or ADJUSTMENT; 2) GroupTransaction stores: executedBy, executionDate, notes, metadata with filter criteria, and affectedBarrelIds (initially empty); 3) Individual transfers reference groupId in createOwnershipTransaction() which creates OwnershipTransaction audit records; 4) After processing, the relationship is maintained through the groupId reference in individual transactions. This enables: grouping related transfers for audit, rollback capability, reporting on bulk operations, correlation between OwnershipTransaction records and GroupTransaction. Note: totalBarrels field was removed from GroupTransaction model.

---

### Question 64 (Category: data_flow_integration)

What is the complete flow when a withdrawal/transfer-out event is processed? How does the barrel disposition change?

Expected Answer: Withdrawal flow in handleWithdrawalOrTransferOutEvent(): 1) Extract barrelSN, locationString, operator from data, 2) Find barrel by serialNumber (with lot variation normalization), 3) If barrel not found, throw NotFoundException, 4) Validate withdrawal payload if present (class-validator), 5) Clear barrel's locationId AND holdingLocationId (exits warehouse), 6) Set disposition to 'Withdrawn' or 'TransferredOut', 7) Lookup 'Withdrawal' eventType by name, 8) Create BarrelEvent with: barrelId, eventTypeId, eventTime, createdBy, notes from jsonEntityPayload, 9) For TransferOut, same flow but different disposition, 10) Returns created barrelEvent, storageLocation undefined (barrel no longer at location).

---

### Question 65 (Category: service_relationships)

How does the AndroidSyncProcessor service coordinate with multiple cached services during record processing, and why is cache warming performed at module initialization?

Expected Answer: AndroidSyncProcessor depends on: CachedEventTypeService, CachedEventReasonService, CachedLotVariationService, CachedStorageLocationService. Cache warming in onModuleInit() calls warmAllCaches() which uses Promise.all() to parallelize: cachedEventTypeService.warmCache(), cachedEventReasonService.warmCache(), cachedLotVariationService.warmCache(100), cachedStorageLocationService.warmCache('WHK01'). This is done because: 1) BullMQ processes jobs with high concurrency (2 concurrent jobs); 2) Without pre-warming, the first job would trigger cache-miss database queries causing latency; 3) Reference data (event types, reasons, locations) is frequently accessed but rarely changes, making it ideal for caching; 4) Pre-warming ensures consistent low-latency processing regardless of which job runs first.

---

### Question 66 (Category: architecture_structure)

How does the barrel resolver handle primary owner filtering when ownership is stored in a separate table?

Expected Answer: BarrelResolver's buildUnifiedWhere method handles ownership filters (barrel_customercode, barrel_customerid) by querying BarrelOwnership for active ownerships where validTo IS NULL AND validFrom <= current date. Matching barrel IDs are pre-resolved and returned as preResolvedBarrelIds for use in an 'IN' filter. Note: Primary owner filters (primaryOwnerCode, primaryOwnerName) are handled separately via the applyPrimaryOwnerFilter utility, which calls BarrelService.getPrimaryOwnerBarrelIds - a raw SQL query that finds barrels where the specified customer has the highest ownership percentage among active owners.

---

### Question 67 (Category: architecture_structure)

How does the inventory-upload module architecture support high-volume CSV processing?

Expected Answer: InventoryUploadModule imports BullModule from @nestjs/bull (not BullMQModule) for async job processing and contains specialized services: CsvParsingService (parsing), DataValidationService (validation), PostgreSQLCopyService (bulk inserts), MicroBatchOrchestrator (chunking), ProgressTrackingService (status updates), and SessionManagementService (upload sessions). It also includes ProgressGateway for WebSocket progress streaming to clients. This decomposition allows parallel processing, progress streaming via WebSocket, and graceful handling of large files without blocking the main event loop.

---

### Question 68 (Category: service_relationships)

What is the relationship between SyncService and PrismaService for handling external entity synchronization?

Expected Answer: SyncService injects PrismaService and creates a prismaEntityMap that maps entity names to Prisma delegates (this.prisma.item, this.prisma.customer, etc.). During upsertEntity(), SyncService: 1) Uses handleNestedRelations() to parse Prisma schema and convert nested relations to Prisma format (connect, create); 2) Validates payload using entityCreateInputMap/entityUpdateInputMap DTOs; 3) Calls the appropriate prisma delegate's create/update/upsert. This enables: external systems to send data in their format, automatic relation handling, and validation against defined schemas before persistence.

---

### Question 69 (Category: service_relationships)

What services does the FinanceReportProcessor depend on for scheduled report generation?

Expected Answer: FinanceReportProcessor (finance-reports queue) injects FinanceReportSchedulerService. The scheduler service depends on: 1) FinanceService - for point-in-time inventory and transaction queries; 2) FinanceExportService - for generating report files (CSV, PDF); 3) FinanceCacheService - for caching expensive queries; 4) AzureEmailService or NotificationService - for sending reports; 5) PrismaService - for storing report metadata. The processor handles BullMQ jobs for scheduled reports, calling scheduler.generateReport() or similar. This architecture enables: scheduled reports via cron, on-demand generation via queue, background processing without blocking API.

---

### Question 70 (Category: cross_cutting_concerns)

How does the error resolution system validate resolution data and what validation layers exist?

Expected Answer: Resolution validation occurs in multiple layers: 1) FieldExtractionUtil.extractFieldsFromDeviceSyncErrors() auto-extracts common fields from errors. 2) ResolutionValidationUtil.getExtractedRequiredFields() determines which fields must be extracted. 3) validateExtractedRequiredFields() checks extracted fields are present. 4) validateResolutionData() validates user-provided data against error type and resolution option requirements. 5) Each resolution function can have additional business logic validation. The system supports error-type-specific required fields (e.g., barrelId for BARREL_MISMATCH, locationId for location errors). Missing field errors include enhanced messages via createMissingFieldsErrorMessage() showing which fields are needed and their purpose.

---

### Question 71 (Category: cross_cutting_concerns)

How does the application ensure consistent error responses across REST and GraphQL?

Expected Answer: The HttpExceptionFilter (HttpExceptions.filter.ts) is specifically a Prisma exception filter that catches PrismaClientKnownRequestError and PrismaClientValidationError, NOT a general HTTP exceptions filter. For REST, it uses response.status(statusCode).send(errorResponse). The response format is { message, statusCode } without timestamp or path fields. For GraphQL, it throws HttpException with the original exception as cause. The business-logic-error-handler.ts provides error categorization specifically for resolution operations (RESOURCE_NOT_FOUND, RESOURCE_CONFLICT, PERMISSION_ERROR, VALIDATION_ERROR, DATABASE_CONSTRAINT_ERROR, EXTERNAL_SERVICE_ERROR, UNKNOWN_ERROR) and throws BadRequestException for single-error scenarios. It does not provide general REST/GraphQL error consistency. GraphQL errors are handled separately through Apollo Server's error formatting.

---

### Question 72 (Category: architecture_structure)

What is the architecture of the reconciliation feature across frontend and backend?

Expected Answer: Backend ReconciliationModule handles inventory reconciliation logic (comparing expected vs actual counts). Frontend has components/reconciliation/ with UI components and lib/graphql/reconciliation.ts for queries. The reconciliation workflow involves: creating reconciliation sessions, staging scanned barrels, comparing against expected inventory, and resolving discrepancies. This architecture supports warehouse audit processes where physical counts are matched against system records.

---

### Question 73 (Category: cross_cutting_concerns)

How does the ACL response filtering interceptor sanitize outgoing data?

Expected Answer: The AclFilterResponseInterceptor processes response data after handler execution. It extracts the user's roles from the request and the resource type being accessed. Using accesscontrol library, it gets the Permission object for read operations on that resource. The interceptor applies permission.filter() to the response data, removing any fields the user's role is not allowed to see. For arrays, it maps over each item applying the filter. The structuredClone() is necessary because permission.filter() doesn't handle objects with null prototypes. This ensures sensitive fields (like internal IDs, system metadata) are stripped from responses based on the user's role.

---

### Question 74 (Category: cross_cutting_concerns)

How does the system handle role-based attribute filtering for different entity types?

Expected Answer: The ACL system uses accesscontrol library to define granular permissions. For each role and entity type, permissions specify which attributes can be read/written. The Permission.filter() method returns a new object containing only allowed attributes. Different roles may see different fields of the same entity (e.g., admins see all fields, operators see subset). The aclFilterResponse.interceptor.ts applies this after handler execution. For nested objects and relations, permissions cascade - if a user can't see a field, nested objects under that field are also hidden. The ABAC (Attribute-Based Access Control) approach allows fine-grained control beyond simple role-based access.

---

### Question 75 (Category: business_logic_constraints)

What validation ensures storage location capacity is not exceeded?

Expected Answer: httpResponseLackofPositionsAvailableException throws when 'Not enough positions available!' This occurs when attempting to place barrels in a location that's already full. StorageLocation has position field indicating capacity. Barrel placement checks available positions before assignment. The exception uses BAD_REQUEST status. This prevents: overfilling storage locations, unsafe stacking, inventory tracking errors. Palletized vs Rick warehouses have different capacity calculations based on warehouseType.

---

### Question 76 (Category: service_relationships)

How does FinanceService use Prisma's periodBreakdownEventInclude for efficient data loading?

Expected Answer: periodBreakdownEventInclude uses Prisma.validator<Prisma.BarrelEventInclude>() for type-safe include definition: includes barrel.barrelOwnerships (latest ownership record with customer id, ordered by createdAt desc, take 1), barrel.location.warehouse, eventType.name, groupTransaction.transactionType. This precise include: 1) Avoids over-fetching by selecting specific fields; 2) Gets nested data in single query via Prisma's join optimization; 3) Provides data needed for categorizeTransaction() which uses eventType.name, groupTransaction.transactionType, and WarehouseJobs.jobType; 4) Type is inferred via Prisma.BarrelEventGetPayload<{ include: typeof periodBreakdownEventInclude }>. Used in period breakdown queries at line 1260 for finance reports.

---

### Question 77 (Category: architecture_structure)

How does the frontend useAgentSocket hook support real-time AI interactions?

Expected Answer: useAgentSocket hook manages WebSocket connections to the AI agent service for real-time, streaming responses. Unlike regular GraphQL queries, agent interactions may take time and stream partial results. The hook handles: connection lifecycle, message streaming, reconnection on failure, and state synchronization. Combined with ChatMainPanel, this enables conversational AI with typing indicators and progressive response display.

---

### Question 78 (Category: service_relationships)

What is the complete service coordination when ReconciliationService creates an approval request?

Expected Answer: In createReconciliation() when input.requiresApproval: 1) After creating ReconciliationRecord, calls the private createApprovalRequest() method (within ReconciliationService, lines 1536-1558) with parameters { reconciliationRecordId, requestType: 'RECONCILIATION', title, description, requestedBy, priority, estimatedImpact, requestData }; 2) createApprovalRequest() creates an ApprovalRequest record via prisma.approvalRequest.create() linked to the ReconciliationRecord; 3) AuditService logs the reconciliation record creation. Note: NotificationService is NOT involved - approvers are not notified automatically. The approval workflow then involves: approvers reviewing via UI, ApprovalRequest status updates, and final resolution updating ReconciliationRecord. Services involved: ReconciliationService (contains createApprovalRequest as private method), AuditService.

---

### Question 79 (Category: service_relationships)

What is the role of BarrelEventService.extractRelevantLocation in location string parsing?

Expected Answer: extractRelevantLocation(locationString): 1) Splits by '-': parts = locationString.split('-'); 2) Validates length >= 7 (full format expected); 3) Ignores first two pieces (typically org/warehouse identifiers); 4) Returns remaining: parts.slice(2).join('-'). Example: '01-WHK01-01-1-061-A-1' -> '01-1-061-A-1' (floor-tier-position-rick-number). This standardization enables: consistent location parsing regardless of leading identifiers, compatibility with different location string formats from devices, separation of routing (first pieces) from physical location (remaining).

---

### Question 80 (Category: service_relationships)

How does FinanceService coordinate with BarrelAuditService and LotAuditService to provide point-in-time inventory snapshots?

Expected Answer: FinanceService injects BarrelAuditService and LotAuditService. In getPointInTimeInventory(): 1) Calls barrelAuditService.getInventoryAggregatesAtDate() for summary data (total count, customer summary, warehouse summary); 2) Calls barrelAuditService.getInventoryAtDate() for detailed barrel data with pagination. BarrelAuditService uses complex raw SQL queries with CTEs (RankedAudits, LatestAudits) to reconstruct inventory state at any historical date by finding the latest audit record per barrel before the target date. LotAuditService is available for lot-level historical queries. This enables regulatory compliance by providing accurate historical inventory without modifying the original data, using the audit trail as the source of truth.

---

### Question 81 (Category: service_relationships)

How does the ReconciliationService calculate impact level from discrepancy details?

Expected Answer: ReconciliationService has calculateImpactLevel(discrepancies: DiscrepancyDetail[]) method that evaluates: discrepancies array where each item has { field, systemValue, actualValue, severity: 'HIGH'|'MEDIUM'|'LOW', description }. The calculation counts HIGH severity discrepancies to determine EnumImpactLevel. In createReconciliation(), the result is stored as impactLevel on the ReconciliationRecord. This enables: prioritization of reconciliation work, filtering by impact in UI, automated escalation based on impact level. The separation of severity (per-field) and impact (overall) allows nuanced assessment.

---

### Question 82 (Category: architecture_structure)

How does the McpModule (Model Context Protocol) integrate AI capabilities into the warehouse system?

Expected Answer: McpModule implements the Model Context Protocol by creating an MCP server that registers and exposes 9 specialized tools for AI agents to interact with warehouse data and operations. The module exports McpService, which implements: (1) Database tools - ping, get_schema, run_query for direct database access, (2) GraphQL tools - get_graphql_schema and run_graphql for querying the in-process GraphQL schema, (3) Vector search tool for similarity-based document retrieval using embeddings, and (4) RAG tools - rag_query, rag_inventory, and rag_operations for retrieval-augmented generation with domain-specific context. The service manages stateful MCP sessions via StreamableHTTPServerTransport, maintains request context across tool invocations, and provides both MCP protocol compliance and direct tool execution methods. Each tool is registered with Zod schema validation and returns standardized MCP result objects.

---

### Question 83 (Category: data_flow_integration)

Trace the complete data flow for bruteforce barrel creation when barrel doesn't exist in the system.

Expected Answer: Bruteforce creation flow: 1) BarrelEventService.findOrCreate fails to find barrel, 2) createBarrelByBruteforce() called with transactionClient, serialNumber, data, eventType, locationString, 3) Parse serial to extract lot code, 4) LotVariationService.findLotByNumberOrVariation() finds canonical lot, 5) If no lot: return null (caller creates CRITICAL error), 6) resolveCustomerFromLot() gets default customer, 7) Create Barrel via Prisma: serialNumber, lotId, customerId, disposition based on location type, 8) Create BarrelOwnership record linking barrel to customer, 9) Attach bruteForceWarning to result for caller to log after transaction, 10) Warning logged via AndroidSyncErrorLoggerService with WARN level.

---

### Question 84 (Category: architecture_structure)

How does the WhiskeyTypeModule support product categorization?

Expected Answer: WhiskeyTypeModule manages whiskey type classifications with TTB compliance support (via isTTBDefinition and self-referencing hierarchy). The actual hierarchy is NOT WhiskeyType -> Recipe -> Lot -> Barrel. Instead, Lot has PARALLEL relations to both WhiskeyType (via whiskeyTypeId) and Recipe (via recipeId), with Barrel linking to Lot. The correct model is: WhiskeyType -> Lot <- Recipe, then Lot -> Barrel. WhiskeyType and Recipe are independent classification systems that both feed into Lot.

---

### Question 85 (Category: data_flow_integration)

Trace the complete data flow when an Android device scans a barrel QR code and submits it to the WMS. What happens from the moment the scan is received until the barrel's location is updated in the database?

Expected Answer: The data flow is: 1) Android device sends scan data to AndroidSyncInbox via REST API, 2) The android-sync.processor.ts picks up the inbox record from the BullMQ queue, 3) The processor fetches the AndroidSyncInbox record using the deviceName and inboxId compound key, 4) BarrelEventService.createBarrelEventWithLocationAndLocationCheck() is called which routes to appropriate handler based on eventType (Entry, InventoryCheck, Relocation, etc.), 5) The service uses Prisma.$androidSyncTransaction for serializable isolation to prevent race conditions, 6) Barrel is looked up/created via findOrCreateBarrel with lot variation normalization, 7) Location is resolved (storage or holding), 8) BarrelEvent is created and barrel.locationId/holdingLocationId is updated, 9) WebSocket progress updates are broadcast via ProgressGateway.

---

### Question 86 (Category: cross_cutting_concerns)

How does the error categorization help with automated error routing?

Expected Answer: The categorizeError() function in business-logic-error-handler.ts enables intelligent error routing by pattern-matching error messages to 6 categories: RESOURCE_NOT_FOUND (triggers data creation or lookup workflows), RESOURCE_CONFLICT (suggests retry or different parameters), VALIDATION_ERROR (returns immediately to user with guidance), DATABASE_CONSTRAINT_ERROR (indicates concurrent modification requiring refresh), PERMISSION_ERROR (routes to access request workflow), EXTERNAL_SERVICE_ERROR (triggers retry with backoff). DeviceSyncErrorReprocessorService uses error categorization for reprocessability assessment via nonReprocessableTypes (4 types requiring manual data fix) and reprocessableTypes (6 types that can be automatically retried). This automation reduces manual triage and routes errors to appropriate handlers using array-based categorization, not a centralized ERROR_CONFIGURATION constant.

---

### Question 87 (Category: data_flow_integration)

What is the complete flow for Trust Mode cutoff date processing?

Expected Answer: Cutoff date flow: 1) Session has trustModeCutoffDate (DateTime) from upload config, 2) After all batches complete, check trustModeEnabled && trustModeCutoffDate, 3) TrustModeProcessingService processes with cutoffDate, 4) Query barrels not in uploaded set, 5) For each: check lastEventDate vs cutoffDate, 6) If lastEventDate < cutoffDate: barrel not seen since before cutoff, 7) Such barrels candidates for nullification (assumed gone), 8) Create 'Nullified' BarrelEvent, clear locations, 9) trustModeDescription added to event notes for audit.

---

### Question 88 (Category: business_logic_constraints)

How does the group transaction status machine work?

Expected Answer: EnumGroupTransactionStatus: PENDING (created, not executed), IN_PROGRESS (executing), COMPLETED (success), FAILED (error), REVERSED (undone), CANCELLED (abandoned). GroupTransactionService.executeTransaction(): 1) Validates status=PENDING. 2) Updates to IN_PROGRESS. 3) Executes based on transactionType. 4) On success: COMPLETED with completedAt. 5) On failure: FAILED with error in metadata. REVERSED status used for transaction rollback. CANCELLED for user-initiated abort. Status machine prevents: re-execution of completed transactions, execution of failed transactions without intervention.

---

### Question 89 (Category: data_flow_integration)

Trace the data flow for parallel job queue operations with BullMQ.

Expected Answer: Parallel operations: 1) @Processor concurrency: 5 allows 5 concurrent jobs, 2) Each worker gets separate blocking connection for BLMOVE/BRPOPLPUSH, 3) Jobs fetched in parallel, processed independently, 4) Database operations may contend - Prisma handles connection pooling, 5) Rate limiter (10/s) prevents burst overload, 6) Each job has own lock, renewed independently, 7) Progress updates from all jobs broadcast via single WebSocket server, 8) Session isolation: each session's jobs don't interfere, 9) Resource limits monitor aggregate memory across workers.

---

### Question 90 (Category: service_relationships)

How does BarrelOwnershipService handle the joint ownership 2+ owner invariant during creation?

Expected Answer: In establishJointOwnership(): 1) Pre-validation: if (customers.length < 2) throws BadRequestException('Joint ownership requires at least 2 owners'); 2) Creates all records atomically via createMany (not individual creates); 3) Post-creation verification: if (createResult.count !== customers.length) throws with rollback; 4) Double-check: fetches created records, if (ownerships.length !== customers.length) throws. Using createMany ensures: database trigger for JOINT_OWNER validation sees all records in single batch, no race condition between individual creates. The comment explains: 'This is critical for the database trigger that validates JOINT_OWNER requires 2+ records'. Three-layer protection ensures invariant is never violated.

---

### Question 91 (Category: data_flow_integration)

How does Trust Mode processing work after inventory upload completion? Describe the cutoff date logic and barrel nullification flow.

Expected Answer: Trust Mode flow: 1) InventoryProcessingProcessor checks session.trustModeEnabled and trustModeCutoffDate after all batches complete, 2) TrustModeProcessingService.processBarrelsForTrustMode() called with sessionId, cutoffDate, description, 3) Service queries InventoryUploadStaging for resolved barrel IDs, 4) For each barrel not in uploaded set: checks lastEventDate against cutoffDate, 5) Barrels with no events after cutoff are candidates for nullification, 6) Nullification creates BarrelEvent with 'Nullified' type, clears locationId/holdingLocationId, 7) Result includes: barrelsProcessed, barrelsNullified, barrelEventsCreated, errors array, 8) Progress logged via DatabaseProgressService with correlationId, 9) Trust Mode failure doesn't fail entire session - caught and logged as ERROR.

---

### Question 92 (Category: cross_cutting_concerns)

How does the audit correlation ID enable request tracing across services?

Expected Answer: Correlation IDs are generated at request entry (typically a UUID v4) and propagated through all service calls and audit logs. The ContextService stores correlationId in AsyncLocalStorage context. All audit log entries include correlationId, enabling reconstruction of the full request flow. For distributed systems, correlationId is passed in request headers (e.g., x-correlation-id) and extracted by receiving services. AuditService.getAuditLogsByCorrelationId() retrieves all entries for a single request. This enables debugging complex operations like resolution workflows where multiple services (ResolutionService, BarrelService, PrinterAutomationService) participate in handling a single user action.

---

### Question 93 (Category: service_relationships)

What transaction boundaries exist in the ReconciliationService.createReconciliation flow?

Expected Answer: createReconciliation does NOT use a single wrapping transaction: 1) Creates ReconciliationRecord (single Prisma create); 2) If barrelReconciliation needed, calls reconcileGroupBarrelCount (separate operation); 3) If that fails, updates ReconciliationRecord with error status (separate update); 4) If requiresApproval, creates ApprovalRequest (separate create); 5) Creates AuditLog (separate create). Design decision: allows partial success - reconciliation record exists even if barrel adjustment fails. Trade-off: potential inconsistency vs resilience. The separate operations each have their own transactions. For atomic behavior, would need prisma.$transaction wrapper around all operations.

---

### Question 94 (Category: architecture_structure)

How does the frontend filter components architecture support persistence and responsive design?

Expected Answer: The components/filters directory contains ResponsiveFilterBar/ with index.ts (barrel file for exports) and useActiveFilters.ts (hook for filter state). FilterPersistenceBridge.tsx connects to URL state for shareable filter links. FilterPresets.tsx allows saving/loading filter configurations. This architecture separates filter UI rendering (responsive breakpoints), state management (active filters hook), persistence (URL/local storage), and presets (saved configurations) into composable pieces.

---

### Question 95 (Category: service_relationships)

How does the error propagation flow work when BarrelOwnershipService.establishJointOwnership encounters validation failures?

Expected Answer: In establishJointOwnership(): 1) Pre-transaction validation: checks customers.length < 2 throws BadRequestException('Joint ownership requires at least 2 owners'); checks totalPercentage !== 100 throws BadRequestException('percentages must add up to 100%'); 2) Within transaction: if createMany.count !== customers.length, throws BadRequestException('Failed to create all...') causing rollback; 3) Post-creation verification: if fetched ownerships.length !== customers.length, throws BadRequestException('count mismatch'). All BadRequestExceptions bubble up to the controller, which returns 400 status. The transaction rollback ensures no partial joint ownership state exists.

---

### Question 96 (Category: cross_cutting_concerns)

How does the audit log PubSub integration enable real-time audit monitoring?

Expected Answer: The AuditService injects PubSubService and publishes audit events for real-time distribution. Each audit log creation triggers a publish to 'auditLogAdded' channel (not 'audit:created') with the log entry data in a GraphQL subscription format: { auditLogAdded: { edges: [{node, cursor}], pageInfo } }. The PubSubService uses graphql-subscriptions PubSub (in-memory), NOT Redis. This enables GraphQL subscriptions to relay audit events to connected clients. Sensitive audit data is sanitized before storage via sanitizeForAudit() which removes fields like password, token, secret, apiKey. This enables real-time monitoring without polling the database, though cross-instance distribution requires additional Redis configuration not present in the current implementation.

---

### Question 97 (Category: service_relationships)

What is the relationship between BarrelService and BarrelServiceBase for method inheritance?

Expected Answer: BarrelService extends BarrelServiceBase. The base class (generated, in base/ folder) provides: standard CRUD methods (createBarrel, findBarrels, updateBarrel), common query patterns, Prisma integration via protected prisma. BarrelService overrides/extends: adds createBarrelWithOwnership for ownership-aware creation, adds createBarrelWithSoleOwnership convenience method, adds validateBarrelCreationData, validateDispositionLocationState for business rules, adds buildUnifiedDirectWhere for complex filtering. Pattern: base provides infrastructure, derived adds business logic. The super() call in constructor passes prisma to base.

---

### Question 98 (Category: business_logic_constraints)

What business rules govern EnumOwnershipTransactionType and when is each type recorded?

Expected Answer: EnumOwnershipTransactionType tracks ownership changes: 1) ESTABLISHMENT - single owner created (createOwnership). 2) JOINT_ESTABLISHMENT - joint ownership created (establishJointOwnership). 3) INHERITANCE - child customer inherits from parent (establishInheritance). 4) TRANSFER - ownership moved between customers (transferOwnership). 5) BULK_TRANSFER - batch transfer operation. 6) BULK_ESTABLISHMENT - batch ownership creation. 7) BULK_JOINT_ESTABLISHMENT - batch joint ownership. Each transaction records fromCustomerId, toCustomerId, ownershipPercentage, transferReasonId, notes, jointOwnershipGroupId, operationId, batchId, groupId for full audit trail.

---

### Question 99 (Category: service_relationships)

What is the relationship between BarrelEventService and CachedEventTypeService for avoiding in-transaction queries?

Expected Answer: BarrelEventService injects CachedEventTypeService (comment: 'TEC-fix: Cache to avoid in-transaction EventType queries'). EventTypes are reference data (Entry, Withdrawal, etc.) rarely changing. Without caching: every barrel event creation would query EventType table within the transaction, increasing lock duration and deadlock risk. With CachedEventTypeService: 1) Cache is warmed at startup; 2) findByName() returns cached value; 3) No database query during event creation transaction. This reduces transaction duration, prevents lock contention, improves throughput. Similar pattern used for EventReason, LotVariation, StorageLocation caches.

---

### Question 100 (Category: architecture_structure)

How does the FeatureFlagsModule integrate with LaunchDarkly and affect module loading?

Expected Answer: FeatureFlagsModule is imported early in AppModule and uses LaunchDarkly SDK (configured via LAUNCHDARKLY_SDK_KEY). The FeatureFlagsService provides flags to other modules for conditional feature enablement. Frontend uses useFeatureFlag hook connecting to the same LaunchDarkly project. This enables gradual rollouts, A/B testing, and quick feature disabling without deployment. Being early in imports ensures flags are available before dependent modules initialize.

---

## FINAL REPORT (REQUIRED)

After answering ALL questions, record END_TIME and provide:

```
=== BASELINE TEST v4 RESULTS ===
START_TIME: [from Phase 1]
INGESTION_COMPLETE_TIME: [from Phase 1]
END_TIME: [now - run: date "+%Y-%m-%d %H:%M:%S"]
TOTAL_ELAPSED_TIME: [calculate]
INGESTION_TIME: [calculate]
QUESTION_TIME: [calculate]

FILES_EXPECTED: 3288
FILES_VERIFIED: [output of wc -l command from Phase 1]

ANSWERS_FROM_MEMORY: [count]
ANSWERS_FROM_LOOKUP: [count]
ANSWERS_FROM_BOTH: [count]

TOTAL_SCORE: [sum of scores]
SCORE_BY_CATEGORY:
  - architecture_structure: [X/Y]
  - service_relationships: [X/Y]
  - cross_cutting_concerns: [X/Y]
  - data_flow_integration: [X/Y]
  - business_logic_constraints: [X/Y]

CONTEXT_RETENTION_NOTES:
  - [what did you remember vs forget?]
  - [which categories were harder from memory?]
===
```
