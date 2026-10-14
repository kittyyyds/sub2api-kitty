-- Bounded, creation-date statistics and customer task lists. These indexes do
-- not rewrite billing snapshots or change any settlement state.
CREATE INDEX IF NOT EXISTS video_tasks_created_id_idx ON video_tasks(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS video_tasks_user_created_id_idx ON video_tasks(user_id, created_at DESC, id DESC);
