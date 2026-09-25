ALTER TABLE model_turns DROP CONSTRAINT model_turns_state_check;
ALTER TABLE model_turns ADD CONSTRAINT model_turns_state_check CHECK(state IN ('dispatched','complete','unknown','failed'));
