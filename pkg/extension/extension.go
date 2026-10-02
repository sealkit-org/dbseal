package extension

import (
	"context"
	"sync"
	"time"
)

// Pipeline defines the database data streaming/subsetting pipeline interface.
type Pipeline interface {
	Name() string
	Run(ctx context.Context, plan any) error
}

// Masker defines the field-level data anonymization interface.
type Masker interface {
	Name() string
	Mask(val any, rule string) (any, error)
}

// Global registry for pipelines and maskers.
var (
	mu              sync.RWMutex
	currentPipeline Pipeline
	customMaskers   = make(map[string]Masker)
	baseMaskerFn    func(val any, rule string) (any, error)
)

// SetBaseMasker sets the base OSS rule evaluator.
func SetBaseMasker(fn func(val any, rule string) (any, error)) {
	mu.Lock()
	defer mu.Unlock()
	baseMaskerFn = fn
}

// ApplyBaseRule invokes the base rule evaluator.
func ApplyBaseRule(val any, rule string) (any, error) {
	mu.RLock()
	defer mu.RUnlock()
	if baseMaskerFn != nil {
		return baseMaskerFn(val, rule)
	}
	return val, nil
}

// SetPipeline allows Pro/commercial wrapper to override the default pipeline.
func SetPipeline(p Pipeline) {
	mu.Lock()
	defer mu.Unlock()
	currentPipeline = p
}

// GetPipeline returns the currently active pipeline.
func GetPipeline() Pipeline {
	mu.RLock()
	defer mu.RUnlock()
	return currentPipeline
}

// RegisterMasker registers an advanced masking operator (e.g. JSONPath, dynamic HMAC).
func RegisterMasker(name string, m Masker) {
	mu.Lock()
	defer mu.Unlock()
	customMaskers[name] = m
}

// GetMasker retrieves a registered custom masker.
func GetMasker(name string) (Masker, bool) {
	mu.RLock()
	defer mu.RUnlock()
	m, ok := customMaskers[name]
	return m, ok
}

// SyncTableFunc defines the function to sync a single table.
type SyncTableFunc func(ctx context.Context, table string) (rows int64, duration time.Duration, maskedCols []string, err error)

// ResetFunc defines the post-sync auto-increment / sequence reset function.
type ResetFunc func(ctx context.Context) error

// SyncPlan contains the orchestrator data passed to custom pipelines.
type SyncPlan struct {
	SourceName string
	TargetName string
	Tables     []string
	SyncTable  SyncTableFunc
	ResetSeeds ResetFunc
}

