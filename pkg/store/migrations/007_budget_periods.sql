ALTER TABLE budget_limits DROP CONSTRAINT budget_limits_check1;
ALTER TABLE budget_limits ADD CONSTRAINT budget_custom_period CHECK(period<>'custom' OR period_end>period_start);
