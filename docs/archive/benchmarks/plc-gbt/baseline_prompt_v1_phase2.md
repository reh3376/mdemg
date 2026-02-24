# PHASE 2: TEST QUESTIONS (100 total)

## INSTRUCTIONS

You have completed ingestion. Now answer these 100 questions.

### RULES

- You MAY access /Users/reh3376/repos/plc-gbt/ to verify/lookup information
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

### Question 1 (Category: ai_ml_integration, Difficulty: medium)

What is the trainer_callback.py module used for in the OpenAI fine-tuning workflow?

---

### Question 2 (Category: data_models_schema, Difficulty: hard)

In the CascadeConfiguration interface, what are the two possible values for cascade_type and what additional fields are used for each type?

---

### Question 3 (Category: control_loop_architecture, Difficulty: hard)

What is the default value and valid range for the overshoot_limit parameter in master loop tuning configuration?

---

### Question 4 (Category: n8n_workflow, Difficulty: medium)

What PostgreSQL database name is used for N8N workflows in docker-compose?

---

### Question 5 (Category: configuration_infrastructure, Difficulty: medium)

What is the default cache TTL (time-to-live) in seconds for the CLI?

---

### Question 6 (Category: configuration_infrastructure, Difficulty: easy)

What is the OpenAI Seed value configured for deterministic responses in plc-gbt?

---

### Question 7 (Category: api_services, Difficulty: hard)

What temperature value does the OpenAIService use when calling the chat completion API for truth-seeking responses?

---

### Question 8 (Category: data_models_schema, Difficulty: hard)

What are the four schema organization directories under plc-gbt-stack/schemas/control-loops/subtypes/?

---

### Question 9 (Category: database_persistence, Difficulty: hard)

What is the maximum number of command results stored in CLIExecutor's command_history?

---

### Question 10 (Category: data_models_schema, Difficulty: medium)

What are the four ValidationLevel values in plc-gbt and what does each level validate?

---

### Question 11 (Category: security_authentication, Difficulty: medium)

What port does the mTLS proxy expose in the docker-compose configuration for secure API access?

---

### Question 12 (Category: acd_l5x_conversion, Difficulty: medium)

Which controller family has the highest I/O module capacity?

---

### Question 13 (Category: ui_architecture, Difficulty: conceptual)

How does the plc-gbt UI implement a flexible, user-customizable layout system?

---

### Question 14 (Category: data_models_schema, Difficulty: easy)

What is the regex pattern used for ProcessVariable tag_name validation in plc-gbt schemas?

---

### Question 15 (Category: ai_ml_integration, Difficulty: medium)

What is the default OpenAI model used for fine-tuning in plc-gbt, and what base model is it derived from?

---

### Question 16 (Category: ui_ux, Difficulty: medium)

What toast notification library is used in the Providers component, and where is its default position configured?

---

### Question 17 (Category: control_loop_architecture, Difficulty: hard)

What is the default response_ratio for slave loop tuning, and what are its minimum and maximum allowed values?

---

### Question 18 (Category: database_persistence, Difficulty: medium)

What database is used for Neo4j knowledge graph storage in the RAG architecture?

---

### Question 19 (Category: acd_l5x_conversion, Difficulty: hard)

What are the three Rockwell/Allen-Bradley controller families supported by plc-gbt?

---

### Question 20 (Category: api_services, Difficulty: medium)

What are the four ExecutionStatus enum values defined in cli_api_bridge.py?

---

### Question 21 (Category: api_services, Difficulty: medium)

What is the maximum number of command results stored in the CLIExecutor's command_history before older entries are trimmed?

---

### Question 22 (Category: system_architecture, Difficulty: conceptual)

How does the cascade control configuration in plc-gbt enable complex multi-loop control strategies?

---

### Question 23 (Category: ai_ml_integration, Difficulty: medium)

What temperature value is used in the OpenAI chat completion API to ensure deterministic responses?

---

### Question 24 (Category: n8n_workflow, Difficulty: hard)

What three workflow priority levels are defined in WorkflowMetadata?

---

### Question 25 (Category: integration, Difficulty: conceptual)

What PLC controller families does plc-gbt support and what are their key capability differences?

---

### Question 26 (Category: api_services, Difficulty: easy)

