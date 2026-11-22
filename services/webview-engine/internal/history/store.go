package history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrHistoryUnavailable = errors.New("history time-series database is not configured")

// SampleRecord is a single archived register telemetry sample destined for TimescaleDB.
type SampleRecord struct {
	Time              time.Time
	SiteID            uuid.UUID
	RegisterMappingID uuid.UUID
	Value             float64
}

// HistoryPoint is one timestamped point in a queried history series.
// For raw resolution, Min/Max are nil and Count is 1.
// For continuous aggregate resolutions (1m/5m/1h/6h), Value is the LOCF avg, with Min/Max/Count from the aggregate.
type HistoryPoint struct {
	Time  time.Time `json:"time"`
	Value *float64  `json:"value"`
	Min   *float64  `json:"min,omitempty"`
	Max   *float64  `json:"max,omitempty"`
	Count int64     `json:"count"`
}

// QueryResult holds the selected resolution and per-mapping time-series points.
type QueryResult struct {
	Resolution string
	Series     map[uuid.UUID][]HistoryPoint
}

// Store abstracts TimescaleDB persistence so engine tests can mock it cleanly.
type Store interface {
	WriteBatch(ctx context.Context, rows []SampleRecord) error
	Query(ctx context.Context, siteID uuid.UUID, mappingIDs []uuid.UUID, from, to time.Time) (*QueryResult, error)
	DeleteSiteHistory(ctx context.Context, siteID uuid.UUID) error
	PruneBefore(ctx context.Context, mappingID uuid.UUID, cutoff time.Time) error
	Ping(ctx context.Context) error
	Close()
}

// ChooseResolution picks the optimal hypertable or continuous aggregate based on time range:
//   - <= 6h  : raw data_value_histories
//   - <= 24h : data_value_1m (1-minute gapfill + LOCF)
//   - <= 7d  : data_value_1m (5-minute gapfill + LOCF)
//   - <= 90d : data_value_1h (1-hour gapfill + LOCF)
//   - > 90d  : data_value_1h (6-hour gapfill + LOCF)
func ChooseResolution(span time.Duration) (resolution string, tableName string, bucketInterval string) {
	switch {
	case span <= 6*time.Hour:
		return "raw", "platform_webview_history.data_value_histories", ""
	case span <= 24*time.Hour:
		return "1m", "platform_webview_history.data_value_1m", "1 minute"
	case span <= 7*24*time.Hour:
		return "5m", "platform_webview_history.data_value_1m", "5 minutes"
	case span <= 90*24*time.Hour:
		return "1h", "platform_webview_history.data_value_1h", "1 hour"
	default:
		return "6h", "platform_webview_history.data_value_1h", "6 hours"
	}
}

// TimescaleStore persists and queries register history in TimescaleDB (platform_webview_history).
type TimescaleStore struct {
	pool *pgxpool.Pool
}

func NewTimescaleStore(ctx context.Context, dsn string) (*TimescaleStore, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse TSDB_DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 12
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create TSDB pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping TSDB: %w", err)
	}
	return &TimescaleStore{pool: pool}, nil
}

func (s *TimescaleStore) Ping(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return ErrHistoryUnavailable
	}
	return s.pool.Ping(ctx)
}

func (s *TimescaleStore) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *TimescaleStore) WriteBatch(ctx context.Context, rows []SampleRecord) error {
	if len(rows) == 0 {
		return nil
	}
	_, err := s.pool.CopyFrom(
		ctx,
		pgx.Identifier{"platform_webview_history", "data_value_histories"},
		[]string{"time", "register_mapping_id", "site_id", "value"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.Time.UTC(), r.RegisterMappingID, r.SiteID, r.Value}, nil
		}),
	)
	if err != nil {
		return fmt.Errorf("copy history batch (%d rows): %w", len(rows), err)
	}
	return nil
}

func (s *TimescaleStore) Query(ctx context.Context, siteID uuid.UUID, mappingIDs []uuid.UUID, from, to time.Time) (*QueryResult, error) {
	from = from.UTC()
	to = to.UTC()
	if !to.After(from) {
		to = from.Add(time.Minute)
	}
	resName, viewName, bucketInterval := ChooseResolution(to.Sub(from))
	series := make(map[uuid.UUID][]HistoryPoint, len(mappingIDs))
	for _, id := range mappingIDs {
		series[id] = []HistoryPoint{}
	}
	if len(mappingIDs) == 0 {
		return &QueryResult{Resolution: resName, Series: series}, nil
	}

	if resName == "raw" {
		rows, err := s.pool.Query(ctx, `
			SELECT register_mapping_id, time, value
			FROM platform_webview_history.data_value_histories
			WHERE site_id = $1
			  AND register_mapping_id = ANY($2)
			  AND time >= $3
			  AND time <= $4
			ORDER BY register_mapping_id, time ASC
		`, siteID, mappingIDs, from, to)
		if err != nil {
			return nil, fmt.Errorf("query raw history: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var mid uuid.UUID
			var ts time.Time
			var val float64
			if err := rows.Scan(&mid, &ts, &val); err != nil {
				return nil, fmt.Errorf("scan raw history: %w", err)
			}
			v := val
			series[mid] = append(series[mid], HistoryPoint{
				Time:  ts.UTC(),
				Value: &v,
				Count: 1,
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return &QueryResult{Resolution: resName, Series: series}, nil
	}

	// Continuous aggregate with time_bucket_gapfill + LOCF
	query := fmt.Sprintf(`
		SELECT
			register_mapping_id,
			time_bucket_gapfill($5::interval, bucket, $3::timestamptz, $4::timestamptz) AS b,
			locf(avg(avg_value)) AS val,
			min(min_value) AS min_val,
			max(max_value) AS max_val,
			COALESCE(sum(sample_count), 0)::bigint AS cnt
		FROM %s
		WHERE site_id = $1
		  AND register_mapping_id = ANY($2)
		  AND bucket >= $3::timestamptz
		  AND bucket <= $4::timestamptz
		GROUP BY register_mapping_id, b
		ORDER BY register_mapping_id, b ASC
	`, viewName)

	rows, err := s.pool.Query(ctx, query, siteID, mappingIDs, from, to, bucketInterval)
	if err != nil {
		return nil, fmt.Errorf("query %s history: %w", resName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var mid uuid.UUID
		var ts time.Time
		var val, minVal, maxVal *float64
		var cnt int64
		if err := rows.Scan(&mid, &ts, &val, &minVal, &maxVal, &cnt); err != nil {
			return nil, fmt.Errorf("scan aggregate history: %w", err)
		}
		// Skip leading empty gapfill buckets before the first real observation
		if val == nil && cnt == 0 && len(series[mid]) == 0 {
			continue
		}
		series[mid] = append(series[mid], HistoryPoint{
			Time:  ts.UTC(),
			Value: val,
			Min:   minVal,
			Max:   maxVal,
			Count: cnt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &QueryResult{Resolution: resName, Series: series}, nil
}

func (s *TimescaleStore) DeleteSiteHistory(ctx context.Context, siteID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM platform_webview_history.data_value_histories WHERE site_id = $1`, siteID)
	if err != nil {
		return fmt.Errorf("delete site history: %w", err)
	}
	return nil
}

func (s *TimescaleStore) PruneBefore(ctx context.Context, mappingID uuid.UUID, cutoff time.Time) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM platform_webview_history.data_value_histories
		WHERE register_mapping_id = $1 AND time < $2
	`, mappingID, cutoff.UTC())
	return err
}
