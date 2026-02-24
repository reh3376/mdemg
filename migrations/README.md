# Cypher migrations

These migrations are designed to be:

- **idempotent** (`IF NOT EXISTS` wherever supported)
- **append-only** in their history log
- safe to run at service startup, but typically executed by CI/CD or an ops job

Apply in lexicographic order:

```bash
for f in migrations/V*.cypher; do
  cypher-shell -a "$NEO4J_URI" -u "$NEO4J_USER" -p "$NEO4J_PASS" -f "$f";
done
```

Schema versioning is tracked by:

- `(:SchemaMeta {key:'schema'})` with `current_version`
- `(:Migration {version})` nodes for audit
