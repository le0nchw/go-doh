package startup

import (
	"sync"
	"testing"
)

// TestCachedTimeNoRace validates F9: concurrent reads of Now() while the
// timestamp is being written must not race (run with -race).
func TestCachedTimeNoRace(t *testing.T) {
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = Now()
				}
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		CachedTime.Store(int64(i))
	}
	close(stop)
	wg.Wait()
}
