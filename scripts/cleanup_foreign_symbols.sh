#!/bin/bash
# Cleanup foreign SymbolNodes from mdemg-dev space.
# These are nodes ingested from other codebases (Python venvs, whk-wms, etc.)
# that don't belong in the mdemg-dev space.
#
# Legitimate paths: /internal/, /cmd/, /pkg/, /tests/
# Everything else is foreign contamination.
#
# Usage: bash scripts/cleanup_foreign_symbols.sh [--dry-run]

set -euo pipefail

CONTAINER="mdemg-neo4j-1"
BATCH_SIZE=1000
DRY_RUN=false

if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

echo "=== Foreign SymbolNode Cleanup ==="
echo "Space: mdemg-dev"
echo "Dry run: $DRY_RUN"
echo ""

# Count foreign nodes
FOREIGN=$(docker exec "$CONTAINER" cypher-shell -u neo4j -p testpassword \
    "MATCH (s:SymbolNode {space_id: 'mdemg-dev'})
     WHERE NOT s.file_path STARTS WITH '/internal/'
       AND NOT s.file_path STARTS WITH '/cmd/'
       AND NOT s.file_path STARTS WITH '/pkg/'
       AND NOT s.file_path STARTS WITH '/tests/'
     RETURN count(s) AS cnt" 2>/dev/null | tail -1)

echo "Foreign SymbolNodes found: $FOREIGN"

if [[ "$DRY_RUN" == "true" ]]; then
    echo ""
    echo "Top foreign path prefixes:"
    docker exec "$CONTAINER" cypher-shell -u neo4j -p testpassword \
        "MATCH (s:SymbolNode {space_id: 'mdemg-dev'})
         WHERE NOT s.file_path STARTS WITH '/internal/'
           AND NOT s.file_path STARTS WITH '/cmd/'
           AND NOT s.file_path STARTS WITH '/pkg/'
           AND NOT s.file_path STARTS WITH '/tests/'
         RETURN DISTINCT substring(s.file_path, 0, 50) AS path_prefix, count(s) AS cnt
         ORDER BY cnt DESC LIMIT 15" 2>/dev/null
    echo ""
    echo "Run without --dry-run to delete."
    exit 0
fi

echo ""
echo "Deleting in batches of $BATCH_SIZE..."

TOTAL_DELETED=0
while true; do
    DELETED=$(docker exec "$CONTAINER" cypher-shell -u neo4j -p testpassword \
        "MATCH (s:SymbolNode {space_id: 'mdemg-dev'})
         WHERE NOT s.file_path STARTS WITH '/internal/'
           AND NOT s.file_path STARTS WITH '/cmd/'
           AND NOT s.file_path STARTS WITH '/pkg/'
           AND NOT s.file_path STARTS WITH '/tests/'
         WITH s LIMIT $BATCH_SIZE
         DETACH DELETE s
         RETURN count(*) AS deleted" 2>/dev/null | tail -1)

    if [[ "$DELETED" == "0" ]]; then
        break
    fi

    TOTAL_DELETED=$((TOTAL_DELETED + DELETED))
    echo "  Batch deleted: $DELETED (total: $TOTAL_DELETED)"
done

echo ""
echo "Done. Total deleted: $TOTAL_DELETED"

# Verify
REMAINING=$(docker exec "$CONTAINER" cypher-shell -u neo4j -p testpassword \
    "MATCH (s:SymbolNode {space_id: 'mdemg-dev'}) RETURN count(s) AS cnt" 2>/dev/null | tail -1)
echo "Remaining SymbolNodes in mdemg-dev: $REMAINING"
