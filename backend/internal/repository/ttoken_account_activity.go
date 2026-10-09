package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

// Use existing logs only. Recovered provider errors remain separate from final
// failures. Attribution follows ops_error_logs.account_id, as in upstream Ops.
const ttokenActivityCTE = `WITH errors AS MATERIALIZED (
 SELECT id,account_id,created_at,request_id,client_request_id,model,status_code,upstream_status_code,
   duration_ms,time_to_first_token_ms,error_message,error_type,error_phase,error_owner FROM ops_error_logs
 WHERE account_id = ANY($1) AND created_at >= $2 AND created_at < $3 AND NOT is_count_tokens
), failures AS MATERIALIZED (
 SELECT DISTINCT ON (account_id, COALESCE(NULLIF(request_id,''), NULLIF(client_request_id,''), 'row:' || id::text)) *
 FROM errors WHERE COALESCE(status_code,0) >= 400 OR error_type = 'cyber_policy'
 ORDER BY account_id, COALESCE(NULLIF(request_id,''), NULLIF(client_request_id,''), 'row:' || id::text), created_at DESC, id DESC
), events AS (
 SELECT u.account_id, 'usage:' || u.id::text AS id,
   CASE WHEN EXISTS (SELECT 1 FROM failures f WHERE f.account_id = u.account_id
     AND NULLIF(u.request_id,'') IS NOT NULL AND (f.request_id = u.request_id OR f.client_request_id = u.request_id))
     THEN 'billed_failure' ELSE 'success' END AS kind,
   u.created_at, u.model, NULL::integer AS status_code, u.duration_ms::float8 AS duration_ms,
   u.first_token_ms::float8 AS first_token_ms,
   (COALESCE(u.account_stats_cost, u.total_cost,0) * COALESCE(u.account_rate_multiplier,1))::float8 AS cost,
   ''::text AS message
 FROM usage_logs u WHERE u.account_id = ANY($1) AND u.created_at >= $2 AND u.created_at < $3
 UNION ALL
 SELECT account_id, 'error:' || id::text, 'failed', created_at, COALESCE(model,''),
   COALESCE(upstream_status_code, status_code), duration_ms::float8, time_to_first_token_ms::float8, 0::float8,
   LEFT(COALESCE(error_message,''),500) FROM failures
 UNION ALL
 SELECT account_id, 'recovered:' || id::text, 'recovered', created_at, COALESCE(model,''),
   upstream_status_code, NULL::float8, NULL::float8, 0::float8, LEFT(COALESCE(error_message,''),500)
 FROM errors WHERE COALESCE(status_code,0) < 400 AND error_type <> 'cyber_policy'
   AND error_phase IN ('upstream','account_auth') AND error_owner = 'provider'
) `

func (r *opsRepository) ReadTTokenAccountActivity(ctx context.Context, ids []int64, start, end time.Time, recent bool) (*service.TTokenAccountActivityResponse, error) {
	out := &service.TTokenAccountActivityResponse{StartTime: start, EndTime: end,
		Accounts: make(map[int64]*service.TTokenAccountActivity), Recent: []service.TTokenActivityRequest{}}
	for _, id := range ids {
		out.Accounts[id] = &service.TTokenAccountActivity{AccountID: id, Trend: make([]service.TTokenActivityBucket, 12)}
	}
	args := []any{pq.Array(ids), start, end}
	rows, err := r.db.QueryContext(ctx, ttokenActivityCTE+`
 SELECT account_id, LEAST(11, GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (created_at-$2::timestamptz)) / (EXTRACT(EPOCH FROM ($3::timestamptz-$2::timestamptz))/12))))::int AS bucket,
 COUNT(*) FILTER (WHERE kind='success'), COUNT(*) FILTER (WHERE kind='failed'), COUNT(*) FILTER (WHERE kind='recovered'),
 COUNT(*) FILTER (WHERE status_code=429), SUM(cost),
 COALESCE(SUM(duration_ms) FILTER (WHERE kind='success'),0), COUNT(duration_ms) FILTER (WHERE kind='success'),
 COALESCE(SUM(first_token_ms) FILTER (WHERE kind='success'),0), COUNT(first_token_ms) FILTER (WHERE kind='success'), MAX(created_at)
 FROM events GROUP BY account_id, bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type latency struct {
		duration, firstToken   float64
		durationN, firstTokenN int64
	}
	latencies := make(map[int64]*latency)
	for rows.Next() {
		var id, success, failed, recovered, limited, durationN, firstTokenN int64
		var bucket int
		var cost, duration, firstToken float64
		var last time.Time
		if err = rows.Scan(&id, &bucket, &success, &failed, &recovered, &limited, &cost, &duration, &durationN, &firstToken, &firstTokenN, &last); err != nil {
			return nil, err
		}
		a := out.Accounts[id]
		a.Success += success
		a.Failed += failed
		a.Recovered += recovered
		a.RateLimited += limited
		a.Cost += cost
		a.Trend[bucket] = service.TTokenActivityBucket{Success: success, Failed: failed, Recovered: recovered}
		if a.LastUsedAt == nil || last.After(*a.LastUsedAt) {
			v := last
			a.LastUsedAt = &v
		}
		l := latencies[id]
		if l == nil {
			l = &latency{}
			latencies[id] = l
		}
		l.duration += duration
		l.durationN += durationN
		l.firstToken += firstToken
		l.firstTokenN += firstTokenN
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for id, l := range latencies {
		if l.durationN > 0 {
			v := l.duration / float64(l.durationN)
			out.Accounts[id].AvgDurationMS = &v
		}
		if l.firstTokenN > 0 {
			v := l.firstToken / float64(l.firstTokenN)
			out.Accounts[id].AvgFirstTokenMS = &v
		}
	}
	if !recent {
		return out, nil
	}
	rows, err = r.db.QueryContext(ctx, ttokenActivityCTE+`
 SELECT id,kind,created_at,model,status_code,duration_ms,first_token_ms,cost,message
 FROM events WHERE kind <> 'billed_failure' ORDER BY created_at DESC,id DESC LIMIT 30`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v service.TTokenActivityRequest
		var status sql.NullInt64
		var duration, firstToken sql.NullFloat64
		if err = rows.Scan(&v.ID, &v.Kind, &v.CreatedAt, &v.Model, &status, &duration, &firstToken, &v.Cost, &v.Message); err != nil {
			return nil, err
		}
		if status.Valid {
			n := int(status.Int64)
			v.StatusCode = &n
		}
		if duration.Valid {
			v.DurationMS = &duration.Float64
		}
		if firstToken.Valid {
			v.FirstTokenMS = &firstToken.Float64
		}
		out.Recent = append(out.Recent, v)
	}
	return out, rows.Err()
}
