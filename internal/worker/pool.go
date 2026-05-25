// Package worker pulls waiting records from the database and dispatches them
// through the upstream client + blob store. Concurrency is hot-tunable from
// the admin API.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/liusx/shadraw/internal/blob"
	"github.com/liusx/shadraw/internal/record"
	"github.com/liusx/shadraw/internal/upstream"
)

// UpstreamConfigSource yields the live upstream credentials. It must be
// safe for concurrent reads.
type UpstreamConfigSource interface {
	Snapshot() upstream.Config
}

// Pool drains the records.waiting queue.
type Pool struct {
	records  *record.Repository
	blob     blob.Store
	upstream *upstream.Client
	source   UpstreamConfigSource

	mu          sync.Mutex
	desired     int
	current     int
	idleBackoff time.Duration

	wg     sync.WaitGroup
	stopCh chan struct{}
	wake   chan struct{}
	closed bool
}

// New constructs a pool but does not start workers.
func New(recordRepo *record.Repository, blobStore blob.Store, upstreamCli *upstream.Client, source UpstreamConfigSource) *Pool {
	return &Pool{
		records:     recordRepo,
		blob:        blobStore,
		upstream:    upstreamCli,
		source:      source,
		stopCh:      make(chan struct{}),
		wake:        make(chan struct{}, 1),
		idleBackoff: 2 * time.Second,
	}
}

// Start brings the pool up to `concurrency` workers.
func (p *Pool) Start(concurrency int) {
	p.Resize(concurrency)
}

// Resize tunes the live worker count. New workers spin up immediately; surplus
// workers exit on their next iteration when they observe p.current > p.desired.
func (p *Pool) Resize(target int) {
	if target < 0 {
		target = 0
	}
	if target > 16 {
		target = 16
	}
	p.mu.Lock()
	p.desired = target
	delta := target - p.current
	if delta > 0 {
		for i := 0; i < delta; i++ {
			p.current++
			p.wg.Add(1)
			go p.run()
		}
	}
	p.mu.Unlock()
	slog.Info("worker pool resized", "target", target)
}

// Wake hints that new work may be available (e.g. right after Create).
func (p *Pool) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Stop signals all workers to exit and waits for them.
func (p *Pool) Stop(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.stopCh)
	p.mu.Unlock()

	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run is the per-goroutine drain loop.
func (p *Pool) run() {
	defer p.wg.Done()
	defer func() {
		p.mu.Lock()
		p.current--
		p.mu.Unlock()
	}()

	for {
		p.mu.Lock()
		excess := p.current > p.desired
		p.mu.Unlock()
		if excess {
			return
		}

		select {
		case <-p.stopCh:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		processed, err := p.tickOnce(ctx)
		cancel()
		if err != nil {
			slog.Warn("worker tick error", "err", err)
		}
		if processed {
			continue
		}
		// idle: wait for wake / stop / backoff
		select {
		case <-p.stopCh:
			return
		case <-p.wake:
		case <-time.After(p.idleBackoff):
		}
	}
}

// tickOnce claims at most one waiting record and processes it.
// Returns (true, nil) when work happened; (false, nil) when idle.
func (p *Pool) tickOnce(ctx context.Context) (bool, error) {
	rec, err := p.records.ClaimWaiting(ctx)
	if err != nil {
		if errors.Is(err, record.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("claim: %w", err)
	}

	cfg := p.source.Snapshot()
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		_ = p.records.MarkFailed(ctx, rec.ID, "管理员未配置上游接口")
		return true, nil
	}

	refs := make([]upstream.ReferenceImage, 0, len(rec.ReferenceImages))
	for _, dataURL := range rec.ReferenceImages {
		refs = append(refs, upstream.ReferenceImage{DataURL: dataURL})
	}

	result, gerr := p.upstream.Generate(ctx, cfg, upstream.GenerateParams{
		Model:           rec.Model,
		Prompt:          rec.Prompt,
		Ratio:           rec.Ratio,
		Pixels:          rec.Pixels,
		ReferenceImages: refs,
	})
	if gerr != nil {
		msg := "生成失败"
		var ue *upstream.Error
		if errors.As(gerr, &ue) {
			msg = fmt.Sprintf("%s: %s", ue.Kind, truncate(ue.Message, 200))
		} else {
			msg = truncate(gerr.Error(), 200)
		}
		_ = p.records.MarkFailed(ctx, rec.ID, msg)
		return true, nil
	}

	userKey := fmt.Sprintf("user-%d", rec.UserID)
	fileKey := rec.UUID + ".png"
	path, perr := p.blob.Put(ctx, "images", userKey, fileKey, result.PNG)
	if perr != nil {
		_ = p.records.MarkFailed(ctx, rec.ID, "存储失败: "+perr.Error())
		return true, nil
	}

	if merr := p.records.MarkCompleted(ctx, rec.ID, path); merr != nil {
		slog.Warn("mark completed failed", "id", rec.ID, "err", merr)
	}
	return true, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
