package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetSet(t *testing.T) {
	c := New[string](time.Minute)
	c.Set("k1", "v1")

	v, ok := c.Get("k1")
	if !ok || v != "v1" {
		t.Fatalf("expected v1, got %v (ok=%v)", v, ok)
	}

	_, ok = c.Get("missing")
	if ok {
		t.Fatal("expected miss for non-existent key")
	}
}

func TestTTLExpiry(t *testing.T) {
	c := New[string](50 * time.Millisecond)
	c.Set("k1", "v1")

	v, ok := c.Get("k1")
	if !ok || v != "v1" {
		t.Fatal("expected hit before expiry")
	}

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get("k1")
	if ok {
		t.Fatal("expected miss after expiry")
	}
}

func TestDelete(t *testing.T) {
	c := New[int](time.Minute)
	c.Set("k", 42)
	c.Delete("k")
	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected miss after delete")
	}
}

func TestGetOrLoad_CacheMiss(t *testing.T) {
	c := New[string](time.Minute)
	v, err := c.GetOrLoad("k1", func() (string, error) {
		return "loaded", nil
	})
	if err != nil || v != "loaded" {
		t.Fatalf("expected loaded, got %v err=%v", v, err)
	}

	// Second call should hit cache (loader not called)
	v, err = c.GetOrLoad("k1", func() (string, error) {
		t.Fatal("loader should not be called on cache hit")
		return "", nil
	})
	if err != nil || v != "loaded" {
		t.Fatalf("expected cached value loaded, got %v", v)
	}
}

func TestGetOrLoad_ErrorNotCached(t *testing.T) {
	c := New[string](time.Minute)
	loadErr := errors.New("fail")
	_, err := c.GetOrLoad("k1", func() (string, error) {
		return "", loadErr
	})
	if !errors.Is(err, loadErr) {
		t.Fatalf("expected loadErr, got %v", err)
	}

	// Key should not be cached after error
	_, ok := c.Get("k1")
	if ok {
		t.Fatal("error result should not be cached")
	}
}

func TestGetOrLoad_Singleflight(t *testing.T) {
	c := New[string](time.Minute)
	var calls atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.GetOrLoad("k1", func() (string, error) {
				calls.Add(1)
				time.Sleep(50 * time.Millisecond)
				return "result", nil
			})
			if err != nil || v != "result" {
				t.Errorf("unexpected: v=%v err=%v", v, err)
			}
		}()
	}
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Fatalf("expected 1 loader call (singleflight), got %d", n)
	}
}

func TestCleanup(t *testing.T) {
	c := New[string](50 * time.Millisecond)
	c.Set("k1", "v1")
	c.Set("k2", "v2")

	stop := make(chan struct{})
	c.StartCleanup(30*time.Millisecond, stop)

	time.Sleep(100 * time.Millisecond)

	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()

	close(stop)

	if n != 0 {
		t.Fatalf("expected 0 items after cleanup, got %d", n)
	}
}
