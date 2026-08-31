CREATE INDEX rule_outbox_retention_idx ON rule_outbox_events (published_at, id) WHERE published_at IS NOT NULL;
