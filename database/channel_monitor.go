package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultChannelMonitorIntervalMinutes = 5
	MinChannelMonitorIntervalMinutes     = 1
	MaxChannelMonitorIntervalMinutes     = 60
	ChannelMonitorHistoryRetentionDays   = 30
)

const (
	ChannelMonitorStatusUnknown     = "unknown"
	ChannelMonitorStatusOperational = "operational"
	ChannelMonitorStatusDegraded    = "degraded"
	ChannelMonitorStatusFailed      = "failed"
)

// ChannelMonitorConfig is the one-to-one health monitor state for a Responses
// API account.
type ChannelMonitorConfig struct {
	AccountID       int64
	Enabled         bool
	IntervalMinutes int
	Model           string

	Status        string
	HTTPStatus    int
	LatencyMS     int64
	FirstTokenMS  int64
	Message       string
	LastCheckedAt sql.NullTime
	NextCheckAt   sql.NullTime

	CreatedAt time.Time
	UpdatedAt time.Time
}

type ChannelMonitorCheck struct {
	ID           int64
	AccountID    int64
	Status       string
	HTTPStatus   int
	Model        string
	LatencyMS    int64
	FirstTokenMS int64
	Message      string
	CheckedAt    time.Time
}

type ChannelMonitorAvailability struct {
	Total     int64
	Available int64
}

type ChannelMonitorHealthResult struct {
	AccountID    int64
	Status       string
	HTTPStatus   int
	Model        string
	LatencyMS    int64
	FirstTokenMS int64
	Message      string
	CheckedAt    time.Time
	NextCheckAt  time.Time
}

func NormalizeChannelMonitorInterval(minutes int) int {
	if minutes < MinChannelMonitorIntervalMinutes || minutes > MaxChannelMonitorIntervalMinutes {
		return DefaultChannelMonitorIntervalMinutes
	}
	return minutes
}

func (db *DB) ensureChannelMonitorSchema(ctx context.Context) error {
	if db == nil || db.conn == nil {
		return errors.New("database unavailable")
	}
	db.channelMonitorOnce.Do(func() {
		db.channelMonitorInitErr = db.createChannelMonitorSchema(ctx)
	})
	return db.channelMonitorInitErr
}

func (db *DB) createChannelMonitorSchema(ctx context.Context) error {
	configDDL := `CREATE TABLE IF NOT EXISTS channel_monitor_configs (
		account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
		enabled BOOLEAN NOT NULL DEFAULT FALSE,
		interval_minutes INTEGER NOT NULL DEFAULT 5,
		model VARCHAR(255) NOT NULL DEFAULT '',
		status VARCHAR(24) NOT NULL DEFAULT 'unknown',
		http_status INTEGER NOT NULL DEFAULT 0,
		latency_ms BIGINT NOT NULL DEFAULT 0,
		first_token_ms BIGINT NOT NULL DEFAULT 0,
		message TEXT NOT NULL DEFAULT '',
		last_checked_at TIMESTAMPTZ NULL,
		next_check_at TIMESTAMPTZ NULL,
		created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`
	historyDDL := `CREATE TABLE IF NOT EXISTS channel_monitor_checks (
		id BIGSERIAL PRIMARY KEY,
		account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		status VARCHAR(24) NOT NULL,
		http_status INTEGER NOT NULL DEFAULT 0,
		model VARCHAR(255) NOT NULL DEFAULT '',
		latency_ms BIGINT NOT NULL DEFAULT 0,
		first_token_ms BIGINT NOT NULL DEFAULT 0,
		message TEXT NOT NULL DEFAULT '',
		checked_at TIMESTAMPTZ NOT NULL
	)`
	if db.isSQLite() {
		historyDDL = strings.Replace(historyDDL, "id BIGSERIAL PRIMARY KEY", "id INTEGER PRIMARY KEY AUTOINCREMENT", 1)
		configDDL = strings.ReplaceAll(configDDL, "TIMESTAMPTZ", "TIMESTAMP")
		historyDDL = strings.ReplaceAll(historyDDL, "TIMESTAMPTZ", "TIMESTAMP")
	}
	for _, statement := range []string{
		configDDL,
		historyDDL,
		`CREATE INDEX IF NOT EXISTS idx_channel_monitor_configs_due ON channel_monitor_configs(enabled, next_check_at)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_monitor_checks_account_time ON channel_monitor_checks(account_id, checked_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_channel_monitor_checks_time ON channel_monitor_checks(checked_at)`,
	} {
		if _, err := db.conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize channel monitor schema: %w", err)
		}
	}
	return nil
}

