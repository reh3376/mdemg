# PHASE 2: QUESTIONS (Read-Only Phase)

You have now read all files. Below are 100 questions to answer.

REMINDER:

- Do NOT read any files
- Do NOT use Glob or Grep
- Answer from memory ONLY

---

## Questions

### Question 1 (Category: data_flow_integration)

Trace the data flow for circuit breaker reset on successful batch.

Expected Answer (for scoring):
Circuit breaker reset: 1) After batch processing, if batchResult.errorCount === 0, 2) Reset metrics.consecutiveErrors = 0, 3) Call safetyLimitsService.resetCircuitBreaker(sessionId), 4) Service clears any error state for session, 5) Allows processing to continue without accumulated penalty, 6) If errors: increment consecutiveErrors, no reset, 7) Circuit breaker pattern: prevents cascading failures, 8) After MAX_CONSECUTIVE_ERRORS: circuit open, processing halts, 9) Recovery: manual intervention or time-based auto-reset.

---

### Question 2 (Category: architecture_structure)

What is the architecture pattern for handling device sync errors across multiple modules?

Expected Answer (for scoring):
Device sync errors span: DeviceSyncErrorModule (entity storage with DeviceSyncErrorService/Controller), DeviceSyncErrorManagementModule (error processing with DeviceSyncErrorReprocessorService and EnhancedErrorLoggerService), and AndroidSyncInboxModule (receiving sync data). There is NO separate ResolutionModule - error resolution is handled by DeviceSyncErrorReprocessorService methods (reprocessErrors, bulkResolve, markForRetry). The flow: mobile sends data -> AndroidSyncInbox receives -> validation fails -> error stored in DeviceSyncError -> DeviceSyncErrorReprocessorService handles reprocessing/resolution. Frontend error display is in app/dashboard/jobs/[id] with error-context-utils.ts.

---

### Question 3 (Category: business_logic_constraints)

What constraints govern lot audit actions?

Expected Answer (for scoring):
EnumLotAuditAction: Create, Increase, Decrease, Adjustment - tracks changes to Lot records with quantity-focused actions. LotAuditService (injected into GroupTransactionService) records lot-level changes. EnumBarrelAuditAction (Create, Update, Delete) mirrors for barrel changes. Audit actions recorded with: lotId, action, changedBy, changedAt, quantity snapshots (bblTotal, totalPGs, totalWGs), deltaUnits, deltaUnitType, groupTransactionId, transferId, notes. Lot audits are critical for: TTB compliance (federal regulations require complete lot history), customer reporting, production traceability. All lot modifications (bblTotal, status, quantities) generate audit records.

---

### Question 4 (Category: cross_cutting_concerns)

How does the application secure API keys for machine-to-machine authentication?

Expected Answer (for scoring):
Security headers do NOT differ between development and production environments. The SecurityHeadersMiddleware applies the same headers universally: X-Content-Type-Options, X-Frame-Options, X-XSS-Protection, Strict-Transport-Security, Content-Security-Policy. There is no environment-based conditional logic in the middleware implementation.

---

### Question 5 (Category: cross_cutting_concerns)

How does the feature flag guard pattern work for conditionally enabling features?

Expected Answer (for scoring):
The McpFeatureFlagGuard demonstrates the pattern for feature-flag-controlled features. It implements CanActivate and injects FeatureFlagsService. In canActivate(), it first checks for local development override (MCP_SERVER_ENABLED env var in non-production). Then it evaluates the LaunchDarkly flag using evaluateBoolean() with a system context. If the flag is disabled, it throws HttpException with 404 status, making the feature appear non-existent. This pattern allows features to be toggled without code deployment. The guard runs BEFORE authentication, ensuring clean 404 responses even for unauthenticated requests. Multiple feature guards can be composed using @UseGuards().

---

### Question 6 (Category: service_relationships)

What is the role of ContextService in the audit logging flow?

Expected Answer (for scoring):
AuditService injects ContextService but based on createAuditLog(), context is passed explicitly via CreateAuditLogDto (userId, sessionId, ipAddress, userAgent, correlationId). ContextService provides: 1) Request-scoped context storage (user from JWT, request metadata); 2) getCurrentUserId() for resolvers that don't pass explicitly; 3) Request correlation ID for tracing. The AuditService primarily uses passed parameters but ContextService serves as fallback and for request-scoped operations. This separation allows: explicit context passing for clarity, fallback to request context when convenient, async operations that outlive request scope.

---

### Question 7 (Category: business_logic_constraints)

What constraints govern the report generation system?

Expected Answer (for scoring):
ReportType enumerates: CURRENT_INVENTORY, DAILY/WEEKLY/MONTHLY/QUARTERLY/ANNUAL_INVENTORY, TRANSACTION_SUMMARY/DETAIL, GROUP_TRANSACTION_SUMMARY, INVENTORY_AGING, WAREHOUSE_UTILIZATION, CUSTOMER_ALLOCATION, LOT_TRACKING, CUSTOM. ReportFormat: JSON, CSV, EXCEL, PDF, HTML. ReportStatus: PENDING, GENERATING, COMPLETED, FAILED, SCHEDULED. finance-report-templates.dto.ts defines report templates. Reports support: date range filters, entity filters, format selection. Large reports run asynchronously with status tracking. Reports used for: TTB compliance, customer statements, management dashboards.

---

### Question 8 (Category: architecture_structure)

How does the SchedulerModule integrate with NestJS's ScheduleModule for cron jobs?

Expected Answer (for scoring):
The SchedulerModule does NOT use NestJS's ScheduleModule directly in the way described. Instead, it: (1) Uses Bull Queue - integrates with @nestjs/bull for job processing (BullModule.registerQueue), (2) Uses @nestjs/schedule for decorators - The BarrelReportSchedulerService uses @Cron decorators to schedule weekly barrel reports at 4 PM EST Monday-Friday, (3) Implements distributed locking using Redis SETNX + EX to ensure only one instance runs across multiple nodes, (4) Stores process.pid in Redis lock for debugging. Architecture: ScheduleModule provides cron decorator framework, BullModule.registerQueue provides async job queue infrastructure, Redis is used for both Bull queue backend AND distributed locking.

---

### Question 9 (Category: service_relationships)

What transaction boundaries exist in the ReconciliationService.createReconciliation flow?

Expected Answer (for scoring):
createReconciliation does NOT use a single wrapping transaction: 1) Creates ReconciliationRecord (single Prisma create); 2) If barrelReconciliation needed, calls reconcileGroupBarrelCount (separate operation); 3) If that fails, updates ReconciliationRecord with error status (separate update); 4) If requiresApproval, creates ApprovalRequest (separate create); 5) Creates AuditLog (separate create). Design decision: allows partial success - reconciliation record exists even if barrel adjustment fails. Trade-off: potential inconsistency vs resilience. The separate operations each have their own transactions. For atomic behavior, would need prisma.$transaction wrapper around all operations.

---

### Question 10 (Category: business_logic_constraints)

How does the audit log system track entity changes?

Expected Answer (for scoring):
EnumAuditOperationType defines operations: Create, Update, Delete, UPDATE (duplicate legacy). EnumAuditEntityType enumerates auditable entities: User, StorageLocation, Barrel, Sample, Customer, SalesOrder, Item, Lot, WeightLog, etc. EnumAuditOperationResult: success, failure. AuditService.createAuditLog() records: operationType, entityType, entityId, userId, correlationId, beforeState, afterState, operationDetails. Used throughout services for compliance (TTB regulations require complete audit trail). ReconciliationService creates audit logs for reconciliation operations. Audit reports support multiple formats and date ranges.

---

### Question 11 (Category: architecture_structure)

How does the frontend filter components architecture support persistence and responsive design?

Expected Answer (for scoring):
The components/filters directory contains ResponsiveFilterBar/ with index.ts (barrel file for exports) and useActiveFilters.ts (hook for filter state). FilterPersistenceBridge.tsx connects to URL state for shareable filter links. FilterPresets.tsx allows saving/loading filter configurations. This architecture separates filter UI rendering (responsive breakpoints), state management (active filters hook), persistence (URL/local storage), and presets (saved configurations) into composable pieces.

---

### Question 12 (Category: architecture_structure)

How does the secretsManager module integrate with Azure Key Vault for credential management?

Expected Answer (for scoring):
The secretsManager module does NOT currently integrate with Azure Key Vault. Instead: (1) Current Implementation - SecretsManagerServiceBase uses NestJS ConfigService to retrieve secrets from environment variables. getSecret<T>(key) simply calls configService.get(key). (2) No Caching Mechanism - No TTL caching exists, no LRU cache implementation. Each call directly queries ConfigService. (3) Azure Configuration Present but Unused - .env.example shows Azure environment variables (AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, etc.) but these are NOT used by SecretsManagerService. Azure AD is implemented in AzureAdService (authentication) but separate from secrets management. (4) Architecture - Minimal wrapper around NestJS ConfigService with no provider pattern for alternative implementations.

---

### Question 13 (Category: business_logic_constraints)

What invariants must hold for the barrel-lot relationship?

