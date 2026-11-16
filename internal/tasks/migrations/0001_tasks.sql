-- The Tasks module's schema. Only this module's migrations and queries
-- name it, and only its role, tasks_app, has privileges on it.
CREATE SCHEMA IF NOT EXISTS tasks;

DO $$
BEGIN
  CREATE ROLE tasks_app NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL;
END
$$;

GRANT USAGE ON SCHEMA tasks TO tasks_app;

-- A location's table: every key starts with location_id, then the person
-- (actor) whose task it is. A deleted task stays as a tombstone, so a
-- retried DeleteTask with the same request_id succeeds again.
CREATE TABLE tasks.tasks (
  location_id         text        NOT NULL,
  actor               text        NOT NULL,
  id                  text        NOT NULL,
  title               text        NOT NULL,
  notes               text        NOT NULL DEFAULT '',
  state               text        NOT NULL CHECK (state IN ('OPEN', 'DONE')),
  deadline_time       timestamptz,
  create_time         timestamptz NOT NULL,
  update_time         timestamptz NOT NULL,
  complete_time       timestamptz,
  complete_request_id text        NOT NULL DEFAULT '',
  delete_time         timestamptz,
  delete_request_id   text        NOT NULL DEFAULT '',
  PRIMARY KEY (location_id, actor, id)
);

CREATE INDEX tasks_by_create_time ON tasks.tasks (location_id, actor, create_time DESC, id DESC)
  WHERE delete_time IS NULL;

-- Row-level security: a transaction sees and writes only the rows of the
-- location and actor InTenantTx set. Forced, so even the table's owner is
-- held to it; the service's role owns no tables.
ALTER TABLE tasks.tasks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tasks.tasks FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON tasks.tasks
  USING (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true))
  WITH CHECK (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true));

GRANT SELECT, INSERT, UPDATE ON tasks.tasks TO tasks_app;
