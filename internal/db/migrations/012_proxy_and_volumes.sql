ALTER TABLE projects ADD COLUMN data_volume_enabled BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE projects ADD COLUMN data_mount_path TEXT NOT NULL DEFAULT '/app/data';
