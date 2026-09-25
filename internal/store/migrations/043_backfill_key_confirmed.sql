ALTER TABLE connections NO FORCE ROW LEVEL SECURITY;
ALTER TABLE model_turns NO FORCE ROW LEVEL SECURITY;
ALTER TABLE budget_reservations NO FORCE ROW LEVEL SECURITY;
UPDATE connections c SET key_confirmed_at = s.confirmed
FROM (
    SELECT m.org_id, (r.record->>'connection_id')::uuid AS connection_id, max(m.completed_at) AS confirmed
    FROM model_turns m JOIN budget_reservations r ON r.org_id = m.org_id AND r.id = m.reservation_id
    WHERE m.state = 'complete'
    GROUP BY 1, 2
) s
WHERE c.org_id = s.org_id AND c.id = s.connection_id AND c.key_confirmed_at IS NULL;
ALTER TABLE connections FORCE ROW LEVEL SECURITY;
ALTER TABLE model_turns FORCE ROW LEVEL SECURITY;
ALTER TABLE budget_reservations FORCE ROW LEVEL SECURITY;
