ALTER TABLE autopilot_requests ADD COLUMN continuous boolean NOT NULL DEFAULT false;
ALTER TABLE autopilot_requests ADD COLUMN served_at timestamptz NOT NULL DEFAULT '-infinity';