func scanChannelMonitorConfig(scanner interface{ Scan(...interface{}) error }) (*ChannelMonitorConfig, error) {
	var config ChannelMonitorConfig
	err := scanner.Scan(
		&config.AccountID, &config.Enabled, &config.IntervalMinutes, &config.Model,
		&config.Status, &config.HTTPStatus, &config.LatencyMS, &config.FirstTokenMS,
		&config.Message, &config.LastCheckedAt, &config.NextCheckAt,
		&config.CreatedAt, &config.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	config.IntervalMinutes = NormalizeChannelMonitorInterval(config.IntervalMinutes)
	config.Model = strings.TrimSpace(config.Model)
	return &config, nil
}

const channelMonitorConfigColumns = `account_id, enabled, interval_minutes, model,
	status, http_status, latency_ms, first_token_ms, message, last_checked_at, next_check_at,
	created_at, updated_at`

func (db *DB) GetChannelMonitorConfig(ctx context.Context, accountID int64) (*ChannelMonitorConfig, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	return scanChannelMonitorConfig(db.conn.QueryRowContext(ctx,
		`SELECT `+channelMonitorConfigColumns+` FROM channel_monitor_configs WHERE account_id = $1`,
		accountID,
	))
}

// UpsertChannelMonitorConfig saves the user-facing settings and schedules an
// immediate health refresh when monitoring is enabled.
func (db *DB) UpsertChannelMonitorConfig(ctx context.Context, accountID int64, enabled bool, intervalMinutes int, model string, now time.Time) (*ChannelMonitorConfig, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	intervalMinutes = NormalizeChannelMonitorInterval(intervalMinutes)
	model = strings.TrimSpace(model)
	now = now.UTC()
	var next interface{}
	if enabled {
		next = db.timeArg(now)
	}
	_, err := db.conn.ExecContext(ctx, `INSERT INTO channel_monitor_configs (
		account_id, enabled, interval_minutes, model, next_check_at, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$6)
	ON CONFLICT (account_id) DO UPDATE SET
		enabled=EXCLUDED.enabled,
		interval_minutes=EXCLUDED.interval_minutes,
		model=EXCLUDED.model,
		next_check_at=EXCLUDED.next_check_at,
		updated_at=EXCLUDED.updated_at`,
		accountID, enabled, intervalMinutes, model, next, db.timeArg(now),
	)
	if err != nil {
		return nil, err
	}
	return db.GetChannelMonitorConfig(ctx, accountID)
}

func (db *DB) ListEnabledChannelMonitors(ctx context.Context) ([]ChannelMonitorConfig, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT `+channelMonitorConfigColumns+`
		FROM channel_monitor_configs
		WHERE enabled = TRUE AND EXISTS (
			SELECT 1 FROM accounts
			WHERE accounts.id = channel_monitor_configs.account_id
				AND accounts.status <> 'deleted'
				AND COALESCE(accounts.error_message, '') <> 'deleted'
		)
		ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := make([]ChannelMonitorConfig, 0)
	for rows.Next() {
		config, scanErr := scanChannelMonitorConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		configs = append(configs, *config)
	}
	return configs, rows.Err()
}

func (db *DB) ListDueChannelMonitors(ctx context.Context, now time.Time, limit int) ([]ChannelMonitorConfig, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT `+channelMonitorConfigColumns+`
		FROM channel_monitor_configs
		WHERE enabled = TRUE AND EXISTS (
			SELECT 1 FROM accounts
			WHERE accounts.id = channel_monitor_configs.account_id
				AND accounts.status <> 'deleted'
				AND COALESCE(accounts.error_message, '') <> 'deleted'
			) AND (next_check_at IS NULL OR next_check_at <= $1)
		ORDER BY CASE WHEN next_check_at IS NULL THEN 0 ELSE 1 END, next_check_at, account_id
		LIMIT $2`, db.timeArg(now.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := make([]ChannelMonitorConfig, 0, limit)
	for rows.Next() {
		config, scanErr := scanChannelMonitorConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		configs = append(configs, *config)
	}
	return configs, rows.Err()
}

func (db *DB) RecordChannelMonitorHealth(ctx context.Context, result ChannelMonitorHealthResult) error {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return err
	}
	return db.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO channel_monitor_checks (
			account_id, status, http_status, model, latency_ms, first_token_ms, message, checked_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			result.AccountID, result.Status, result.HTTPStatus, strings.TrimSpace(result.Model),
			result.LatencyMS, result.FirstTokenMS, truncateChannelMonitorMessage(result.Message),
			db.timeArg(result.CheckedAt.UTC()),
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE channel_monitor_configs SET
			status=$1, http_status=$2, latency_ms=$3, first_token_ms=$4, message=$5,
			last_checked_at=$6,
			next_check_at=CASE WHEN enabled=TRUE THEN `+db.channelMonitorTimestampPlaceholder(7)+` ELSE NULL END,
			updated_at=$6
			WHERE account_id=$8`,
			result.Status, result.HTTPStatus, result.LatencyMS, result.FirstTokenMS,
			truncateChannelMonitorMessage(result.Message), db.timeArg(result.CheckedAt.UTC()),
			db.timeArg(result.NextCheckAt.UTC()), result.AccountID,
		)
		return err
	})
}

