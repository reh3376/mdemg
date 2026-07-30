#!/usr/bin/env bash
set -euo pipefail
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; CYAN='\033[0;36m'; NC='\033[0m'
PASS() { echo -e "  ${GREEN}✅ PASS${NC}: $1"; }
FAIL() { echo -e "  ${RED}❌ FAIL${NC}: $1"; FAILURES=$((FAILURES+1)); }
WARN() { echo -e "  ${YELLOW}⚠️  WARN${NC}: $1"; }
INFO() { echo -e "  ${CYAN}ℹ️  INFO${NC}: $1"; }
Q() { docker compose exec -T timescaledb psql -U mdemg -d mdemg_metrics -tAc "$1" 2>/dev/null; }
FAILURES=0
echo ""; echo "=== MDEMG TSDB Data Spot Check ==="; echo ""
echo "── 1. Schema ──"
SCHEMA=$(Q "SELECT value FROM tsdb_schema_meta WHERE key='schema_version';")
[ "$SCHEMA" = "11" ] && PASS "Schema version: $SCHEMA" || FAIL "Schema version: $SCHEMA (expected 11)"
echo ""; echo "── 2. Row Counts ──"
for t in metric_samples llm_interactions embedding_events retrieval_events; do
  c=$(Q "SELECT count(*) FROM $t;"); [ "$c" -gt 0 ] 2>/dev/null && PASS "$t: $c rows" || FAIL "$t: 0 rows"
done
for t in ft_training_cycles ft_model_versions; do
  c=$(Q "SELECT count(*) FROM $t;"); [ "$c" = "0" ] && PASS "$t: empty (expected)" || WARN "$t: $c rows"
done
# ft_benchmarks + ft_hitl_decisions dropped in V0032 (FT-DORMANT-CLEANUP-001)
# — superseded by benchmark_runs (V0012) + review_grades (V0028).
echo ""; echo "── 3. metric_samples ──"
MS=$(Q "SELECT count(*) FROM metric_samples;")
if [ "$MS" -gt 0 ] 2>/dev/null; then
  INFO "Distinct metrics: $(Q "SELECT count(DISTINCT metric_name) FROM metric_samples;")"
  echo "  Top 10:"; Q "SELECT metric_name, count(*) FROM metric_samples GROUP BY metric_name ORDER BY count DESC LIMIT 10;" | while IFS='|' read -r n c; do printf "    %-50s %s\n" "$n" "$c"; done
  INFO "Spaces: $(Q "SELECT string_agg(DISTINCT space_id, ', ') FROM metric_samples;")"
  INFO "Latest: $(Q "SELECT max(time)::text FROM metric_samples;")"
fi
echo ""; echo "── 4. llm_interactions ──"
LLM=$(Q "SELECT count(*) FROM llm_interactions;")
if [ "$LLM" -gt 0 ] 2>/dev/null; then
  echo "  Tasks:"; Q "SELECT task_name, count(*) FROM llm_interactions GROUP BY task_name ORDER BY count DESC;" | while IFS='|' read -r n c; do printf "    %-40s %s\n" "$n" "$c"; done
  echo "  Models:"; Q "SELECT provider, model_name, count(*) FROM llm_interactions GROUP BY provider, model_name ORDER BY count DESC;" | while IFS='|' read -r p m c; do printf "    %-10s %-30s %s\n" "$p" "$m" "$c"; done
  INFO "Error rate: $(Q "SELECT ROUND(100.0*count(CASE WHEN error!='' AND error IS NOT NULL THEN 1 END)/NULLIF(count(*),0),1) FROM llm_interactions;")%"
  for col in task_name system_prompt user_prompt response model_name provider; do
    e=$(Q "SELECT count(*) FROM llm_interactions WHERE $col IS NULL OR $col='';")
    [ "$e" = "0" ] && PASS "$col: 100% populated" || WARN "$col: $e empty"
  done
  echo "  Scrub check (last 50):"; UK=$(Q "SELECT count(*) FROM (SELECT * FROM llm_interactions ORDER BY time DESC LIMIT 50) s WHERE user_prompt~'sk-[a-zA-Z0-9]{20}' OR response~'sk-[a-zA-Z0-9]{20}';")
  [ "$UK" = "0" ] && PASS "No API keys in prompts/responses" || FAIL "API keys found in $UK rows"
  INFO "RAFT context: $(Q "SELECT count(*) FROM llm_interactions WHERE retrieval_node_ids IS NOT NULL AND array_length(retrieval_node_ids,1)>0;") rows populated"
  INFO "Prompt hashes: $(Q "SELECT count(DISTINCT system_prompt_hash) FROM llm_interactions WHERE system_prompt_hash IS NOT NULL AND system_prompt_hash!='';") distinct"
  INFO "Think mode: $(Q "SELECT count(*) FROM llm_interactions WHERE think_content IS NOT NULL AND think_content!='';") rows"
