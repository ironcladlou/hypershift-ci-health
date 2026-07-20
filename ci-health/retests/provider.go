package retests

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Provider struct {
	mu   sync.RWMutex
	data *AnalysisResult
}

func NewProvider(ctx context.Context, cfg Config, interval time.Duration) *Provider {
	p := &Provider{}

	refresh := func() {
		fmt.Fprintf(logWriter, "retests: collecting data...\n")
		result, err := Run(ctx, cfg)
		if err != nil {
			fmt.Fprintf(logWriter, "retests: collection error: %v\n", err)
			return
		}
		p.mu.Lock()
		p.data = result
		p.mu.Unlock()
		fmt.Fprintf(logWriter, "retests: collection complete: %d PRs analyzed\n", result.PRsAnalyzed)
	}

	go refresh()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	return p
}

func (p *Provider) Data() *AnalysisResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.data
}
