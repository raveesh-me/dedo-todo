-- The Time Tracking module's schema. Only this module's migrations and
-- queries name it, and only its role, time_tracking_app, has privileges on
-- it. A task is referred to by its resource name, never by a foreign key:
-- tasks belong to another module.
CREATE SCHEMA IF NOT EXISTS time_tracking;

DO $$
BEGIN
  CREATE ROLE time_tracking_app NOLOGIN;
EXCEPTION WHEN duplicate_object THEN NULL;
END
$$;

GRANT USAGE ON SCHEMA time_tracking TO time_tracking_app;

-- Running timers. The unique key on (location_id, actor) is the rule that
-- a person has at most one running timer, held even under concurrent
-- starts.
CREATE TABLE time_tracking.timers (
  location_id text        NOT NULL,
  actor       text        NOT NULL,
  id          text        NOT NULL,
  task        text        NOT NULL,
  start_time  timestamptz NOT NULL,
  PRIMARY KEY (location_id, actor, id),
  CONSTRAINT timers_one_per_person UNIQUE (location_id, actor)
);

-- Time entries: written once, never updated.
CREATE TABLE time_tracking.time_entries (
  location_id text        NOT NULL,
  actor       text        NOT NULL,
  id          text        NOT NULL,
  task        text        NOT NULL,
  start_time  timestamptz NOT NULL,
  end_time    timestamptz NOT NULL,
  origin      text        NOT NULL CHECK (origin IN ('TIMER', 'MANUAL')),
  create_time timestamptz NOT NULL,
  PRIMARY KEY (location_id, actor, id),
  CONSTRAINT time_entries_interval CHECK (end_time >= start_time)
);

CREATE INDEX time_entries_by_start ON time_tracking.time_entries (location_id, actor, start_time DESC, id DESC);
CREATE INDEX time_entries_by_task ON time_tracking.time_entries (location_id, actor, task);

ALTER TABLE time_tracking.timers ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_tracking.timers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON time_tracking.timers
  USING (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true))
  WITH CHECK (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true));

ALTER TABLE time_tracking.time_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE time_tracking.time_entries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON time_tracking.time_entries
  USING (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true))
  WITH CHECK (location_id = current_setting('app.location_id', true)
     AND actor = current_setting('app.actor', true));

-- Timers come and go; time entries are only ever inserted or deleted.
GRANT SELECT, INSERT, DELETE ON time_tracking.timers TO time_tracking_app;
GRANT SELECT, INSERT, DELETE ON time_tracking.time_entries TO time_tracking_app;