Expected Answer (for scoring):
Every barrel must have a valid lotId (foreign key to Lot). populate_barrel_details() ensures lot exists or creates it before barrel. Barrel.serialNumber format typically includes lot number (e.g., '24A001.0478'). Lot.bblTotal should equal count of barrels with that lotId (maintained by events). Barrels in lot share: whiskeyType, recipe, fillDate (approximately). These invariants ensure: accurate lot-level reporting, proper inventory aggregation, regulatory compliance for batch tracking. Orphaned barrels (no lot) are invalid states.

---

### Question 14 (Category: business_logic_constraints)

How does the aggregated storage entries system work?

Expected Answer (for scoring):
aggregated-storage-entries-response.dto.ts returns paginated storage entries for TTB compliance reporting. aggregated-storage-entry.dto.ts defines per-entry metrics including: periodStart/End, lotNumber, eventCategory (ENTRY, WITHDRAWAL, TRANSFER_IN, DUMP), barrelCount, proofGallons. breakdown.dto.ts provides breakdowns by customer (StorageCustomerBreakdown), whiskey type (StorageWhiskeyTypeBreakdown), and recipe (StorageRecipeBreakdown). NOTE: This is NOT about warehouse locations, occupied positions, or capacity planning - it's specifically for TTB storage compliance aggregation by time period, lot, and event category.

---

### Question 15 (Category: data_flow_integration)

How does location string parsing work for storage locations? Describe the extraction of floor, warehouse, row, bay, rick, and tier.

Expected Answer (for scoring):
Location components include: LocationService for CRUD operations, LocationResolver for GraphQL endpoints, LocationController for REST endpoints. The location hierarchy uses parentId for self-referential tree structure. StorageLocation is a separate entity representing physical storage positions within a Location, with fields for rack, level, position. The answer incorrectly implied a single location model - there are two distinct entities: Location (warehouse/building/area) and StorageLocation (specific barrel positions).

---

### Question 16 (Category: cross_cutting_concerns)

How does the Public decorator work with Swagger documentation?

Expected Answer (for scoring):
The @Public() decorator uses applyDecorators to combine two metadata settings. PublicAuthMiddleware sets IS_PUBLIC_KEY to true, which auth guards check via Reflector to bypass authentication. PublicAuthSwagger sets 'swagger/apiSecurity' metadata to ['isPublic'], which Swagger/OpenAPI generators use to mark the endpoint as not requiring authentication in API documentation. This dual approach ensures: 1) Runtime auth bypass works correctly, 2) Generated API docs accurately reflect that the endpoint is publicly accessible. The decorator can be applied at controller or method level, with method-level taking precedence for nested configurations.

---

### Question 17 (Category: data_flow_integration)

Trace the data flow for resolving customer from lot for barrel ownership.

Expected Answer (for scoring):
Customer resolution flow: 1) resolveCustomerFromLot(lot) in staging service, 2) Lot has relation to Customer (via customer or customerLots), 3) Query lot with customer include, 4) Return lot.customer if present, 5) If no direct relation: fallback logic (default customer, lookup by lot prefix), 6) If not resolvable: return null, caller creates CRITICAL error, 7) Customer used for: BarrelOwnership creation, access control, reporting, 8) Ownership validFrom set to current date, validTo null (active).

---

### Question 18 (Category: service_relationships)

How does InventoryUploadService.validateCsvStructure handle header validation with BOM and encoding issues?

Expected Answer (for scoring):
validateCsvStructure(): 1) Sets validationTimeout (5000ms) with cleanup; 2) Converts buffer to UTF-8 string; 3) Creates stream from csv-parser; 4) On first row (isFirstRow): extracts headers via Object.keys(data); 5) Cleans headers: header.replace(/^\uFEFF/, '').trim() - removes BOM (Byte Order Mark); 6) Case-insensitive comparison: header.toLowerCase() === required.toLowerCase(); 7) Checks for missing required headers; 8) Rejects with descriptive error if missing. This handles: Excel-exported CSVs with BOM, case variations in headers, early termination after validation (maxRowsToCheck=10), timeout prevention for large files.

---

### Question 19 (Category: service_relationships)

How does AuditService.createAuditLogs handle bulk audit logging atomically?

Expected Answer (for scoring):
createAuditLogs(dataArray): 1) Wraps all operations in prisma.$transaction(async (tx) => {...}); 2) Iterates dataArray, for each: validates userId via validateUserId(data.userId, tx) using same tx; creates auditLog via tx.auditLog.create(); 3) Pushes to logs array; 4) After loop, publishes all to PubSubService; 5) Returns all created logs. Benefits: single transaction for all logs (atomic), user validation uses same transaction (consistent reads), batch PubSub publishing. Use case: bulk operations that need multiple related audit entries to be all-or-nothing.

---

### Question 20 (Category: architecture_structure)

What modules must be imported for the BarrelModule to function, and why does it use forwardRef for AuthModule and BarrelOwnershipModule?

Expected Answer (for scoring):
The BarrelModule imports BarrelModuleBase (base CRUD operations), AuthModule (authentication/authorization), BarrelOwnershipModule (ownership tracking), LotVariationModule (lot variation handling), AuditModule (audit logging), and ContextModule (request context). forwardRef is used for AuthModule and BarrelOwnershipModule to resolve circular dependencies - BarrelModule needs ownership services, but ownership may reference barrels back, creating a circular import that forwardRef lazily resolves.

---

### Question 21 (Category: data_flow_integration)

How does the consecutive empty batch detection prevent infinite loops?

Expected Answer (for scoring):
Empty batch detection: 1) sessionMetrics Map tracks consecutiveEmptyBatches per session, 2) After processBatch(): if processedCount === 0, increment counter, 3) If processedCount > 0: reset counter to 0, 4) SafetyLimitsService.checkSafetyLimits() evaluates: consecutiveEmptyBatches vs MAX_EMPTY_BATCHES, 5) If exceeded: return {canContinue: false, reason: 'Too many empty batches'}, 6) Processor logs FATAL, fails job, cleans up metrics, 7) Prevents infinite continuation when no work remains but shouldContinue logic is flawed, 8) Empty batches indicate: all processed, or processing stuck.

---

### Question 22 (Category: cross_cutting_concerns)

How does the FeatureFlagsService handle graceful shutdown and event flushing?

Expected Answer (for scoring):
The FeatureFlagsService implements OnApplicationShutdown interface. In onApplicationShutdown(), it first attempts to flush() any pending events to LaunchDarkly, ensuring analytics data is not lost. It catches and logs flush errors but continues shutdown. Then it calls close() on the client to properly disconnect. Finally, it sets client to null and isInitialized to false. This graceful shutdown is important in containerized environments where SIGTERM triggers shutdown. The service also handles initialization failures gracefully - if LaunchDarkly fails to initialize, it logs a warning but continues operating, returning default values for all flag evaluations.

---

### Question 23 (Category: business_logic_constraints)

What business rules govern EnumReconciliationType and when is each type created?

Expected Answer (for scoring):
EnumReconciliationType categorizes discrepancy sources: 1) INVENTORY_DISCREPANCY - physical count doesn't match system (inventory checks). 2) TRANSACTION_VARIANCE - calculated vs actual transaction values differ. 3) OWNERSHIP_MISMATCH - ownership records don't align with expected state. 4) DATA_CORRECTION - manual data fix needed. 5) MANUAL_ADJUSTMENT - operator-initiated adjustment. 6) SYSTEM_ERROR - system bug caused inconsistency. 7) REVERSAL - transaction needs to be reversed. Each type has different priority defaults and may require different approval workflows based on impactLevel.

---

### Question 24 (Category: architecture_structure)

How does the DeviceSyncLogModule support mobile device troubleshooting?

Expected Answer (for scoring):
DeviceSyncLogModule records detailed logs of device sync operations - what was sent, what succeeded, what failed. Unlike DeviceSyncError (which stores errors for resolution), DeviceSyncLog stores all sync activity for diagnostics. This enables: troubleshooting sync issues, identifying device problems, and auditing what data came from which device. The separation allows querying errors independently from the full sync log history.

---

### Question 25 (Category: cross_cutting_concerns)

How do role guards interact with resolvers to enforce access control?

Expected Answer (for scoring):
Access control is enforced through a layered guard system. The RolesGuard reads roles from the @Roles() decorator metadata and checks if the authenticated user's roles include any of the required roles. The GqlACGuard extends this for GraphQL by extracting the request from GqlExecutionContext. The ACL system uses the 'accesscontrol' library with getInvalidAttributes() in abac.util.ts to filter response data based on permissions. The aclFilterResponse.interceptor.ts filters response data, while aclValidateRequest.interceptor.ts validates incoming request data. The MultiStrategyAuthGuard tries multiple authentication strategies (JWT, API key) in sequence. Guards are applied globally via APP_GUARD or per-resolver using @UseGuards(). The @Public() decorator sets IS_PUBLIC_KEY metadata to bypass authentication entirely.

---

### Question 26 (Category: cross_cutting_concerns)

How does the application handle long-running GraphQL mutations?

Expected Answer (for scoring):
TimeoutInterceptor is HTTP-specific, implementing NestInterceptor. It wraps handler execution with RxJS timeout operator. It is NOT GraphQL-specific - it applies to both REST and GraphQL requests when configured. The interceptor is not globally registered; it must be applied per-controller or per-method using @UseInterceptors().

---

### Question 27 (Category: architecture_structure)