What is the CLI version constant (CLI_VERSION) defined in the plc_control_loop_cli.py file?

---

### Question 27 (Category: ai_ml_integration, Difficulty: hard)

What is the max_tokens parameter typically used in OpenAI completions for PLC assistance queries?

---

### Question 28 (Category: acd_l5x_conversion, Difficulty: hard)

What are the capacity differences between ControlLogix and Micro800 controllers in terms of max programs and tags?

---

### Question 29 (Category: ui_ux, Difficulty: hard)

In the Providers component, what is the default staleTime configured for React Query queries, and what is the gcTime (garbage collection time)?

---

### Question 30 (Category: acd_l5x_conversion, Difficulty: medium)

How does plc-gbt bridge process control engineers and software developers?

---

### Question 31 (Category: ui_ux, Difficulty: hard)

What drag-and-drop library is used for the IconStrip component's icon reordering functionality, and what two sensors are configured?

---

### Question 32 (Category: acd_l5x_conversion, Difficulty: medium)

Why are .ACD files unsuitable for traditional version control systems?

---

### Question 33 (Category: database_persistence, Difficulty: medium)

What is the default chunk_size in bytes for file processing in FileStorageService?

---

### Question 34 (Category: control_loop_architecture, Difficulty: easy)

What are the four PID controller types defined in the plc-gbt system?

---

### Question 35 (Category: api_services, Difficulty: easy)

What is the default timeout value (in seconds) for the CommandRequest model's timeout field in the CLI-to-API Bridge?

---

### Question 36 (Category: acd_l5x_conversion, Difficulty: easy)

What is the core purpose of plc-gbt's ACD to L5X conversion capability?

---

### Question 37 (Category: business_logic_workflows, Difficulty: hard)

What validation level includes simulation testing but not PLC connectivity validation?

---

### Question 38 (Category: ai_ml_integration, Difficulty: easy)

What environment variable stores the OpenAI API key for AI service authentication?

---

### Question 39 (Category: data_models_schema, Difficulty: hard)

What are all seven InstanceType enum values in plc-gbt?

---

### Question 40 (Category: n8n_workflow, Difficulty: hard)

What is the relationship between n8n-framework and plc-gbt event handling?

---

### Question 41 (Category: ai_ml_integration, Difficulty: hard)

What OpenAI seed value is configured in plc-gbt for reproducible responses?

---

### Question 42 (Category: acd_l5x_conversion, Difficulty: hard)

What DevOps capabilities are enabled by converting .ACD to .L5X format?

---

### Question 43 (Category: n8n_workflow, Difficulty: medium)

What type of workflows can be automated using n8n in the plc-gbt stack?

---

### Question 44 (Category: business_logic_workflows, Difficulty: hard)

What are the four PID parameters that MUST be present for BASIC_PID instance type to pass STANDARD validation level?

---

### Question 45 (Category: data_models_schema, Difficulty: medium)

List all InstanceStatus enum values in plc-gbt in the order they appear in the code.

---

### Question 46 (Category: business_logic_workflows, Difficulty: hard)

In the OpenAI service system prompt, what are the three critical behavior requirements listed (keywords only)?

---

### Question 47 (Category: business_logic_workflows, Difficulty: medium)

What database column is used to implement soft delete functionality in the file storage service?

---

### Question 48 (Category: configuration_infrastructure, Difficulty: hard)

What is the full path to the CLI configuration file location?

---

### Question 49 (Category: control_loop_architecture, Difficulty: medium)

What are the three scaling_type enum values available for setpoint scaling between master and slave loops?

---

### Question 50 (Category: ui_ux, Difficulty: medium)

What are the three layout presets available in the useLayoutStore's applyLayoutPreset action?

---

### Question 51 (Category: data_models_schema, Difficulty: medium)

What JSON Schema draft version is used in plc-gbt control loop schemas?

---

### Question 52 (Category: security_authentication, Difficulty: medium)

What is the default session timeout value for the CLI security session?

---

### Question 53 (Category: integration, Difficulty: conceptual)

How does plc-gbt's n8n integration enable workflow automation for industrial processes?

---

### Question 54 (Category: security_authentication, Difficulty: medium)

What soft delete mechanism is used in the file storage service for audit compliance?

---

