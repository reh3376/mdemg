-- Migration 010: Fix schema version tracking.
-- Migrations 008 and 009 omitted the schema_version update.
-- This corrects it to reflect the actual applied state.

UPDATE tsdb_schema_meta SET value = '10' WHERE key = 'schema_version';