How does the frontend components/bulk-operations architecture support multi-step wizards?

Expected Answer (for scoring):
The bulk-operations directory contains wizard-steps/ subdirectory with step components and an index.ts barrel export. BulkTransferWizard and bulk operation pages use these steps. The wizard has 5 steps, not 4: (1) BarrelSelectionStep - select barrels using FilterBuilder with advanced filtering, (2) TransferConfigurationStep - configure operation including new owner selection, transfer reason, ownership percentage, and effective date, (3) OperationPreviewStep - preview changes before execution showing barrel details and transfer summary, (4) ConfirmationStep - confirm with checkboxes and execute the operation with progress tracking, (5) ResultSummaryStep - display execution results, success/failure counts, and provide options to download reports or start new operation. Each step is a separate component with typed props interfaces (e.g., BarrelSelectionStepProps) and shares state via BulkTransferFormData type. The bulk-operations/index.ts also exports FilterBuilder, OperationMonitor, and related types.

---

### Question 28 (Category: cross_cutting_concerns)

How does the resolution system ensure idempotency for resolution operations?

Expected Answer (for scoring):
Idempotency is ensured through: 1) Pre-flight validation checking if errors are already RESOLVED - duplicate resolution attempts throw clear errors listing already-resolved IDs, 2) Database constraints preventing duplicate resolution records, 3) Prisma transactions ensuring atomic updates - partial failures rollback completely, 4) Status transitions are one-way (UNRESOLVED -> RESOLVED), preventing state oscillation. The resolution record stores the unique combination of error IDs and resolution context. Re-submitting the same resolution request fails with 'already resolved' error rather than creating duplicates. Post-resolution verification confirms status changes persisted.

---

### Question 29 (Category: cross_cutting_concerns)

How does the audit service handle high-volume logging without impacting application performance?

Expected Answer (for scoring):
The AuditService uses several strategies for performance: 1) Async logging - createAuditLog() returns immediately after queueing the write, 2) Batch inserts for bulk operations using createAuditLogBatch(), 3) Indexed queries using correlationId, entityId, userId for fast lookups, 4) Pagination for large result sets, 5) The service can be configured to log asynchronously via PubSub, decoupling the write from the request. For extremely high volume, audit logs can be written to a separate database or time-series store. The changedFields array is pre-computed rather than storing full before/after for small changes, reducing storage.

---

### Question 30 (Category: service_relationships)

What is the role of BarrelEventService.extractRelevantLocation in location string parsing?

Expected Answer (for scoring):
extractRelevantLocation(locationString): 1) Splits by '-': parts = locationString.split('-'); 2) Validates length >= 7 (full format expected); 3) Ignores first two pieces (typically org/warehouse identifiers); 4) Returns remaining: parts.slice(2).join('-'). Example: '01-WHK01-01-1-061-A-1' -> '01-1-061-A-1' (floor-tier-position-rick-number). This standardization enables: consistent location parsing regardless of leading identifiers, compatibility with different location string formats from devices, separation of routing (first pieces) from physical location (remaining).

---

### Question 31 (Category: business_logic_constraints)

How does the changelog system track application changes?

Expected Answer (for scoring):
changelog.dto.ts defines changelog entry structure. ChangelogEntry is EnumCommentableEntity (supports comments). Changelog tracks: version releases, feature additions, bug fixes, breaking changes. EnumCommentType includes GENERAL and SYSTEM for changelog annotations. Used for: user communication, support reference, audit compliance. Changelog entries may link to: git commits, issue trackers, affected entities. Comments on changelog enable: user feedback, clarification requests, deployment notes.

---

### Question 32 (Category: cross_cutting_concerns)

How does the audit correlation ID enable request tracing across services?

Expected Answer (for scoring):
Correlation IDs are generated at request entry (typically a UUID v4) and propagated through all service calls and audit logs. The ContextService stores correlationId in AsyncLocalStorage context. All audit log entries include correlationId, enabling reconstruction of the full request flow. For distributed systems, correlationId is passed in request headers (e.g., x-correlation-id) and extracted by receiving services. AuditService.getAuditLogsByCorrelationId() retrieves all entries for a single request. This enables debugging complex operations like resolution workflows where multiple services (ResolutionService, BarrelService, PrinterAutomationService) participate in handling a single user action.

---

### Question 33 (Category: business_logic_constraints)

How does the sample type result type affect sample processing?

Expected Answer (for scoring):
EnumSampleTypeTypeResultType at schema.prisma defines only TWO values: Boolean, Numeric. The actual implementation uses a simpler type system where sample result types are categorized as either Boolean (binary pass/fail) or Numeric (continuous value measurement). Sample analysis results are tracked separately in the Sample.sampleResult field (Boolean) and Sample.sampleValue field (Float), not through result type categorization. This differs from a predicted status-based approach (Pass, Fail, Pending, etc.).

---

### Question 34 (Category: architecture_structure)

How does the backend handle WebSocket connections for GraphQL subscriptions?

Expected Answer (for scoring):
AppModule's GraphQLModule.forRootAsync configures subscriptions with both 'graphql-ws' (modern protocol) and 'subscriptions-transport-ws' (legacy). The graphql-ws config includes onConnect/onDisconnect handlers for logging and storing connectionParams. PubSubModule provides the pub/sub engine (Redis in production). This dual-protocol support ensures compatibility with both modern (Apollo Client 3+) and legacy (Apollo Client 2) frontend clients while maintaining stateful subscription connections.

---

### Question 35 (Category: data_flow_integration)

What is the data flow when a location is already occupied during entry event processing?

Expected Answer (for scoring):
Location occupied handling: 1) In storage location processing, after location lookup, 2) checkLocationOccupied(locationId, incomingBarrelId) queries barrel with that locationId, 3) If found and id !== incomingBarrelId: location occupied, 4) For staging validation: creates WARN issue 'LocationAlreadyOccupied', context includes displacedBarrelId, displacedBarrelSN, targetLocationId, resolutionAction='DISPLACE', 5) For direct processing: creates displacement BarrelEvent with 'Displaced' type for occupying barrel, 6) Clears occupyingBarrel.locationId, 7) Then assigns location to incoming barrel, 8) Ensures location-barrel relationship is 1:1.

---

### Question 36 (Category: business_logic_constraints)

How does the trust mode configuration affect validation?

Expected Answer (for scoring):
trust-mode-config.dto.ts defines a specific 'Trust Android Scans Mode' configuration with: enabled (boolean toggle), cutoffDate (ISO date for AndroidSyncInbox records), and optional description. This mode affects how the system handles Android device scan data, not general validation strictness. Separate ValidationOptions in DataValidationService (skipStorageValidation, skipEntityValidation, skipIntegrityValidation) control validation behavior. Auto-lot creation is handled by validate_business_entities_with_auto_lot_creation() PostgreSQL function, independent of trust mode settings.

---

### Question 37 (Category: business_logic_constraints)

What database constraints ensure data integrity for barrel-location relationships, and how does BarrelLocationConflictException prevent conflicts?

Expected Answer (for scoring):
A barrel cannot have both a storage location (locationId) AND a holding location (holdingLocationId) simultaneously. The BarrelLocationConflictException throws BAD_REQUEST with message 'A barrel cannot have both a storage location and a holding location. Please specify only one.' This business rule exists because a physical barrel can only be in one place at a time - either ricked storage (StorageLocation) or temporary holding areas (HoldingLocation). The LocationAlreadyOccupiedException handles the case where a storage location is already occupied by a different barrel, preserving full error context for transaction boundary survival.

---

### Question 38 (Category: business_logic_constraints)

How does the Android sync inbox handle duplicate records and what strategy prevents deadlocks during bulk inserts?

Expected Answer (for scoring):
AndroidSyncInboxService.saveToInbox() uses chunked processing with CHUNK_SIZE=50 and CHUNK_DELAY_MS=100 between chunks to: 1) Reduce transaction size and lock duration. 2) Prevent identical createdAt timestamps. 3) Improve database resource utilization. 4) Enable better error isolation. Duplicate handling uses raw SQL with 'ON CONFLICT (deviceName, inboxId) DO NOTHING RETURNING *' to atomically identify actually inserted records. Records returned by RETURNING were inserted; null results are duplicates. This eliminates race conditions from timestamp-based duplicate detection queries.

---

### Question 39 (Category: data_flow_integration)

Trace the data flow for resolving canonical lot from a lot variation serial number.

Expected Answer (for scoring):
Canonical lot resolution: 1) resolveLotFromSerialNumber() in staging service, 2) Parse serial: extract potential lot code from first parts, 3) Call LotVariationService.findLotByNumberOrVariation(potentialLotCode), 4) Service queries: Lot where lotNumber matches OR LotVariation where variationNumber matches, 5) If found via variation: return canonical Lot (the parent), 6) If found directly: return Lot, 7) If not found: return {success: false, error: 'Lot not found'}, 8) Canonical lot has: id, lotNumber (canonical), customer relation, 9) Used for: barrel creation, duplicate detection, ownership resolution.

---

### Question 40 (Category: service_relationships)

How does the CachedStorageLocationService integrate with AndroidSyncProcessor for location validation?

