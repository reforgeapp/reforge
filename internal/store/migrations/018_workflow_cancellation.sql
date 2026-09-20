ALTER TABLE workflow_tasks ADD COLUMN cancellation_requested boolean NOT NULL DEFAULT false;
UPDATE workflow_tasks SET cancellation_requested=true WHERE state IN ('cancelling','cancelled') OR (state='reconciling' AND reason='Cancellation requested; external outcome remains unknown');
