-- Preserve access to the migrated role-management UI on older Custom installs.
-- Only the existing oldest active administrator is promoted; no new user is created.
UPDATE users SET role = 'super_admin', updated_at = NOW()
WHERE id = (
  SELECT id FROM users
  WHERE role = 'admin' AND status = 'active' AND deleted_at IS NULL
  ORDER BY id LIMIT 1
) AND NOT EXISTS (
  SELECT 1 FROM users WHERE role = 'super_admin' AND deleted_at IS NULL
);