Expected Answer (for scoring):
AndroidSyncProcessor injects CachedStorageLocationService. In onModuleInit(), calls cachedStorageLocationService.warmCache('WHK01') for the primary warehouse. During event processing: when validating/finding storage locations, uses cached lookups instead of database queries. Benefits: 1) No database roundtrip for common location lookups; 2) Consistent within processing batch (cache is warmed snapshot); 3) Reduces contention on StorageLocation table during high-throughput sync. The 'WHK01' parameter focuses cache on the primary warehouse, optimizing memory usage while covering the common case.

---

### Question 41 (Category: business_logic_constraints)

How does the resolution system suggest serial numbers?

Expected Answer (for scoring):
Resolution services provide serial number suggestions for error correction: predict-next-9xxx-serial.dto.ts handles 9xxx-series temporary serials (e.g., 24E01A.01.9001 with nextNumber 9001-9999 range). suggest-serial-number.dto.ts returns SerialNumberSuggestionDto with suggestedSerial, confidence score (0-100), confidenceLevel (high/medium/low), lotAnalysis with barrelCount and percentage, reasoning, alternatives with multiple suggestions, and adjacentBarrels for visualization. The BarrelMatchingService matches scanned data against existing barrels considering: lot variations via LotVariationService and distance weights. The prediction algorithm uses weights defined in prediction-weights.dto.ts: adjacentLotsWithSequence (60%), currentJobLots (20%), horizontalPositionLots (20%).

---

### Question 42 (Category: data_flow_integration)

How does the GraphQL subscription system propagate comment updates to connected clients? Describe the PubSub pattern and filtering mechanism.

Expected Answer (for scoring):
Comment subscription flow: 1) PubSubService wraps graphql-subscriptions PubSub, provides publish() and asyncIterableIterator(), 2) CommentResolver defines @Subscription with filter function checking entityType and entityId match, 3) When comment created/updated, service calls pubSub.publish('commentAdded', payload), 4) Subscription filter evaluates: payload.entityType === variables.entityType && payload.entityId === variables.entityId, 5) Only matching subscribers receive the update, 6) commentUpdated, commentDeleted follow same pattern, 7) mentionedInComment subscription filters by payload.mentionedUserIds?.includes(variables.userId), 8) resolve function transforms payload before sending to client.

---

### Question 43 (Category: architecture_structure)

How does the barrel GraphQL query structure support unified filtering across multiple entity relationships?

Expected Answer (for scoring):
The unifiedBarrels query in barrel.resolver.ts accepts UnifiedBarrelFilterInput that spans barrel properties (serialNumber, disposition), ownership (customerCode, customerId), location (warehouse, floor, rick), and lot (lotNumber, bblOem). The resolver's buildUnifiedWhere method translates these into Prisma where clauses, handling both direct fields and relation filters. This unified approach lets frontends build complex queries without knowing the underlying data model relationships.

---

### Question 44 (Category: cross_cutting_concerns)

How does the audit system handle entity history reconstruction?

Expected Answer (for scoring):
AuditService provides getEntityAuditHistory(entityType, entityId, options) (not getEntityHistory) which queries all audit logs for that entity. Options include skip, take, and orderBy for pagination and sorting. Each entry contains beforeState and afterState, allowing reconstruction of the entity's complete history. changedFields array (automatically calculated via AuditService.getChangedFields()) identifies which fields changed in each update. getAuditLogsByCorrelation(correlationId) links related entity changes from a single operation, returning logs in chronological order. The stored data supports point-in-time reconstruction and change timeline analysis, though these are not explicitly implemented as separate methods.

---

### Question 45 (Category: architecture_structure)

How does the frontend Table-new directory structure support different table configurations?

Expected Answer (for scoring):
The Table-new directory supports different table configurations through component composition and specialization: (1) Specialized Table Components - Contains BarrelInventoryTableWithSelection.tsx, BarrelReceiptsTable.tsx, WarehouseDetailsTableWithSelection.tsx, PurchaseOrderCard.tsx. (2) Configuration via Sub-directories - /free-barrels/, /openLocationsTable/, /BarrelInventoryComponents/, /Actions/. (3) Table Customization Mechanisms - Dynamic Column Configuration via TableHeaderBarrelInventory.tsx with AVAILABLE_SORT_KEYS array, Hierarchical Sorting for storage location columns, Advanced Filtering via lazy-loaded FilterBuilder component, Feature Flags integration (useFeatureFlag hook), URL-based State Management for sort order, pagination, filters. (4) Dynamic Imports - Uses Next.js dynamic() for code-splitting. The 'new' suffix indicates component specialization/composition, not replacement.

---

### Question 46 (Category: cross_cutting_concerns)

How does the ContextService propagate user context across async operations and what data does it carry?

Expected Answer (for scoring):
The ContextService uses AsyncLocalStorage from Node.js to maintain request-scoped context across async operations. It stores userId, sessionId, correlationId, and request metadata. The ContextMiddleware extracts the user ID from req.user and runs the rest of the request handler within the context using contextService.run({ userId }, callback). This allows any service to access getCurrentUserId() or getContext() without explicitly passing user information through method parameters. The AuditService uses ContextService to automatically capture the current user for audit logs. The context is thread-safe and properly propagates through Promise chains and async/await patterns.

---

### Question 47 (Category: architecture_structure)

What is the purpose of the IndexingModule in the providers directory?

Expected Answer (for scoring):
IndexingModule handles vector document indexing for RAG/semantic search capabilities (NOT Elasticsearch or traditional database indexes). It uses BullMQ for job scheduling and LlamaModule for vector embeddings. The service indexes database tables (Barrel, Lot, StorageLocation, BarrelEvent, Customer) into vector documents via LlamaService.upsertDocumentsBatch. Combined with VectorSearchModule (which provides VectorSearchResolver using LlamaModule), this creates a RAG search stack: IndexingModule converts records to vector embeddings -> VectorSearch provides semantic search queries. It does NOT do database index optimization - it specifically generates text content and vector embeddings for AI-powered semantic search.

---

### Question 48 (Category: architecture_structure)

How does the BarrelOemCodeModule support cooperage/manufacturer tracking?

Expected Answer (for scoring):
BarrelOemCodeModule manages barrel manufacturer codes (OEM = Original Equipment Manufacturer). Barrels have barrelOemCode relationships linking to manufacturer/cooperage details. The resolver includes barrelOemCode in barrel queries for reporting which cooperage made each barrel. This supports: quality tracking by cooperage, cost accounting per supplier, and compliance reporting. The threeLetterCode field enables quick identification while full vendor details are accessible via relation.

---

### Question 49 (Category: service_relationships)

What is the relationship between BarrelOwnershipService and GroupTransaction for ownership transfer tracking?

Expected Answer (for scoring):
In bulk transfer methods (bulkTransferOwnership, bulkUpdateOwnership, bulkEstablishJointOwnership): 1) Creates GroupTransaction via createTransactionGroup() with type OWNERSHIP_TRANSFER or ADJUSTMENT; 2) GroupTransaction stores: executedBy, executionDate, notes, metadata with filter criteria, and affectedBarrelIds (initially empty); 3) Individual transfers reference groupId in createOwnershipTransaction() which creates OwnershipTransaction audit records; 4) After processing, the relationship is maintained through the groupId reference in individual transactions. This enables: grouping related transfers for audit, rollback capability, reporting on bulk operations, correlation between OwnershipTransaction records and GroupTransaction. Note: totalBarrels field was removed from GroupTransaction model.

---

### Question 50 (Category: business_logic_constraints)

What validation prevents lot variation from matching canonical lot number?

Expected Answer (for scoring):
LotVariationService.create() enforces three validations: 1) createdBy is required - throws BadRequestException if missing (authentication may have failed). 2) Variation number cannot equal canonical lot number - prevents circular reference where a lot maps to itself. 3) Variation number cannot match existing lot number - throws 'already exists as a canonical lot number. Cannot create a variation that matches an existing lot.' This prevents ambiguity where findLotByNumberOrVariation would find different results depending on lookup order. These rules maintain referential integrity in the lot normalization system.

---

### Question 51 (Category: service_relationships)

How does BarrelOwnershipService.getBulkBarrelOwnerships optimize fetching ownership for multiple barrels?

Expected Answer (for scoring):
getBulkBarrelOwnerships(barrelIds, activeOnly = true): 1) Early return empty array if no IDs; 2) Single query: findMany with where: { barrelId: { in: barrelIds },...(activeOnly && { validTo: null }) }; 3) Includes customer and barrel relations; 4) Orders by validFrom desc; 5) Maps results through normalizeOwnershipType for consistent enum values. Compared to calling getBarrelOwnerships for each barrel: reduces N database queries to 1, maintains same data shape (customer, barrel included), still normalizes ownership types. Used in bulk operations and reporting where multiple barrels' ownerships are needed.

---

### Question 52 (Category: service_relationships)

How does FinanceService handle customer data parsing with defensive JSON handling?

Expected Answer (for scoring):
parseCustomerData(customerData): 1) Early return null if !customerData || !customerData.id; 2) Checks if data is string: calls safeJsonParse(customerData.data, 'customer data'); 3) If already object, uses directly; 4) Validates parsed has required fields (name, code), returns null if missing; 5) Returns merged object with id from parent. safeJsonParse(): try-catch JSON.parse, returns null on failure instead of throwing. This handles: legacy string JSON in JSONB columns, Prisma returning objects vs strings, malformed data without crashing, graceful degradation for display purposes.