### Question 55 (Category: data_models_schema, Difficulty: medium)

In plc-gbt, what keyword is used in JSON schemas to reference base schemas for inheritance?

---

### Question 56 (Category: acd_l5x_conversion, Difficulty: easy)

What file format is .L5X based on?

---

### Question 57 (Category: business_logic_workflows, Difficulty: easy)

What PLC connection mode is enforced by default in plc-gbt for safety reasons?

---

### Question 58 (Category: configuration_infrastructure, Difficulty: hard)

Which PostgreSQL database name is used for N8N workflows according to docker-compose.yml?

---

### Question 59 (Category: ui_ux, Difficulty: easy)

What four types of widgets are supported in the analytics store Dashboard interface?

---

### Question 60 (Category: ui_ux, Difficulty: hard)

In the ControlLoopGrid component, how many columns does the responsive grid display at different breakpoints (default, md, lg, xl)?

---

### Question 61 (Category: api_services, Difficulty: hard)

What is the default fine-tuned OpenAI model ID used by the OpenAIService when no OPENAI_MODEL environment variable is set?

---

### Question 62 (Category: configuration_infrastructure, Difficulty: easy)

What port does the mTLS proxy expose in the docker-compose configuration?

---

### Question 63 (Category: data_architecture, Difficulty: conceptual)

How does plc-gbt ensure data integrity for file storage operations?

---

### Question 64 (Category: security_authentication, Difficulty: hard)

What decorator is used in the CLI to enforce permission levels for protected operations?

---

### Question 65 (Category: n8n_workflow, Difficulty: medium)

What is the purpose of the WorkflowState type in workflow management?

---

### Question 66 (Category: control_loop_architecture, Difficulty: medium)

In cascade control configuration, what are the four communication_method enum values that define how master-slave loops communicate?

---

### Question 67 (Category: business_logic_workflows, Difficulty: easy)

What is the correct lifecycle state progression from DRAFT to DEPLOYED?

---

### Question 68 (Category: database_persistence, Difficulty: hard)

What is the default FILE_STORAGE_ROOT path for the CLI API bridge file storage?

---

### Question 69 (Category: system_purpose, Difficulty: conceptual)

What is the core purpose of plc-gbt and how does it provide a novel solution to process control and automation version control?

---

### Question 70 (Category: system_architecture, Difficulty: conceptual)

How does plc-gbt integrate AI assistance with traditional PLC programming workflows?

---

### Question 71 (Category: business_logic_workflows, Difficulty: easy)

What is the default instance status when creating a new control loop instance via the wizard?

---

### Question 72 (Category: control_loop_architecture, Difficulty: easy)

What are the six control modes defined in the ControlMode type?

---

### Question 73 (Category: api_services, Difficulty: medium)

What hashing algorithm does the FileStorageService use to calculate file checksums?

---

### Question 74 (Category: acd_l5x_conversion, Difficulty: hard)

Which controller family supports motion control capabilities?

---

### Question 75 (Category: acd_l5x_conversion, Difficulty: medium)

What is the external library used for ACD to L5X conversion in plc-gbt?

---

### Question 76 (Category: ai_ml_integration, Difficulty: medium)

What is the purpose of the neural_tuning.py module in the plc-gbt AI integration?

---

### Question 77 (Category: data_models_schema, Difficulty: easy)

What are the possible values for ProcessVariable quality in the plc-gbt control-loop.types.ts file?

---

### Question 78 (Category: ai_ml_integration, Difficulty: medium)

How does plc-gbt's AI assistant handle domain-specific PLC terminology in responses?

---

### Question 79 (Category: api_services, Difficulty: medium)

What is the default chunk_size (in bytes) used by the FileStorageService for file operations?

---

### Question 80 (Category: safety_security, Difficulty: conceptual)

What validation levels does plc-gbt provide and when would you use each?

---

### Question 81 (Category: business_logic_workflows, Difficulty: medium)

What are the four communication methods supported for cascade master-slave loop coordination?

---

### Question 82 (Category: ui_ux, Difficulty: easy)

What is the name of the Zustand store used for managing layout state in the plc-gbt UI, and what is its localStorage persistence key?

---

### Question 83 (Category: configuration_infrastructure, Difficulty: medium)

