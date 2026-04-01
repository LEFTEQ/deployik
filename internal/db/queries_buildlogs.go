package db

import "fmt"

func (db *DB) InsertBuildLog(deploymentID string, lineNumber int, content, stream string) error {
	_, err := db.Exec(
		`INSERT INTO build_logs (deployment_id, line_number, content, stream)
		 VALUES (?, ?, ?, ?)`,
		deploymentID, lineNumber, content, stream,
	)
	if err != nil {
		return fmt.Errorf("insert build log: %w", err)
	}
	return nil
}

func (db *DB) GetBuildLogs(deploymentID string) ([]BuildLog, error) {
	rows, err := db.Query(
		`SELECT id, deployment_id, line_number, content, stream, timestamp
		 FROM build_logs WHERE deployment_id = ?
		 ORDER BY line_number ASC`, deploymentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get build logs: %w", err)
	}
	defer rows.Close()

	var logs []BuildLog
	for rows.Next() {
		var l BuildLog
		if err := rows.Scan(&l.ID, &l.DeploymentID, &l.LineNumber, &l.Content, &l.Stream, &l.Timestamp); err != nil {
			return nil, fmt.Errorf("scan build log: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// PruneBuildLogs deletes logs for old deployments, keeping the last N per project.
func (db *DB) PruneBuildLogs(projectID string, keepCount int) error {
	_, err := db.Exec(`
		DELETE FROM build_logs WHERE deployment_id IN (
			SELECT d.id FROM deployments d
			WHERE d.project_id = ?
			AND d.id NOT IN (
				SELECT id FROM deployments
				WHERE project_id = ?
				ORDER BY created_at DESC
				LIMIT ?
			)
		)`, projectID, projectID, keepCount,
	)
	if err != nil {
		return fmt.Errorf("prune build logs: %w", err)
	}
	return nil
}