---

### Question 53 (Category: data_flow_integration)

What is the data flow when a DumpForRegauge event is processed? How does this differ from other barrel events?

Expected Answer (for scoring):
DumpForRegauge flow in handleDumpForRegaugeEvent(): 1) Extract barrelSN, operator, jsonEntityPayload from data, 2) Find barrel by serialNumber (throws if not found), 3) Lookup 'DumpForRegauge' eventType by name, 4) Special handling: barrel disposition may change to indicate regauge pending, 5) Location may be cleared or set to special regauge holding area, 6) Create BarrelEvent with eventTypeId, notes containing regauge context, 7) Differs from standard events: triggers downstream regauge workflow, 8) May create linked records for regauge tracking, 9) Returns barrelEvent with updated barrel state, 10) Business process: barrel contents dumped for re-measurement/re-grading.

---

### Question 54 (Category: service_relationships)

What services does BarrelEventService depend on for handling barrel entry events at holding locations?

Expected Answer (for scoring):
BarrelEventService constructor injects: 1) PrismaService - database operations; 2) AndroidSyncErrorLoggerService (errorLogger) - logs warnings/errors to DeviceSyncError table; 3) LotVariationService - normalizes serial numbers using lot variations; 4) FeatureFlagsService - controls feature behavior like BarrelReentryMode; 5) CachedEventTypeService - avoids in-transaction EventType queries. In createHoldingLocationEntry(): finds holding location, normalizes serial via lotVariationService.findLotByNumberOrVariation(), finds/creates barrel via createBarrelByBruteforce(), updates barrel location, creates event. Error logging happens outside transaction to prevent deadlocks.

---

### Question 55 (Category: data_flow_integration)

How does the safety limits system prevent runaway inventory processing jobs? Describe the circuit breaker pattern and resource monitoring.

Expected Answer (for scoring):
Safety limits in SafetyLimitsService: 1) checkSafetyLimits() evaluates: batchNumber vs MAX_BATCHES, consecutiveEmptyBatches vs MAX_EMPTY_BATCHES, consecutiveErrors vs MAX_CONSECUTIVE_ERRORS, processingTime vs MAX_PROCESSING_TIME, 2) Returns {canContinue, reason} - processor halts if !canContinue, 3) checkResourceLimits() monitors: memory usage via process.memoryUsage(), shouldAbort if exceeds threshold, 4) Circuit breaker pattern: track consecutiveErrors in sessionMetrics Map, resetCircuitBreaker() on successful batch, 5) emergencyStop() marks session failed, cleans up resources, 6) Processor calls cleanupSessionMetrics() on completion/failure/safety limit to prevent memory leaks, 7) cleanupOldSessionMetrics() periodic cleanup removes entries older than 24 hours, 8) All violations logged to DatabaseProgressService with FATAL logLevel.

---

### Question 56 (Category: architecture_structure)

How does the frontend API routes structure support both proxy and direct backend calls?

Expected Answer (for scoring):
Frontend API routes in app/api/ serve two purposes: graphql-proxy/route.ts forwards authenticated GraphQL requests to the backend with token handling, while other routes (barrel-inventory/, ownership/, transfers/) make direct REST calls to backend endpoints. This hybrid approach uses GraphQL for complex queries needing selective field loading, and REST routes for simpler operations or file uploads. API routes also handle rate limiting and origin validation before forwarding.

---

### Question 57 (Category: architecture_structure)

How does the WarehouseJobsModule demonstrate circular dependency resolution with BarrelModule?

Expected Answer (for scoring):
WarehouseJobsModule uses forwardRef(() => BarrelModule) to defer module resolution, allowing warehouse jobs to query/update barrels. However, BarrelModule does NOT have a reverse forwardRef to WarehouseJobsModule - the circular dependency mentioned in the original answer is not bidirectional. BarrelModule uses forwardRef for AuthModule and BarrelOwnershipModule instead. The enum import pattern is correct: './base/warehouse-jobs.enums' is imported separately to ensure GraphQL enum types are registered before resolvers.

---

### Question 58 (Category: business_logic_constraints)

What is the purpose of bulk operation status tracking?

Expected Answer (for scoring):
EnumBulkOperationStatus tracks multi-record operations: PENDING, RUNNING, COMPLETED, FAILED, CANCELLED, PARTIAL_SUCCESS. BulkOperation model records: operationType, status, targetEntityType, targetEntityIds, operationData (Json), totalRecords, processedCount, successCount, failedCount, results (Json[]), errors (Json[]), startedAt, completedAt, executedBy, correlationId. Used by reconciliation batch operations. BulkOwnershipOperationResponse (separate from BulkOperation) returns totalRequested, successfulUpdates, failedUpdates, successes array, errors array, operationId, completedAt for bulk ownership transfers. These are distinct structures - BulkOperation is a database model while BulkOwnershipOperationResponse is a GraphQL response type.

---

### Question 59 (Category: architecture_structure)

What is the purpose of the BarrelAggregatesModule and how does it optimize dashboard performance?

Expected Answer (for scoring):
BarrelAggregatesModule provides server-side aggregation queries that execute ON-DEMAND, not pre-computed statistics. The module uses efficient PostgreSQL GROUP BY queries with direct SQL and chunked processing (ID_CHUNK_SIZE = 5000) to prevent memory issues. Endpoints include: barrelCountBySize, barrelCountByPeriod, barrelCountByOem, barrelCountByRecipe, inventoryByRecipe, and inventoryAggregates. Results are computed at query time with proper authorization (non-privileged users must provide customer scoping). This improves dashboard performance by pushing aggregation to the database rather than loading millions of records client-side.

---

### Question 60 (Category: business_logic_constraints)

How does the warehouse job aggregated staging work?

Expected Answer (for scoring):
GetAggregatedStagingRecordsDto provides query parameters for filtering staging records: status (InventoryCheckStagingStatus enum: PENDING, PROCESSING, READY, APPROVED, REJECTED, COMMITTED, ERROR), hasIssues (boolean string), issueTypes (comma-separated list like 'InventoryCheck:BarrelNoLabel,InventoryCheck:OrphanBarrelPrior'), issueFilterLogic (AND/OR via IssueFilterLogic enum), issueLevels (comma-separated EnumDeviceSyncErrorLevel values like 'CRITICAL,ERROR,WARN'). ProcessBarrelFillJobDto defines barrel fill job processing with BarrelFillJobItemInput array containing: serialNumber, fillDate, locationPath, fillWeight, harvestTime, harvestedBy, entryTime, entryBy, typeSize, toast, char. ProcessBarrelFillJobResponse returns: jobId, lotNumber, barrelsCreated, barrelIds.

---

### Question 61 (Category: data_flow_integration)

How does the barrel ownership resolution work during event processing? Describe the data flow for determining customer ownership.

Expected Answer (for scoring):
Ownership resolution flow: 1) When processing barrel, include barrelOwnerships relation with customer, 2) Query: barrelOwnerships: { include: { customer: true }, where: { validTo: null } }, 3) validTo: null filters to current/active ownership only, 4) For bruteforce barrel creation: resolve customer via resolveCustomerFromLot(), 5) Lot has default customer relationship, 6) If no customer found: CRITICAL error 'BruteForceCreationFailed:CustomerNotFound', 7) For existing barrels: ownership inherited, no resolution needed, 8) Ownership tracked in BarrelOwnership table: barrelId, customerId, validFrom, validTo, 9) Used for inventory reporting, billing, access control.

---

### Question 62 (Category: data_flow_integration)

Trace the data flow for time zone handling in barrel events. How are Eastern timezone DST transitions handled?

Expected Answer (for scoring):
Timezone handling in BarrelEventService: 1) isEasternDST(year, month, day) determines EDT vs EST, 2) DST rules: starts 2nd Sunday of March 2AM, ends 1st Sunday of November 2AM, 3) getNthSundayOfMonth() calculates specific Sunday day-of-month, 4) For months 0-1, 11 (Jan, Feb, Dec): always EST, 5) For months 3-9 (Apr-Oct): always EDT, 6) For March (month 2): compare day vs secondSunday, 7) For November (month 10): compare day vs firstSunday, 8) Used when creating eventTime: adjusts for -5 (EST) or -4 (EDT) offset, 9) Ensures consistent timestamps regardless of server timezone, 10) Critical for warehouse operations spanning DST transitions.

---

### Question 63 (Category: data_flow_integration)

How does the warehouse job staging system track scan processing status? Describe the state machine and transitions.

Expected Answer (for scoring):
Staging state machine: 1) Initial state: record created in InventoryCheckStaging with status PROCESSING, processingStartedAt set, 2) States: PROCESSING (validation in progress), READY (validation complete, awaiting review), APPROVED (human approved), COMMITTED (changes applied), REJECTED, 3) Transitions: PROCESSING->READY on successful validation, 4) hasIssues flag set based on detected issues, 5) WarehouseJob stats updated: totalScans, pendingReview, issueCount, criticalIssueCount, 6) Job auto-transitions PENDING->IN_PROGRESS on first scan, 7) Stuck detection via processingStartedAt, lastHeartbeat, processingAttempts, 8) Commit flow: READY->APPROVED->COMMITTED, creates actual BarrelEvents, 9) Issues with requiresManualReview=true block approval.

