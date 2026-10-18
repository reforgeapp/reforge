ALTER TABLE autopilot_attempts ADD COLUMN platform_retries integer NOT NULL DEFAULT 0 CHECK(platform_retries BETWEEN 0 AND 2);
ALTER TABLE autopilot_attempts DROP CONSTRAINT autopilot_attempts_blocker_kind_check;
ALTER TABLE autopilot_attempts ADD CONSTRAINT autopilot_attempts_blocker_kind_check CHECK(blocker_kind IN ('','approve','reconnect','raise_limit','restore_runner','unsupported','exhausted','platform'));
