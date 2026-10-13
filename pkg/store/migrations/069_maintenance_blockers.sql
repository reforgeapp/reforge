ALTER TABLE autopilot_attempts ADD COLUMN blocker_kind text NOT NULL DEFAULT '' CHECK(blocker_kind IN ('','approve','reconnect','raise_limit','restore_runner','unsupported','exhausted'));
ALTER TABLE autopilot_attempts ADD COLUMN max_runs integer NOT NULL DEFAULT 6 CHECK(max_runs BETWEEN 6 AND 100);
ALTER TABLE autopilot_attempts ADD COLUMN merge_needs_human boolean NOT NULL DEFAULT false;
ALTER TABLE autopilot_bot_merges ADD COLUMN needs_human boolean NOT NULL DEFAULT false;
UPDATE autopilot_attempts SET blocker_kind=CASE WHEN runs>=6 THEN 'exhausted' ELSE 'unsupported' END WHERE outcome='blocked';
UPDATE autopilot_attempts SET merge_needs_human=true WHERE merge_reason LIKE 'Awaiting human%';
