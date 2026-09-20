CREATE TABLE inventory_scheduler_cursors (
    id uuid PRIMARY KEY CHECK(id='00000000-0000-4000-8000-000000000011'::uuid),
    org_id uuid NOT NULL
);
ALTER TABLE inventory_scheduler_cursors ENABLE ROW LEVEL SECURITY;
ALTER TABLE inventory_scheduler_cursors FORCE ROW LEVEL SECURITY;
CREATE POLICY scheduler_cursor_read ON inventory_scheduler_cursors FOR SELECT USING (true);
CREATE POLICY scheduler_cursor_write ON inventory_scheduler_cursors FOR ALL USING (current_setting('reforge.inventory_scheduler',true)='true') WITH CHECK (current_setting('reforge.inventory_scheduler',true)='true');
