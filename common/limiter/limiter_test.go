package limiter

import (
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/wyx2685/XrayR/api"
	"golang.org/x/time/rate"
)

func testLimiter(tb testing.TB, n int, speed uint64) (*Limiter, []string) {
	tb.Helper()
	users := make([]api.UserInfo, n)
	keys := make([]string, n)
	for i := range users {
		users[i] = api.UserInfo{UID: i + 1000, Email: fmt.Sprintf("user%d@example.test", i), SpeedLimit: speed}
		keys[i] = fmt.Sprintf("test|%s|%d", users[i].Email, users[i].UID)
	}
	l := New()
	if err := l.AddInboundLimiter("test", 0, &users, nil); err != nil {
		tb.Fatal(err)
	}
	return l, keys
}

func TestBucketLifecycle(t *testing.T) {
	l, keys := testLimiter(t, 1, 1000000)
	key := keys[0]
	first, limited, reject := l.GetUserBucket("test", key, "192.0.2.1", true)
	if first == nil || !limited || reject {
		t.Fatal("first TCP connection rejected or not limited")
	}
	for i := 0; i < 10; i++ {
		next, limited, reject := l.GetUserBucket("test", key, "192.0.2.1", true)
		if next != first || !limited || reject {
			t.Fatal("bucket must be shared")
		}
	}
	l.GetUserBucket("test", key, "192.0.2.2", true)
	online, err := l.GetOnlineDevice("test")
	if err != nil || len(*online) != 2 {
		t.Fatalf("online=%v err=%v", online, err)
	}
	// Reporting keeps the bucket for users active in this period.
	next, _, _ := l.GetUserBucket("test", key, "192.0.2.1", true)
	if next != first {
		t.Fatal("report must preserve active bucket")
	}
	l.GetOnlineDevice("test")
	l.GetOnlineDevice("test")
	next, _, _ = l.GetUserBucket("test", key, "192.0.2.1", true)
	if next == first {
		t.Fatal("inactive bucket must be evicted")
	}
	users := []api.UserInfo{{UID: 1000, Email: "user0@example.test", SpeedLimit: 2000000}}
	l.UpdateInboundLimiter("test", &users)
	if next.Limit() != rate.Limit(2000000) {
		t.Fatal("live rate update lost")
	}
	users[0].SpeedLimit = 0
	l.UpdateInboundLimiter("test", &users)
	next, limited, reject = l.GetUserBucket("test", key, "192.0.2.1", true)
	if next != nil || limited || reject {
		t.Fatal("unlimited connection changed")
	}
	l.DeleteInboundLimiter("test")
	if _, err := l.GetOnlineDevice("test"); err == nil {
		t.Fatal("missing inbound must fail")
	}
}

func TestBucketDeviceBranches(t *testing.T) {
	for _, tcp := range []bool{true, false} {
		t.Run(fmt.Sprint(tcp), func(t *testing.T) {
			l, keys := testLimiter(t, 1, 0)
			v, _ := l.InboundInfo.Load("test")
			in := v.(*InboundInfo)
			in.UserInfo.Store(keys[0], UserInfo{UID: 1000, DeviceLimit: 1})
			in.AliveList = map[int]int{1000: 1}
			_, _, reject := l.GetUserBucket("test", keys[0], "192.0.2.1", tcp)
			if reject != tcp {
				t.Fatal("device limit must apply only to TCP")
			}
			in.OldUserOnline.Store("192.0.2.1", 1000)
			_, _, reject = l.GetUserBucket("test", keys[0], "192.0.2.1", tcp)
			if reject {
				t.Fatal("previous online IP must be admitted")
			}
			_, _, reject = l.GetUserBucket("test", keys[0], "192.0.2.1", tcp)
			if reject {
				t.Fatal("repeated IP must be admitted")
			}
			_, _, reject = l.GetUserBucket("test", keys[0], "192.0.2.2", tcp)
			if reject != tcp {
				t.Fatal("new IP at limit")
			}
			online, _ := l.GetOnlineDevice("test")
			want := 0
			if tcp {
				want = 1
			}
			if len(*online) != want {
				t.Fatalf("online=%v", online)
			}
		})
	}
}