---

### Question 64 (Category: data_flow_integration)

How does the InventoryCheckStaging processor validate barrel scans and detect issues? Describe the complete validation pipeline including BAD_QR, NOLABEL, and duplicate detection.

Expected Answer (for scoring):
The validation pipeline in InventoryCheckStagingProcessor.validateAndDetectIssues(): 1) Phase 0A checks for BAD_QR/NOLABEL patterns using regex (isBadQrScan, isNoLabelScan), creates CRITICAL issues requiring E&T intervention, 2) Phase 0B detects EMPTY_POSITION scans, checks if location has occupying barrel for displacement, 3) Phase 1 validates barrel existence - if not found, attempts lot resolution via resolveLotFromSerialNumber, checks for duplicate lot variations on canonical lot, 4) Phase 1B checks for duplicate barrels with same barrel number on canonical lot, 5) Phase 2 validates location - resolves via resolveLocationFromString, checks occupancy via checkLocationOccupied, 6) Phase 3 finds duplicate scans within same warehouse job via findDuplicateScans, 7) Phase 4 validates serial number format, 8) Issues are persisted to InventoryCheckIssue table with errorLevel (INFO/WARN/ERROR/CRITICAL).

---

### Question 65 (Category: cross_cutting_concerns)

How does the application configure different rate limit tiers for different operation types?

Expected Answer (for scoring):
ThrottlerModule is configured with three tiers: short (10 req/sec for immediate protection), medium (50 req/10sec for sustained activity), and long (200 req/min for overall rate). The GqlThrottlerGuard applies all tiers by default. Individual endpoints can customize using @Throttle({ short: { ttl: 1000, limit: 5 } }) to override specific tiers. @SkipThrottle() bypasses rate limiting entirely (use sparingly). For GraphQL, rate limiting applies per operation. Heavy operations like bulk imports may have stricter limits. The configuration supports named throttlers for different use cases.

---

### Question 66 (Category: service_relationships)

How does the FeatureFlagsService integrate with BarrelEventService to control barrel reentry behavior?

Expected Answer (for scoring):
BarrelEventService injects FeatureFlagsService. The FEATURE_FLAG_KEYS constant defines flags like BarrelReentryMode. During barrel event processing, the service can check featureFlags.isEnabled() or getValue() to determine behavior. For example, BarrelReentryMode may control whether: a barrel can be re-entered at a different location without explicit withdrawal, validation strictness for duplicate entries, automatic conflict resolution strategies. Feature flags enable: runtime behavior changes without deployment, A/B testing of new features, gradual rollout of changes. The service reads from database or external configuration.

---

### Question 67 (Category: service_relationships)

How does CustomerService implement type guards for computed field sorting detection?

Expected Answer (for scoring):
CustomerService has helper functions: 1) hasCustomerNameSort(obj): checks typeof obj === 'object' && obj !== null && 'customerName' in obj && obj.customerName !== undefined; 2) hasTransferDateSort(obj): similar check for 'transferDate'. In findTransfer(): const requiresComputedFieldSort = Array.isArray(args.orderBy) && args.orderBy.some(hasCustomerNameSort || hasTransferDateSort). Type guards enable: TypeScript narrowing after check, safe property access, clear intent in code. When true, delegates to transferService.transfersExtended() which handles raw SQL sorting.

---

### Question 68 (Category: architecture_structure)

What is the purpose of having both HoldingLocationModule and StorageLocationModule?

Expected Answer (for scoring):
StorageLocationModule handles physical warehouse locations (specific positions in ricks). HoldingLocationModule manages temporary holding areas (staging areas, shipping docks, quality inspection areas) that aren't permanent storage. A barrel may move from StorageLocation to HoldingLocation during picking, then back to a different StorageLocation. This separation reflects physical warehouse workflow where barrels transition between permanent storage and temporary holds.

---

### Question 69 (Category: data_flow_integration)

Trace the complete data flow for bruteforce barrel creation when barrel doesn't exist in the system.

Expected Answer (for scoring):
Bruteforce creation flow: 1) BarrelEventService.findOrCreate fails to find barrel, 2) createBarrelByBruteforce() called with transactionClient, serialNumber, data, eventType, locationString, 3) Parse serial to extract lot code, 4) LotVariationService.findLotByNumberOrVariation() finds canonical lot, 5) If no lot: return null (caller creates CRITICAL error), 6) resolveCustomerFromLot() gets default customer, 7) Create Barrel via Prisma: serialNumber, lotId, customerId, disposition based on location type, 8) Create BarrelOwnership record linking barrel to customer, 9) Attach bruteForceWarning to result for caller to log after transaction, 10) Warning logged via AndroidSyncErrorLoggerService with WARN level.

---

### Question 70 (Category: cross_cutting_concerns)

How does the audit log query system support complex filtering and pagination?

Expected Answer (for scoring):
The AuditService provides AuditLogQueryOptions supporting filters: operationType (single or array), entityType (single or array), entityId (single or array), userId (single or array), sessionId, uploadSessionId, correlationId, dateFrom, dateTo, and operationResult. Pagination is handled via skip/take with orderBy options. The queryAuditLogs() method builds Prisma where clauses dynamically based on provided options. Array values become 'in' clauses. Date ranges use 'gte'/'lte'. The service also supports getAuditLogsByCorrelationId() to trace related operations and getEntityHistory() to see all changes to a specific entity over time.

---

### Question 71 (Category: business_logic_constraints)

How does partial ownership transfer work and what happens to remaining percentage?

Expected Answer (for scoring):
BarrelOwnershipService.transferOwnership() supports partial transfers: 1) Gets current ownership for fromCustomerId on barrelId. 2) Calculates percentageToTransfer (defaults to full ownership). 3) Calculates remainingPercentage = current - transfer amount. 4) Ends current ownership by setting validTo = now. 5) Creates new ownership for toCustomerId with transferred percentage. 6) If remainingPercentage > 0, creates new ownership for fromCustomerId preserving original ownershipType, jointOwnershipGroupId, inheritedFromCustomerId, and inheritanceDate. 7) Records TRANSFER transaction with both customer IDs. All within prisma.$transaction for atomicity.

---

### Question 72 (Category: cross_cutting_concerns)

How does the resolution service handle the ACKNOWLEDGE resolution path?

Expected Answer (for scoring):
The ACKNOWLEDGE path marks errors as reviewed without fixing the underlying data. Unlike RESOLVE which changes barrel/location state, ACKNOWLEDGE: 1) Creates a resolution record documenting why the error is being accepted, 2) Optionally creates a subsequent warehouse job for follow-up investigation, 3) Marks the DeviceSyncError as RESOLVED (acknowledged). Use cases include: known data quality issues that can't be fixed immediately, errors during migration, or errors that don't impact operations. The subsequentJob.existingJobId can link to an existing investigation, or newJobData creates a new task. Resolution notes document the rationale.

---

### Question 73 (Category: service_relationships)

What transaction isolation considerations exist when BarrelEventService creates events with location checks?

Expected Answer (for scoring):
createBarrelEventWithLocationAndLocationCheck uses prisma.$androidSyncTransaction or passed tx. The method checks location occupancy before assigning barrel. Considerations: 1) Without serializable isolation, two concurrent events could both see location as empty and both assign; 2) The $androidSyncTransaction uses Serializable isolation with retry; 3) LocationAlreadyOccupiedException thrown if location already occupied; 4) Transaction client (tx) passed through to child operations maintains isolation boundary. The comment 'TEC-1173 FIX' refers to avoiding nested transaction deadlocks by using passed-in tx when available.

---

### Question 74 (Category: service_relationships)

How does ReconciliationService integrate with GroupTransaction for barrel count reconciliation?

Expected Answer (for scoring):
In createReconciliation(): if input.actualData contains barrelReconciliation and entityType is 'GroupTransaction' or 'OwnershipTransactionGroup': 1) Extracts addBarrelIds and removeBarrelIds from barrelReconciliation; 2) Calls reconcileGroupBarrelCount({ reconciliationRecordId, groupId, addBarrelIds, removeBarrelIds, executedBy, notes }); 3) This method updates GroupTransaction.affectedBarrelIds array; 4) On failure, updates ReconciliationRecord with error status and logs via AuditService. This enables: correcting barrel assignments to groups, tracking discrepancies between expected and actual counts, automated or manual reconciliation workflows.

---

### Question 75 (Category: cross_cutting_concerns)

How does the multi-strategy authentication guard determine which authentication method to use?

Expected Answer (for scoring):
Azure AD authentication uses @nestjs/passport with passport-azure-ad BearerStrategy. The strategy validates JWT tokens from Azure AD B2C, extracts user claims, and calls UserRegistrationService.findOrCreateFromAzureProfile() to sync user data. The guard (DefaultAuthGuard) activates the 'azure-ad' strategy. There is NO automatic role mapping from Azure AD groups - roles are managed internally in the User model.

---

### Question 76 (Category: architecture_structure)

How does the BarrelSnapshotModule support point-in-time inventory queries?

