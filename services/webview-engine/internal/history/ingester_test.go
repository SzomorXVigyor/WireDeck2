package history

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockStore struct {
	mu      sync.Mutex
	batches [][]SampleRecord
	all     []SampleRecord
}

func (m *mockStore) WriteBatch(_ context.Context, rows []SampleRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]SampleRecord(nil), rows...)
	m.batches = append(m.batches, cp)
	m.all = append(m.all, cp...)
	return nil
}

func (m *mockStore) Query(_ context.Context, _ uuid.UUID, _ []uuid.UUID, _, _ time.Time) (*QueryResult, error) {
	return &QueryResult{Resolution: "raw", Series: map[uuid.UUID][]HistoryPoint{}}, nil
}

func (m *mockStore) DeleteSiteHistory(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockStore) PruneBefore(_ context.Context, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (m *mockStore) Ping(_ context.Context) error { return nil }
func (m *mockStore) Close()                       {}

func TestChooseResolution(t *testing.T) {
	cases := []struct {
		span   time.Duration
		want   string
		view   string
		bucket string
	}{
		{time.Hour, "raw", "platform_webview_history.data_value_histories", ""},
		{6 * time.Hour, "raw", "platform_webview_history.data_value_histories", ""},
		{12 * time.Hour, "1m", "platform_webview_history.data_value_1m", "1 minute"},
		{7 * 24 * time.Hour, "5m", "platform_webview_history.data_value_1m", "5 minutes"},
		{30 * 24 * time.Hour, "1h", "platform_webview_history.data_value_1h", "1 hour"},
		{180 * 24 * time.Hour, "6h", "platform_webview_history.data_value_1h", "6 hours"},
	}
	for _, tc := range cases {
		res, view, bucket := ChooseResolution(tc.span)
		if res != tc.want || view != tc.view || bucket != tc.bucket {
			t.Fatalf("ChooseResolution(%v) = (%q, %q, %q), want (%q, %q, %q)", tc.span, res, view, bucket, tc.want, tc.view, tc.bucket)
		}
	}
}

func TestIngester_ThrottlesByPollIntervalAndFlushesOnShutdown(t *testing.T) {
	ms := &mockStore{}
	ing := NewIngester(ms, time.Hour, 100)

	siteID := uuid.New()
	mapID := uuid.New()
	t0 := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	// Simulate 1s dashboard polls on a register whose history entry is configured for 60s
	for i := 0; i < 65; i++ {
		ing.Record(siteID, mapID, t0.Add(time.Duration(i)*time.Second), float64(i), 60)
	}

	// Synchronous SIGTERM-style shutdown must flush immediately even though timer is 1h
	ing.Shutdown(context.Background())

	ms.mu.Lock()
	defer ms.mu.Unlock()
	if len(ms.all) != 2 {
		t.Fatalf("expected exactly 2 archived samples (at t=0s and t=60s), got %d: %+v", len(ms.all), ms.all)
	}
	if ms.all[0].Value != 0 || ms.all[1].Value != 60 {
		t.Fatalf("unexpected archived values: %+v", ms.all)
	}
}