func TestBucketConcurrent(t *testing.T) {
	l, keys := testLimiter(t, 1, 1000000)
	const n = 32
	buckets := make([]*rate.Limiter, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range buckets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				buckets[i], _, _ = l.GetUserBucket("test", keys[0], "192.0.2.1", true)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for _, b := range buckets {
		if b != buckets[0] || b == nil {
			t.Fatal("concurrent miss must use the same bucket")
		}
	}
	online, _ := l.GetOnlineDevice("test")
	if len(*online) != 1 {
		t.Fatal("duplicate IP")
	}
}

func BenchmarkGetUserBucket(b *testing.B) {
	for _, n := range []int{100, 1000, 10000, 50000} {
		for _, tcp := range []bool{true, false} {
			b.Run(fmt.Sprintf("users=%d/tcp=%t", n, tcp), func(b *testing.B) {
				l, keys := testLimiter(b, n, 1000000)
				for _, key := range keys {
					l.GetUserBucket("test", key, "192.0.2.1", tcp)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					l.GetUserBucket("test", keys[i%n], "192.0.2.1", tcp)
				}
			})
		}
	}
}

func BenchmarkBucketFirstConnection(b *testing.B) {
	l, keys := testLimiter(b, 1, 1000000)
	v, _ := l.InboundInfo.Load("test")
	in := v.(*InboundInfo)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.GetUserBucket("test", keys[0], "192.0.2.1", true)
		in.UserOnlineIP.Delete(keys[0])
		in.BucketHub.Delete(keys[0])
	}
}

func BenchmarkBucketParallel(b *testing.B) {
	l, keys := testLimiter(b, 1, 1000000)
	l.GetUserBucket("test", keys[0], "192.0.2.1", true)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l.GetUserBucket("test", keys[0], "192.0.2.1", true)
		}
	})
}

func TestLimiterRetainedMemory(t *testing.T) {
	for _, n := range []int{100, 1000, 10000, 50000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			runtime.GC()
			var before, configured, active runtime.MemStats
			runtime.ReadMemStats(&before)
			g := runtime.NumGoroutine()
			l, keys := testLimiter(t, n, 1000000)
			runtime.GC()
			runtime.ReadMemStats(&configured)
			for _, key := range keys {
				l.GetUserBucket("test", key, "192.0.2.1", true)
			}
			runtime.GC()
			runtime.ReadMemStats(&active)
			t.Logf("users=%d configured_heap_delta=%d active_heap_delta=%d objects_configured=%d objects_active=%d goroutine_delta=%d (includes fixture keys; not full core)", n, int64(configured.HeapAlloc)-int64(before.HeapAlloc), int64(active.HeapAlloc)-int64(configured.HeapAlloc), int64(configured.HeapObjects)-int64(before.HeapObjects), int64(active.HeapObjects)-int64(configured.HeapObjects), runtime.NumGoroutine()-g)
			runtime.KeepAlive(l)
			runtime.KeepAlive(keys)
		})
	}
}

// Fixed operation count makes GC/total allocation comparable across versions.
func TestBucketAllocationWorkload(t *testing.T) {
	l, keys := testLimiter(t, 1000, 1000000)
	for _, key := range keys {
		l.GetUserBucket("test", key, "192.0.2.1", true)
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	const operations = 1000000
	for i := 0; i < operations; i++ {
		l.GetUserBucket("test", keys[i%len(keys)], "192.0.2.1", true)
	}
	runtime.ReadMemStats(&after)
	t.Logf("operations=%d allocated_bytes=%d allocations=%d num_gc=%d gc_pause_ns=%d", operations, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, after.NumGC-before.NumGC, after.PauseTotalNs-before.PauseTotalNs)
	runtime.KeepAlive(l)
}
