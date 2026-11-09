package limiter

import (
	"context"
	"runtime"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestWaitNSurvivesBurstShrinkRace hammers waitN while a goroutine keeps
// shrinking and restoring the burst (as UpdateInboundLimiter does when a
// speed-limit change lands mid-transfer). WaitN hard-fails with
// "n exceeds limiter's burst" when the burst shrinks between the caller's
// read and its own check; waitN must retry instead of surfacing the error
// and tearing down the connection.
func TestWaitNSurvivesBurstShrinkRace(t *testing.T) {
	l := rate.NewLimiter(rate.Limit(1<<40), 1000)
	w := &Writer{limiter: l, ctx: context.Background()}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			l.SetBurst(500)
			l.SetBurst(1000)
			runtime.Gosched()
		}
	}()

	for i := 0; i < 20000; i++ {
		if err := w.waitN(2500); err != nil {
			close(stop)
			<-done
			t.Fatalf("waitN must survive concurrent burst shrink: %v", err)
		}
	}
	close(stop)
	<-done
}

// TestWaitNThrottles ensures the chunked waiting still limits throughput
// and drains the requested size exactly once.
func TestWaitNThrottles(t *testing.T) {
	const bytesPerSecond = 200000
	l := rate.NewLimiter(rate.Limit(bytesPerSecond), bytesPerSecond)
	w := &Writer{limiter: l, ctx: context.Background()}

	start := time.Now()
	// 3x burst: the first burst passes immediately, the rest waits ~2s.
	if err := w.waitN(3 * bytesPerSecond); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("waitN must throttle beyond burst, elapsed %v", elapsed)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("waitN waited too long: %v", elapsed)
	}
}