Expected Answer (for scoring):
BarrelSnapshotModule provides ON-DEMAND point-in-time inventory queries, NOT periodic snapshots. The service reconstructs historical barrel state by querying the BarrelAudit table and using complex CTEs with ROW_NUMBER() OVER (PARTITION BY barrel ORDER BY changedAt DESC) to find the barrel state at any given pointInTime parameter. It does NOT run scheduled jobs via SchedulerModule - it performs real-time reconstruction from audit logs when queried. The module is still separate from BarrelModule and supports finance reporting through on-demand historical queries.

---

### Question 77 (Category: service_relationships)

How do BullMQ processor options differ between AndroidSyncProcessor and InventoryCheckStagingProcessor, and why?

Expected Answer (for scoring):
AndroidSyncProcessor: concurrency=2 (TEC-839 limit for simultaneous device uploads), lockDuration=120000ms, stalledInterval=30000ms, limiter={max:20, duration:1000} (20 jobs/second). InventoryCheckStagingProcessor: concurrency=5 (higher for staging), lockDuration=120000ms, stalledInterval=30000ms, limiter={max:10, duration:1000}. Differences: 1) Android sync is device-bound (2 devices uploading), staging is not; 2) Android sync has higher rate limit (20 vs 10) for burst handling; 3) Both use same lock/stalled settings for consistency. These settings prevent: device contention, queue bloat, stalled job accumulation while optimizing throughput for each use case.

---

### Question 78 (Category: cross_cutting_concerns)

How does the resolution system handle partial batch failures?

Expected Answer (for scoring):
When resolving multiple device sync errors in a batch, the ResolutionService tracks success/failure per error. For each errorId, it executes the resolution function in a try-catch. Successes are added to succeededErrorIds array. Failures are captured with categorized error information in failedErrors array. After processing all errors, if failedErrors.length > 0, the entire transaction is rolled back - no errors are marked resolved. The exception message includes counts (failed X of Y errors, Z succeeded) and detailed failure information per error including errorId, errorMessage, and errorCategory. This all-or-nothing approach ensures data consistency but requires the user to address failures and retry the entire batch.

---

### Question 79 (Category: service_relationships)

How does BarrelAuditService handle the limitation of lot filtering using current vs historical assignments?

Expected Answer (for scoring):
In getInventoryAtDate() and getInventoryAggregatesAtDate(): 1) If lotNumber filter provided, logs explicit warning: 'Lot filtering uses CURRENT lot assignments. Historical lot assignments are not tracked in BarrelAudit'; 2) The SQL filter uses EXISTS subquery joining current Barrel and Lot tables; 3) Comment explains: 'BarrelAudit doesn't store lotId, and there's no lot-history table'. This limitation exists because: BarrelAudit captures barrel state changes but not lot reassignments, adding lotId to audit would require schema migration, current design prioritizes location/ownership tracking. The warning ensures users understand potential inaccuracy for historical lot queries.

---

### Question 80 (Category: service_relationships)

How does GroupTransactionService validate barrel existence before creating transactions, and what error information does it provide?

Expected Answer (for scoring):
In createLotEntry(), createTransfer(), createInternalMove(): 1) Fetches barrels: tx.barrel.findMany({ where: { id: { in: input.barrelIds } }, select: { id, serialNumber } }); 2) Compares found count vs requested: if (barrels.length !== input.barrelIds.length); 3) Identifies missing: foundIds = barrels.map(b => b.id); missingIds = input.barrelIds.filter(id => !foundIds.includes(id)); 4) Throws: BadRequestException(`Barrels not found: ${missingIds.join(', ')}`). This provides: specific list of missing IDs for debugging, prevents partial transaction creation, fails fast before any writes. The error message enables operators to identify and resolve data issues.

---

### Question 81 (Category: data_flow_integration)

How does the inventory upload processing system handle large CSV files? Describe the complete data transformation pipeline from file upload to database persistence.

Expected Answer (for scoring):
The flow is: 1) Frontend uploads CSV to inventory-upload endpoint, 2) File is parsed and rows are inserted into InventoryUploadStaging table with status 'pending', 3) A Bull job is queued to 'inventory-processing' queue via InventoryProcessingProcessor, 4) Processor processes in batches (configurable batchSize), calling inventoryProcessingService.processBatch(), 5) SQL function processes staging records: validates data, resolves lots via LotVariationService, creates/updates barrels, 6) Progress is tracked via ProgressTrackingService and broadcast via WebSocket ProgressGateway, 7) SafetyLimitsService monitors for resource limits and circuit breaker patterns, 8) DatabaseProgressService logs batch progress and events, 9) On completion, TrustModeProcessingService handles cutoff date processing if enabled.

---

### Question 82 (Category: cross_cutting_concerns)

How does the application handle sensitive data in error messages and logs?

Expected Answer (for scoring):
Several patterns protect sensitive data: 1) Error categorization in business-logic-error-handler.ts categorizes errors into types (RESOURCE_NOT_FOUND, RESOURCE_CONFLICT, PERMISSION_ERROR, VALIDATION_ERROR, DATABASE_CONSTRAINT_ERROR, EXTERNAL_SERVICE_ERROR, UNKNOWN_ERROR) but still includes detailed error messages in output. 2) AuditService.sanitizeForAudit() removes sensitive fields: password, passwordHash, salt, token, secret, apiKey, privateKey from beforeState/afterState. 3) Frontend sanitizeContextForLogging() removes PII from LaunchDarkly context by only exposing kind, key, anonymous, and environment fields. 4) The error handler uses structured logging with context objects containing errorId, errorType, resolutionOption, userId, warehouseJobId.

---

### Question 83 (Category: data_flow_integration)

How does the CachedEventTypeService optimize event type lookups? Describe the caching strategy and its impact on transaction performance.

Expected Answer (for scoring):
EventType caching: 1) CachedEventTypeService caches EventType records to avoid in-transaction queries (TEC-fix), 2) Cache populated on module init or first access, 3) getEventTypeByName() checks cache first, returns cached EventType if found, 4) Cache invalidation on EventType CRUD operations, 5) In BarrelEventService, injected as cachedEventTypeService, 6) Used in createBarrelEventWithLocationAndLocationCheck handlers, 7) Eliminates need for: transactionClient.eventType.findFirst() inside transaction, 8) Reduces transaction duration, preventing lock contention, 9) EventTypes are reference data, rarely change, ideal for caching, 10) TTL-based refresh or manual invalidation on admin updates.

---

### Question 84 (Category: data_flow_integration)

How does the micro-batch orchestrator improve inventory processing performance? Describe the orchestration pattern and data flow.

Expected Answer (for scoring):
Micro-batch orchestration: 1) MicroBatchOrchestrator service manages fine-grained batch processing, 2) Jobs queued as 'process-micro-batch' type with MicroBatchJobData: sessionId, batchNumber, batchSize, startOffset, 3) Processor.processMicroBatch() delegates to orchestrator.processMicroBatch(job), 4) Orchestrator fetches specific range from staging table using offset pagination, 5) Each micro-batch processes subset of records, enabling better progress tracking, 6) Smaller batches allow more frequent progress updates and checkpoints, 7) If micro-batch fails, only that subset retries, 8) Orchestrator coordinates multiple concurrent micro-batches (controlled by concurrency limit), 9) Final aggregation combines all micro-batch results for session completion.

---

### Question 85 (Category: cross_cutting_concerns)

How does the application handle feature flag changes in real-time?

Expected Answer (for scoring):
On the backend, FeatureFlagsService initializes the LaunchDarkly SDK with LaunchDarkly.init() and waits for initialization via waitForInitialization(). There is no explicit streaming connection handling or flag change subscription implemented in the service - it simply evaluates flags on-demand using client.variation(). On the frontend, the mock client (used when LAUNCHDARKLY_CLIENT_ID is not set) supports 'change' event subscriptions via an eventListeners Map, emitting events when identify() is called. The identifyFeatureFlagUser() function dispatches a 'featureflag:identified' CustomEvent via window.dispatchEvent(). Cache clearing in development clears localStorage keys starting with 'ld:' on initialization. There is no periodic polling implemented in the code.

---

### Question 86 (Category: service_relationships)

What cache invalidation strategy do the cached services use, and how is it coordinated?

Expected Answer (for scoring):
Based on patterns: 1) CachedEventTypeService, CachedLotVariationService, CachedStorageLocationService each maintain in-memory cache; 2) warmCache() methods populate on startup; 3) Individual services may have invalidate() or refresh() methods; 4) PrismaService logs whether PubSubProvider is available for distributed invalidation; 5) Comment notes cache invalidation is 'handled manually in cached services'. Current strategy: local invalidation based on service knowledge of when data changes. For distributed: would use PubSubService to publish invalidation events that other instances subscribe to. Reference data (event types) changes rarely, so simple time-based or on-change invalidation suffices.

---

### Question 87 (Category: business_logic_constraints)

What validation prevents invalid customer data references?

Expected Answer (for scoring):
Customer data stored in JSON 'data' field with customerCode, name, etc. populate_barrel_details() validates: SELECT id FROM Customer WHERE data->>'code' = v_customer_code. If not found: v_error_message = 'Customer not found for code: ' || v_customer_code. TransferService extracts customerName via JSON: c.data::jsonb->>'name' or c.data::jsonb->>'customerName'. Invalid customer references prevent: barrel creation (customer required for ownership), transfer completion, financial transactions. Customer validation ensures data integrity for ownership tracking.