else WARN "llm_interactions: 0 rows (need LLM calls to populate)"; fi
echo ""; echo "── 5. embedding_events ──"
EMB=$(Q "SELECT count(*) FROM embedding_events;")
if [ "$EMB" -gt 0 ] 2>/dev/null; then
  echo "  Types:"; Q "SELECT event_type, count(*) FROM embedding_events GROUP BY event_type ORDER BY count DESC;" | while IFS='|' read -r t c; do printf "    %-15s %s\n" "$t" "$c"; done
  echo "  Call sites:"; Q "SELECT call_site, count(*) FROM embedding_events GROUP BY call_site ORDER BY count DESC;" | while IFS='|' read -r s c; do printf "    %-25s %s\n" "$s" "$c"; done
  INFO "Cache hit: $(Q "SELECT ROUND(100.0*count(CASE WHEN cached THEN 1 END)/NULLIF(count(*),0),1) FROM embedding_events;")%"
  QTS=$(Q "SELECT count(*) FROM (SELECT * FROM embedding_events WHERE query_text IS NOT NULL AND query_text!='' LIMIT 20) s WHERE query_text LIKE '%[REDACTED%';")
  [ "$QTS" = "0" ] && PASS "query_text NOT scrubbed (correct)" || FAIL "query_text scrubbed ($QTS rows)"
  echo "  Models:"; Q "SELECT model_name, dimensions, count(*) FROM embedding_events GROUP BY model_name, dimensions;" | while IFS='|' read -r m d c; do printf "    %-35s dims=%-5s %s\n" "$m" "$d" "$c"; done
else WARN "embedding_events: 0 rows (need embed calls)"; fi
echo ""; echo "── 6. retrieval_events ──"
RET=$(Q "SELECT count(*) FROM retrieval_events;")
if [ "$RET" -gt 0 ] 2>/dev/null; then
  echo "  Call sites:"; Q "SELECT call_site, count(*) FROM retrieval_events GROUP BY call_site ORDER BY count DESC;" | while IFS='|' read -r s c; do printf "    %-25s %s\n" "$s" "$c"; done
  INFO "Recall: $(Q "SELECT count(*) FROM retrieval_events WHERE recall_node_ids IS NOT NULL AND array_length(recall_node_ids,1)>0;")/$RET"
  INFO "BM25:   $(Q "SELECT count(*) FROM retrieval_events WHERE bm25_node_ids IS NOT NULL AND array_length(bm25_node_ids,1)>0;")/$RET"
  INFO "Rerank: $(Q "SELECT count(*) FROM retrieval_events WHERE rerank_node_ids IS NOT NULL AND array_length(rerank_node_ids,1)>0;")/$RET"
  INFO "Result: $(Q "SELECT count(*) FROM retrieval_events WHERE result_node_ids IS NOT NULL AND array_length(result_node_ids,1)>0;")/$RET"
  INFO "guidance_id: $(Q "SELECT count(*) FROM retrieval_events WHERE guidance_id IS NOT NULL AND guidance_id!='';")/$RET"
else WARN "retrieval_events: 0 rows (need retrieval queries)"; fi
echo ""; echo "=== SUMMARY ==="
[ "$FAILURES" -eq 0 ] && echo -e "${GREEN}All critical checks passed.${NC}" || echo -e "${RED}$FAILURES critical failure(s).${NC}"
echo "  metric_samples:   ${MS:-0}"; echo "  llm_interactions:  ${LLM:-0}"; echo "  embedding_events:  ${EMB:-0}"; echo "  retrieval_events:  ${RET:-0}"
if [ "${LLM:-0}" = "0" ] && [ "${EMB:-0}" = "0" ] && [ "${RET:-0}" = "0" ]; then
  echo -e "\n${YELLOW}Training tables empty. Trigger activity:${NC}"
  echo "  mdemg ingest /path/to/project --space-id test"
  echo "  curl -X POST http://localhost:9999/v1/memory/consult -H 'Content-Type: application/json' -d '{\"query\":\"test\",\"space_id\":\"test\",\"llm_synthesis\":true}'"
fi
exit $FAILURES
