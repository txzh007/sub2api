package service

import (
	"context"
	"fmt"
	"time"
)

// TTokenAccountActivity is intentionally separate from upstream dashboard contracts.
// Rates describe recorded final outcomes, not every upstream attempt.
type TTokenAccountActivity struct {
	AccountID       int64                  `json:"account_id"`
	Success         int64                  `json:"success"`
	Failed          int64                  `json:"failed"`
	Recovered       int64                  `json:"recovered"`
	RateLimited     int64                  `json:"rate_limited"`
	Cost            float64                `json:"cost"`
	AvgDurationMS   *float64               `json:"avg_duration_ms"`
	AvgFirstTokenMS *float64               `json:"avg_first_token_ms"`
	LastUsedAt      *time.Time             `json:"last_used_at"`
	Trend           []TTokenActivityBucket `json:"trend"`
}

type TTokenActivityBucket struct {
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
	Recovered int64 `json:"recovered"`
}

type TTokenActivityRequest struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	CreatedAt    time.Time `json:"created_at"`
	Model        string    `json:"model"`
	StatusCode   *int      `json:"status_code"`
	DurationMS   *float64  `json:"duration_ms"`
	FirstTokenMS *float64  `json:"first_token_ms"`
	Cost         float64   `json:"cost"`
	Message      string    `json:"message"`
}

type TTokenAccountActivityResponse struct {
	StartTime         time.Time                        `json:"start_time"`
	EndTime           time.Time                        `json:"end_time"`
	MonitoringEnabled bool                             `json:"monitoring_enabled"`
	Accounts          map[int64]*TTokenAccountActivity `json:"accounts"`
	Recent            []TTokenActivityRequest          `json:"recent"`
}

type ttokenActivityReader interface {
	ReadTTokenAccountActivity(context.Context, []int64, time.Time, time.Time, bool) (*TTokenAccountActivityResponse, error)
}

func (s *OpsService) GetTTokenAccountActivity(ctx context.Context, ids []int64, hours int, recent bool) (*TTokenAccountActivityResponse, error) {
	reader, ok := s.opsRepo.(ttokenActivityReader)
	if !ok {
		return nil, fmt.Errorf("account activity is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	end := time.Now().UTC()
	result, err := reader.ReadTTokenAccountActivity(ctx, ids, end.Add(-time.Duration(hours)*time.Hour), end, recent)
	if err != nil {
		return nil, err
	}
	result.MonitoringEnabled = s.IsMonitoringEnabled(ctx)
	return result, nil
}
