FLOW|rsic-cycle|v1|RSIC self-improvement cycle:assess,reflect,plan,dispatch,calibrate
TRIGGER:manual_api|micro_auto|session_periodic|macro_cron|watchdog_force
IN:POST /v1/self-improve/cycle
→ape.RunCycle|5-stage,tier-gated
  S1:ape.Assess|7-dim health(retrieval,memory,edge,task,guidance,protocol,synergy)
  S2:ape.Reflect|20 patterns+optional LLM reflector
  S3:ape.Plan|prioritized task generation from insights
  S4:ape.Dispatch|13 action types,safety-gated
    actions:prune_decayed_edges|prune_excess_edges|trigger_consolidation|
    graduate_volatile|tombstone_stale|refresh_stale_edges|codify_constraint|
    retire_code|adjust_tier_threshold|adjust_replay_buffer|
    review_guidance_effectiveness|adjust_guidance_confidence|archive_ineffective_constraints
  S5:ape.Calibrate|validate+auto-rollback if criteria unmet
TIERS:micro=30s|meso=10m|macro=30m
SAFETY:dry-run+snapshot+rollback+blast-radius+protected-spaces
