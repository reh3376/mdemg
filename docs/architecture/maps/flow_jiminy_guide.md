FLOW|jiminy-guide|v1|Jiminy inner voice guidance pipeline
TRIGGER:claude-hook .claude/hooks/prompt-context.sh,runs:every-prompt
IN:POST /v1/jiminy/guide
→api.handleGuide
→jim.Guide|parallel orchestration,timeout:6s
  ├─cns.Suggest|active constraints from graph
  ├─neo4j.VectorSearch|corrections similar to current prompt
  ├─neo4j.Query|CONTRADICTS edges
  ├─neo4j.Query|frontier nodes:knowledge-gaps,L3+ low-degree
  └─jim.Trust|per-session trust scoring
→jim.Encode|J17 3-tier encoding:
  T1:coded|~15tok|80% traffic|format:C:!|code|ann:val
  T2:telegraphic|~50-100tok|15% traffic|short NL phrases
  T3:full-NL|~200+tok|5% traffic|complete explanation+rationale
→jim.Effectiveness|track guidance_id→GUIDANCE_OUTCOME edge:followed,ignored,contradicted
OUT:data.guidance[],prompt_augmentation,confidence,source_counts
