ALTER TABLE projects ADD COLUMN package_manager TEXT NOT NULL DEFAULT 'auto'
CHECK (package_manager IN ('auto', 'bun', 'pnpm', 'npm', 'yarn'));
