ALTER TABLE projects ADD COLUMN root_directory TEXT NOT NULL DEFAULT '';

ALTER TABLE projects ADD COLUMN output_directory TEXT NOT NULL DEFAULT '';

UPDATE projects
SET output_directory = CASE
    WHEN framework = 'nextjs' THEN '.next'
    ELSE 'dist'
END
WHERE output_directory = '';
