package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryProductPopularityCacheConcurrentAccessAndExpiry(t *testing.T) {
	current := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	cache := NewMemoryProductPopularityCache()
	cache.now = func() time.Time { return current }
	value := []ProductPopularity{{ProductID: 1, Name: "Phone"}}
	if err := cache.Set(context.Background(), "key", value, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	const workers = 32
	var waitGroup sync.WaitGroup
	for index := 0; index < workers; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result, found, err := cache.Get(context.Background(), "key")
			if err != nil || !found || len(result) != 1 || result[0].ProductID != 1 {
				t.Errorf("cache Get = %+v/%v/%v", result, found, err)
			}
		}()
	}
	waitGroup.Wait()

	current = current.Add(2 * time.Minute)
	if result, found, err := cache.Get(context.Background(), "key"); err != nil || found || result != nil {
		t.Fatalf("expired cache Get = %+v/%v/%v", result, found, err)
	}
}
