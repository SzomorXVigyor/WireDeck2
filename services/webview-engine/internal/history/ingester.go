package history

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Ingester batches archived register samples and flushes them via pgx.CopyFrom
// every flushInterval or flushRows rows, with a guaranteed synchronous flush on Shutdown.
type Ingester struct {
	store         Store
	flushInterval time.Duration
	flushRows     int

	mu           sync.Mutex
	buf          []SampleRecord
	lastArchived map[uuid.UUID]time.Time
	closed       bool

	flushTrigger chan struct{}
	stopCh       chan struct{}
	doneCh       chan struct{}
}

func NewIngester(store Store, flushInterval time.Duration, flushRows int) *Ingester {
	if flushInterval <= 0 {
		flushInterval = time.Second
	}
	if flushRows <= 0 {
		flushRows = 500
	}
	ing := &Ingester{
		store:         store,
		flushInterval: flushInterval,
		flushRows:     flushRows,
		buf:           make([]SampleRecord, 0, flushRows),
		lastArchived:  make(map[uuid.UUID]time.Time),
		flushTrigger:  make(chan struct{}, 1),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	go ing.loop()
	return ing
}

// Record enqueues a good telemetry sample if its configured archival pollIntervalSeconds has elapsed.
// This prevents a 1s interactive dashboard from writing 60x extra rows into a 60s history entry.
func (i *Ingester) Record(siteID, mappingID uuid.UUID, ts time.Time, value float64, pollIntervalSeconds int) {
	if i == nil || i.store == nil {
		return
	}
	if pollIntervalSeconds < 1 {
		pollIntervalSeconds = 1
	}
	minGap := time.Duration(pollIntervalSeconds)*time.Second - 200*time.Millisecond
	if minGap < 0 {
		minGap = 0
	}

	var shouldTrigger bool
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return
	}
	if prev, ok := i.lastArchived[mappingID]; ok && ts.Sub(prev) < minGap {
		i.mu.Unlock()
		return
	}
	i.lastArchived[mappingID] = ts
	i.buf = append(i.buf, SampleRecord{
		Time:              ts.UTC(),
		SiteID:            siteID,
		RegisterMappingID: mappingID,
		Value:             value,
	})
	if len(i.buf) >= i.flushRows {
		shouldTrigger = true
	}
	i.mu.Unlock()

	if shouldTrigger {
		select {
		case i.flushTrigger <- struct{}{}:
		default:
		}
	}
}

func (i *Ingester) loop() {
	defer close(i.doneCh)
	ticker := time.NewTicker(i.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-i.stopCh:
			return
		case <-i.flushTrigger:
			i.flushOnce(context.Background())
		case <-ticker.C:
			i.flushOnce(context.Background())
		}
	}
}

func (i *Ingester) drainBuffer() []SampleRecord {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.buf) == 0 {
		return nil
	}
	batch := i.buf
	i.buf = make([]SampleRecord, 0, i.flushRows)
	return batch
}

func (i *Ingester) flushOnce(parent context.Context) {
	batch := i.drainBuffer()
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	if err := i.store.WriteBatch(ctx, batch); err != nil {
		slog.Warn("history_batch_flush_failed", "rows", len(batch), "error", err)
		// Re-queue up to 4x flushRows to survive brief TSDB hiccups without unbounded RAM growth
		i.mu.Lock()
		maxCap := i.flushRows * 4
		if len(i.buf)+len(batch) <= maxCap {
			i.buf = append(batch, i.buf...)
		} else if len(i.buf) < maxCap {
			keep := maxCap - len(i.buf)
			i.buf = append(batch[len(batch)-keep:], i.buf...)
		}
		i.mu.Unlock()
	}
}

// Shutdown stops accepting new samples, stops the background loop, and synchronously flushes buffered rows.
func (i *Ingester) Shutdown(ctx context.Context) {
	if i == nil {
		return
	}
	i.mu.Lock()
	if i.closed {
		i.mu.Unlock()
		return
	}
	i.closed = true
	i.mu.Unlock()

	close(i.stopCh)
	<-i.doneCh
	i.flushOnce(ctx)
}