// PostgreSQL cannot infer the type of a parameter whose only typed context is
// a CASE branch paired with NULL; it chooses text and then rejects assignment
// to TIMESTAMPTZ. SQLite uses numbered placeholders but does not understand
// PostgreSQL's cast syntax, so keep the cast dialect-specific.
func (db *DB) channelMonitorTimestampPlaceholder(index int) string {
	placeholder := fmt.Sprintf("$%d", index)
	if db != nil && !db.isSQLite() {
		return placeholder + "::TIMESTAMPTZ"
	}
	return placeholder
}

func (db *DB) GetChannelMonitorAvailability(ctx context.Context, since time.Time) (map[int64]ChannelMonitorAvailability, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT account_id, COUNT(*),
		SUM(CASE WHEN status IN ('operational','degraded') THEN 1 ELSE 0 END)
		FROM channel_monitor_checks WHERE checked_at >= $1 GROUP BY account_id`, db.timeArg(since.UTC()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]ChannelMonitorAvailability)
	for rows.Next() {
		var accountID int64
		var availability ChannelMonitorAvailability
		if err := rows.Scan(&accountID, &availability.Total, &availability.Available); err != nil {
			return nil, err
		}
		result[accountID] = availability
	}
	return result, rows.Err()
}

func (db *DB) ListChannelMonitorChecks(ctx context.Context, accountID int64, since time.Time, limit int) ([]ChannelMonitorCheck, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 1000 {
		limit = 168
	}
	rows, err := db.conn.QueryContext(ctx, `SELECT id, account_id, status, http_status, model,
		latency_ms, first_token_ms, message, checked_at
		FROM channel_monitor_checks
		WHERE account_id=$1 AND checked_at >= $2
		ORDER BY checked_at DESC LIMIT $3`, accountID, db.timeArg(since.UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	checks := make([]ChannelMonitorCheck, 0, limit)
	for rows.Next() {
		var check ChannelMonitorCheck
		if err := rows.Scan(&check.ID, &check.AccountID, &check.Status, &check.HTTPStatus,
			&check.Model, &check.LatencyMS, &check.FirstTokenMS, &check.Message, &check.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, rows.Err()
}

func (db *DB) DeleteChannelMonitorChecksBefore(ctx context.Context, before time.Time) (int64, error) {
	if err := db.ensureChannelMonitorSchema(ctx); err != nil {
		return 0, err
	}
	result, err := db.conn.ExecContext(ctx, `DELETE FROM channel_monitor_checks WHERE checked_at < $1`, db.timeArg(before.UTC()))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func truncateChannelMonitorMessage(message string) string {
	message = strings.TrimSpace(message)
	const maxBytes = 2000
	if len(message) <= maxBytes {
		return message
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut]
}
