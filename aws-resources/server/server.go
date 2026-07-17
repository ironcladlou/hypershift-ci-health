package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/dmace/hypershift-ci-health/aws-resources/collector"
)

//go:embed dashboard.html
var dashboardHTML string

type DataProvider interface {
	Data() (*collector.APIResponse, error)
}

type LiveProvider struct {
	mu   sync.RWMutex
	data *collector.APIResponse
}

func NewLiveProvider(ctx context.Context, cfg collector.Config, interval time.Duration) *LiveProvider {
	p := &LiveProvider{}

	refresh := func() {
		fmt.Fprintf(os.Stderr, "Collecting data...\n")
		resp, err := collector.Collect(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Collection error: %v\n", err)
			return
		}
		p.mu.Lock()
		p.data = resp
		p.mu.Unlock()
		fmt.Fprintf(os.Stderr, "Collection complete: %d jobs, %d resources\n", resp.Summary.TotalJobs, resp.Summary.TotalResources)
	}

	refresh()

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

func (p *LiveProvider) Data() (*collector.APIResponse, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.data, nil
}

type FileProvider struct {
	path string
}

func NewFileProvider(path string) *FileProvider {
	return &FileProvider{path: path}
}

func (p *FileProvider) Data() (*collector.APIResponse, error) {
	f, err := os.Open(p.path)
	if err != nil {
		return nil, fmt.Errorf("opening data file: %w", err)
	}
	defer f.Close()

	var resp collector.APIResponse
	if err := json.NewDecoder(f).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decoding data file: %w", err)
	}
	return &resp, nil
}

func Run(ctx context.Context, addr string, provider DataProvider) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(dashboardHTML))
	})

	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		resp, err := provider.Data()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		server.Close()
	}()

	fmt.Fprintf(os.Stderr, "Serving dashboard on %s\n", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