---

### Question 88 (Category: service_relationships)

What is the data flow when BarrelOwnershipService processes a partial ownership transfer?

Expected Answer (for scoring):
In transferOwnership() with partial percentage: 1) Find current ownership for fromCustomer/barrel; 2) Calculate: percentageToTransfer = input or current.percentage; remainingPercentage = current - transfer; 3) Within transaction: end current ownership (validTo = now); create new ownership for toCustomer with transfer percentage; if remainingPercentage > 0, create new ownership for fromCustomer with remaining; 4) Call createOwnershipTransaction with TRANSFER type; 5) Database trigger syncs deprecated barrel.customerId. Example: 100% sole owner transfers 30% -> creates 70% ownership for original + 30% for new owner. Both become joint owners if neither has 100%.

---

### Question 89 (Category: cross_cutting_concerns)

How does the feature flag context support multi-context targeting in LaunchDarkly?

Expected Answer (for scoring):
LaunchDarkly integration uses single-context targeting, not multi-context. The context includes kind: 'user', key (userId), and custom attributes (email, environment). There is no organization context or multi-context evaluation. Feature flags are evaluated per-user only.

---

### Question 90 (Category: data_flow_integration)

What is the complete error categorization system for device sync errors? Describe the error levels and their handling.

Expected Answer (for scoring):
Error categorization system: 1) EnumDeviceSyncErrorLevel from Prisma: INFO, WARN, ERROR, CRITICAL, 2) INFO: informational (duplicate scan, location confirmed empty) - no action needed, 3) WARN: attention needed but auto-resolvable (barrel had prior storage, bruteforce creation), 4) ERROR: requires investigation (validation failed, duplicate lot variation), 5) CRITICAL: blocks processing, requires manual review (missing lot, location not found, BAD_QR/NOLABEL), 6) AndroidSyncErrorLoggerService.logError() persists to DeviceSyncErrorLog with: errorLevel, errorType, errorMessage, context JSON, 7) Issues with requiresManualReview=true block commit, 8) resolutionAction hints: DISPLACE, IGNORE, VALIDATE_EMPTY, REQUIRES_E_AND_T_INTERVENTION.

---

### Question 91 (Category: data_flow_integration)

What is the data flow when processing an inspection event? How are repair reasons captured and persisted?

Expected Answer (for scoring):
Inspection event flow in handleInspectionEvent(): 1) Extract barrelSN, locationString, operator, jsonEntityPayload from data, 2) Find barrel by serialNumber, throw if not found, 3) Validate jsonEntityPayload as InspectionPayload using class-validator: requires repairReason string, 4) Lookup 'Inspection' eventType, 5) Create BarrelEvent with: barrelId, eventTypeId, eventTime, createdBy, notes containing stringified payload with repairReason, 6) Barrel location unchanged (inspection doesn't move barrel), 7) InspectionPayload class defines validation: @IsString() repairReason, 8) Validation errors throw BadRequestException with details, 9) Returns barrelEvent and current location info.

---

### Question 92 (Category: cross_cutting_concerns)

How does the application handle concurrent resolution attempts on the same error?

Expected Answer (for scoring):
The resolution system uses database transactions with Prisma to handle concurrency. Within the transaction, it verifies errors are unresolved before proceeding. If two requests try to resolve the same error simultaneously, one will succeed and mark the error as RESOLVED. The second request's check for 'already resolved' errors will find the error already resolved and throw BadRequestException. The transaction's serializable isolation ensures consistent reads. Additionally, the verification step after resolution updates catches race conditions where the status update may have failed silently. This optimistic concurrency control prevents double-resolution without requiring explicit locking.

---

### Question 93 (Category: data_flow_integration)

How does the progress tracking checkpoint system work for large batch processing?

Expected Answer (for scoring):
Checkpoint system: 1) processBatch() returns batchResult with checkpoint object, 2) Checkpoint contains: totalRows, processedSoFar, percentComplete, remainingWork, 3) remainingWork: { pending, validated, totalRemaining }, 4) percentComplete calculated: (processedSoFar / totalRows) * 100, 5) Logged via databaseProgressService.logProcessingEvent with category='checkpoint', 6) Used for: accurate progress bars, continuation decision, debugging, 7) If checkpoint.remainingWork.totalRemaining > 0: continue processing, 8) Checkpoint data more reliable than simple query (handles edge cases), 9) Stored in batch progress record for historical analysis.

---

### Question 94 (Category: service_relationships)

What queue infrastructure does AndroidSyncProcessor use to route InventoryCheck events to the staging queue?

Expected Answer (for scoring):
AndroidSyncProcessor injects the staging queue via @InjectQueue('inventory-check-staging') private readonly stagingQueue: Queue<StagingJobData>. When processing a record, it checks if the associated warehouseJob.requiresPreReconciliation is true AND the event type is 'InventoryCheck'. If both conditions are met, instead of direct processing, it creates a staging job: stagingQueue.add('process-to-staging', {jobId, deviceName, inboxId}). The InventoryCheckStagingProcessor then handles these jobs separately. This routing enables: pre-reconciliation workflow for inventory counts, separation of direct events (Entry, Withdrawal) from staging events, and dedicated concurrency/error handling for staging operations.

---

### Question 95 (Category: architecture_structure)

How does the LoggerModule provide structured logging across the backend?

Expected Answer (for scoring):
LoggerModule integrates nestjs-pino for structured JSON logging via PinoLoggerModule.forRootAsync(). The LoggerConfiguration function (logger.config.ts) configures pino-http with: configurable log levels (fatal, error, warn, info, debug, trace via LOG_LEVEL env), optional pino-pretty transport for development (via PINO_PRETTY env), sensitive key redaction (via SENSITIVE_KEYS env), request/response logging control (via LOG_REQUEST env), and service name mixin. The module is registered globally in AppModule. Note: The implementation uses pino-http (not direct PinoLogger injection in services), and only supports console transport (no file or external service transports are configured).

---

### Question 96 (Category: architecture_structure)

What is the purpose of the AgentModule and how does it relate to AI functionality?

Expected Answer (for scoring):
AgentModule provides AI agent infrastructure, containing claude-agent.service.ts and claude-agent.controller.ts. This indicates a Claude-based AI agent that can perform actions in the warehouse system. Combined with chatbot components and MCP module, this creates an AI assistant architecture: users chat -> agent processes -> agent uses tools (MCP) -> agent returns responses. The agent may perform warehouse queries, generate reports, or assist with decision-making.

---

### Question 97 (Category: architecture_structure)

How does the frontend chatbot component architecture support AI-powered warehouse assistance?

Expected Answer (for scoring):
The components/chatbot directory has an index.ts barrel file and shared/ subdirectory for common components. The dashboard/chat page contains ChatMainPanel.tsx (conversation UI) and ConversationSidebar.tsx (history). Hooks useChatSession.ts and useChatHistory.ts manage state. This connects to backend ChatHistoryModule for persistence and AgentModule for AI processing. The architecture supports streaming responses, tool use, and conversation context management for warehouse-specific queries.

---

### Question 98 (Category: business_logic_constraints)

What business rules govern ownership type normalization, and when should strict mode vs tolerant mode be used?

Expected Answer (for scoring):
The normalizeOwnershipType() method normalizes various DB formats to canonical PascalCase: SOLE_OWNER/SOLEOWNER/SoleOwner -> SoleOwner, JOINT_OWNER/JOINTOWNER/JointOwner -> JointOwner, INHERITED_CHILD/INHERITEDCHILD/InheritedChild -> InheritedChild. Strict mode (strict=true) throws BadRequestException for unrecognized values - use for write/validation paths where data integrity is critical. Tolerant mode (strict=false, default) logs warning and returns SoleOwner as safe default - use for read paths to prevent application crashes when displaying historical data with legacy enum values.

---

### Question 99 (Category: business_logic_constraints)

What constraints prevent overlapping barrel ownerships?

Expected Answer (for scoring):
The ownership system prevents overlapping active ownerships through: 1) Single owner check: When creating sole ownership, system verifies no existing active ownership (validTo IS NULL) exists. 2) Joint ownership: All joint owners must be created atomically with single jointOwnershipGroupId, percentages summing to 100%. 3) Transfer operations: Old ownership's validTo is set before new ownership created. 4) Aggregate validation: createOwnership() checks currentOwnershipTotal + newPercentage <= 100 within transaction. This ensures at any point in time, total ownership percentages for a barrel equal exactly 100%.

---

### Question 100 (Category: business_logic_constraints)

How does the system enforce that barrel ownership percentages sum to 100%, and what tolerance is allowed for floating-point arithmetic?

Expected Answer (for scoring):
The system uses OWNERSHIP_SUM_TOLERANCE = 0.001 defined in barrel/constants.ts for floating-point comparison tolerance. However, the actual validation logic varies: 1) In createOwnership(), the system checks currentTotal + newPercentage > 100 with no tolerance; 2) In establishJointOwnership(), totalPercentage !== 100 is checked with exact equality; 3) In validateOwnershipUpdates() for bulk operations, Math.abs(totalPercentage - 100) > 0.01 is used (0.01 tolerance, not 0.001). The OWNERSHIP_SUM_TOLERANCE constant exists but is not consistently used across all validation paths.

---
