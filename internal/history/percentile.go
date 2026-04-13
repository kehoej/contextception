package history

import "fmt"

// CurrentScoreVersion is the current risk score formula version.
// Bump this when the scoring formula changes to invalidate old percentiles.
const CurrentScoreVersion = 1

// minRecordsForPercentile is the minimum number of records needed to compute percentiles.
const minRecordsForPercentile = 10

// StoreRiskScore records a per-file risk score in the history database.
func (s *Store) StoreRiskScore(filePath string, riskScore int, refRange string) error {
	_, err := s.db.Exec(
		`INSERT INTO risk_scores (file_path, risk_score, score_version, ref_range) VALUES (?, ?, ?, ?)`,
		filePath, riskScore, CurrentScoreVersion, refRange,
	)
	if err != nil {
		return fmt.Errorf("storing risk score: %w", err)
	}
	return nil
}

// StoreRiskScores records multiple per-file risk scores in a single transaction.
func (s *Store) StoreRiskScores(scores []RiskScoreEntry) error {
	if len(scores) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO risk_scores (file_path, risk_score, score_version, ref_range) VALUES (?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	for _, entry := range scores {
		if _, err := stmt.Exec(entry.FilePath, entry.RiskScore, CurrentScoreVersion, entry.RefRange); err != nil {
			return fmt.Errorf("inserting risk score for %s: %w", entry.FilePath, err)
		}
	}

	return tx.Commit()
}

// RiskScoreEntry holds data for a single risk score record.
type RiskScoreEntry struct {
	FilePath  string
	RiskScore int
	RefRange  string
}

// ComputePercentile returns the percentile ranking for a given score.
// Returns 0 if fewer than minRecordsForPercentile records exist with the current score version.
func (s *Store) ComputePercentile(score int) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM risk_scores WHERE score_version = ?`,
		CurrentScoreVersion,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting risk scores: %w", err)
	}
	if count < minRecordsForPercentile {
		return 0, nil
	}

	var belowCount int
	err = s.db.QueryRow(
		`SELECT COUNT(*) FROM risk_scores WHERE score_version = ? AND risk_score < ?`,
		CurrentScoreVersion, score,
	).Scan(&belowCount)
	if err != nil {
		return 0, fmt.Errorf("computing percentile: %w", err)
	}

	percentile := (belowCount * 100) / count
	return percentile, nil
}
