package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type ProductPopularityCache interface {
	Get(context.Context, string) ([]ProductPopularity, bool, error)
	Set(context.Context, string, []ProductPopularity, time.Duration) error
}

type ProductPopularityCacheStats struct {
	Hits   uint64
	Misses uint64
}

type productPopularityCacheEntry struct {
	value     []ProductPopularity
	expiresAt time.Time
}

type MemoryProductPopularityCache struct {
	mu      sync.RWMutex
	entries map[string]productPopularityCacheEntry
	now     func() time.Time
	hits    atomic.Uint64
	misses  atomic.Uint64
}

func NewMemoryProductPopularityCache() *MemoryProductPopularityCache {
	return &MemoryProductPopularityCache{
		entries: make(map[string]productPopularityCacheEntry),
		now:     time.Now,
	}
}

func (cache *MemoryProductPopularityCache) Get(
	_ context.Context,
	key string,
) ([]ProductPopularity, bool, error) {
	if cache == nil {
		return nil, false, nil
	}
	now := cache.now()
	cache.mu.RLock()
	entry, exists := cache.entries[key]
	cache.mu.RUnlock()
	if !exists || !entry.expiresAt.After(now) {
		if exists {
			cache.mu.Lock()
			if current, ok := cache.entries[key]; ok && !current.expiresAt.After(now) {
				delete(cache.entries, key)
			}
			cache.mu.Unlock()
		}
		cache.misses.Add(1)
		return nil, false, nil
	}
	cache.hits.Add(1)
	return cloneProductPopularity(entry.value), true, nil
}

func (cache *MemoryProductPopularityCache) Set(
	_ context.Context,
	key string,
	value []ProductPopularity,
	ttl time.Duration,
) error {
	if cache == nil || ttl <= 0 {
		return nil
	}
	cache.mu.Lock()
	cache.entries[key] = productPopularityCacheEntry{
		value:     cloneProductPopularity(value),
		expiresAt: cache.now().Add(ttl),
	}
	cache.mu.Unlock()
	return nil
}

func (cache *MemoryProductPopularityCache) Stats() ProductPopularityCacheStats {
	if cache == nil {
		return ProductPopularityCacheStats{}
	}
	return ProductPopularityCacheStats{
		Hits:   cache.hits.Load(),
		Misses: cache.misses.Load(),
	}
}

func cloneProductPopularity(value []ProductPopularity) []ProductPopularity {
	if value == nil {
		return nil
	}
	return append([]ProductPopularity(nil), value...)
}
