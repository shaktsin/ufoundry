package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/shaktsin/umcode/internal/protocol"
)

// Health is the router's memory of one key and model.
type Health struct {
	Provider     string
	Model        string
	CredentialID string
	CooldownEnd  time.Time
	LastStatus   string
	LastError    string
	OKCount      int64
	ErrCount     int64
	LatencyMs    int64
	UpdatedAt    time.Time
}

// CoolingDown reports whether this route should be skipped right now.
func (h Health) CoolingDown(now time.Time) bool {
	return !h.CooldownEnd.IsZero() && h.CooldownEnd.After(now)
}

const healthCols = `provider, model, credential_id, cooldown_until, last_status, last_error,
	ok_count, err_count, latency_ms, updated_at`

func scanHealth(sc interface{ Scan(...any) error }) (Health, error) {
	var h Health
	var cooldown, updated string
	err := sc.Scan(&h.Provider, &h.Model, &h.CredentialID, &cooldown, &h.LastStatus, &h.LastError,
		&h.OKCount, &h.ErrCount, &h.LatencyMs, &updated)
	if err != nil {
		return h, err
	}
	if cooldown != "" {
		h.CooldownEnd = ParseTime(cooldown)
	}
	h.UpdatedAt = ParseTime(updated)
	return h, nil
}

// GetHealth returns what is known about one route; a missing row is a healthy one.
func (s *Store) GetHealth(ctx context.Context, provider, model, credID string) (Health, error) {
	h, err := scanHealth(s.DB.QueryRowContext(ctx, `SELECT `+healthCols+` FROM model_health
		WHERE provider = ? AND model = ? AND credential_id = ?`, provider, model, credID))
	if errors.Is(err, sql.ErrNoRows) {
		return Health{Provider: provider, Model: model, CredentialID: credID}, nil
	}
	return h, err
}

// ListHealth returns every row the router has written, newest first.
func (s *Store) ListHealth(ctx context.Context) ([]Health, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+healthCols+` FROM model_health ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Health
	for rows.Next() {
		h, err := scanHealth(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// RecordSuccess clears any cooldown and counts a good request.
func (s *Store) RecordSuccess(ctx context.Context, provider, model, credID string, latencyMs int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO model_health
		(`+healthCols+`) VALUES (?,?,?,'','','',1,0,?,?)
		ON CONFLICT(provider, model, credential_id) DO UPDATE SET
			cooldown_until = '', last_status = '', last_error = '',
			ok_count = ok_count + 1, latency_ms = excluded.latency_ms, updated_at = excluded.updated_at`,
		provider, model, credID, latencyMs, Now())
	return err
}

// RecordFailure counts a failed request and, when cooldown is non-zero, keeps
// the router off this route until it passes.
func (s *Store) RecordFailure(ctx context.Context, provider, model, credID, status, msg string, cooldown time.Duration) error {
	until := ""
	if cooldown > 0 {
		until = FormatTime(time.Now().UTC().Add(cooldown))
	}
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO model_health
		(`+healthCols+`) VALUES (?,?,?,?,?,?,0,1,0,?)
		ON CONFLICT(provider, model, credential_id) DO UPDATE SET
			cooldown_until = excluded.cooldown_until, last_status = excluded.last_status,
			last_error = excluded.last_error, err_count = err_count + 1, updated_at = excluded.updated_at`,
		provider, model, credID, until, status, msg, Now())
	return err
}

// ClearCooldown lets a route be tried again immediately.
func (s *Store) ClearCooldown(ctx context.Context, provider, model, credID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE model_health SET cooldown_until = '', updated_at = ?
		WHERE provider = ? AND model = ? AND credential_id = ?`, Now(), provider, model, credID)
	return err
}

// SetTurnRoutes stores the routes a turn tried, in order.
func (s *Store) SetTurnRoutes(ctx context.Context, turnID string, trail []protocol.RouteStep) error {
	data, err := json.Marshal(trail)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE turns SET route_trail = ? WHERE id = ?`, string(data), turnID)
	return err
}

// TurnRoutes reads back what SetTurnRoutes stored.
func (s *Store) TurnRoutes(ctx context.Context, turnID string) ([]protocol.RouteStep, error) {
	var data string
	err := s.DB.QueryRowContext(ctx, `SELECT route_trail FROM turns WHERE id = ?`, turnID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil || data == "" {
		return nil, err
	}
	var trail []protocol.RouteStep
	return trail, json.Unmarshal([]byte(data), &trail)
}