What is the default request timeout in seconds for the CLIExecutor class?

---

### Question 84 (Category: n8n_workflow, Difficulty: easy)

What is the purpose of the n8n-mcp module in the plc-gbt stack?

---

### Question 85 (Category: ui_ux, Difficulty: medium)

What are the seven possible values for the ToolType type in the layout store?

---

### Question 86 (Category: system_architecture, Difficulty: conceptual)

What is the instance lifecycle in plc-gbt and how does it track PLC program states?

---

### Question 87 (Category: ui_ux, Difficulty: medium)

What two Google fonts are loaded in the root layout of the plc-gbt Next.js application?

---

### Question 88 (Category: control_loop_architecture, Difficulty: medium)

What is the default stability_margin value for slave loop tuning, and what are its minimum and maximum constraints?

---

### Question 89 (Category: ai_ml_integration, Difficulty: hard)

What RAG (Retrieval-Augmented Generation) database is used to store PLC program context for AI assistance?

---

### Question 90 (Category: ui_ux, Difficulty: hard)

In the ControlLoopStats component, what are the three color thresholds for the avgPerformance value that determine whether it displays as green, yellow, or red?

---

### Question 91 (Category: configuration_infrastructure, Difficulty: medium)

What is the default session timeout value in seconds for the CLI?

---

### Question 92 (Category: control_loop_architecture, Difficulty: hard)

What is the valid range for response_time_target in master loop tuning parameters?

---

### Question 93 (Category: api_services, Difficulty: medium)

What are the five fields defined in the APIResponse Pydantic model in cli_api_bridge.py?

---

### Question 94 (Category: configuration_infrastructure, Difficulty: medium)

What is the default port for the backend API in plc-gbt?

---

### Question 95 (Category: control_loop_architecture, Difficulty: medium)

What is the maximum character length allowed for a tag_name in the plc-gbt system, and what regex pattern must it match?

---

### Question 96 (Category: configuration_infrastructure, Difficulty: hard)

What is the default FILE_STORAGE_ROOT path configured in the CLI API bridge?

---

### Question 97 (Category: api_services, Difficulty: hard)

In the CLIConfiguration dataclass, what is the default value for session_timeout and what unit is it measured in?

---

### Question 98 (Category: security_authentication, Difficulty: hard)

What validation level includes simulation testing before deployment to verify safe operation?

---

### Question 99 (Category: security_authentication, Difficulty: easy)

What connection mode is enforced by default in plc-gbt to prevent accidental PLC modifications?

---

### Question 100 (Category: security_authentication, Difficulty: medium)

What permission level is required to delete a control loop instance?

---

## FINAL REPORT (REQUIRED)

After answering ALL questions, record END_TIME and provide:

```
=== BASELINE TEST v1 RESULTS ===
START_TIME: [from Phase 1]
INGESTION_COMPLETE_TIME: [from Phase 1]
END_TIME: [now - run: date "+%Y-%m-%d %H:%M:%S"]
TOTAL_ELAPSED_TIME: [calculate]
INGESTION_TIME: [calculate]
QUESTION_TIME: [calculate]

FILES_EXPECTED: 12370
FILES_VERIFIED: [output of wc -l command from Phase 1]

ANSWERS_FROM_MEMORY: [count]
ANSWERS_FROM_LOOKUP: [count]
ANSWERS_FROM_BOTH: [count]

TOTAL_SCORE: [sum of scores]
SCORE_BY_CATEGORY:
  - acd_l5x_conversion: [X/Y]
  - ai_ml_integration: [X/Y]
  - api_services: [X/Y]
  - business_logic_workflows: [X/Y]
  - configuration_infrastructure: [X/Y]
  - control_loop_architecture: [X/Y]
  - data_architecture: [X/Y]
  - data_models_schema: [X/Y]
  - database_persistence: [X/Y]
  - integration: [X/Y]
  - n8n_workflow: [X/Y]
  - safety_security: [X/Y]
  - security_authentication: [X/Y]
  - system_architecture: [X/Y]
  - system_purpose: [X/Y]
  - ui_architecture: [X/Y]
  - ui_ux: [X/Y]

CONTEXT_RETENTION_NOTES:
  - [what did you remember vs forget?]
  - [which categories were harder from memory?]
===
```
