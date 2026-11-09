package mydispatcher

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	xstats "github.com/xtls/xray-core/app/stats"
	xfeatures "github.com/xtls/xray-core/features/stats"
)

func newTestDispatcher(tb testing.TB) *DefaultDispatcher {
	tb.Helper()
	mgr, err := xstats.NewManager(context.Background(), &xstats.Config{})
	if err != nil {
		tb.Fatal(err)
	}
	return &DefaultDispatcher{stats: mgr}
}

func TestUserCountersCache(t *testing.T) {
	d := newTestDispatcher(t)

	up, down := d.UserCounters("test|a@example.test|1000")
	if up == nil || down == nil {
		t.Fatal("counters must be registered")
	}

	// Second lookup returns the same objects without re-registering
	up2, down2 := d.LookupUserCounters("test|a@example.test|1000")
	if up2 != up || down2 != down {
		t.Fatal("cached pointers must be stable")
	}

	// Unknown users are nil without registration side effects
	if u, dn := d.LookupUserCounters("test|ghost@example.test|1"); u != nil || dn != nil {
		t.Fatal("unknown user must look up nil")
	}
	if u, _ := d.UserCounters("test|ghost@example.test|1"); u == nil {
		t.Fatal("UserCounters must register on demand")
	}

	// Removal unregisters from the manager and clears the cache
	d.RemoveUserCounters([]string{
		"test|a@example.test|1000",
		"test|ghost@example.test|1",
	})
	if u, _ := d.LookupUserCounters("test|a@example.test|1000"); u != nil {
		t.Fatal("removed user must no longer resolve")
	}
	if u, _ := d.UserCounters("test|a@example.test|1000"); u == nil || u == up {
		t.Fatal("re-add must register a fresh counter")
	}
}

func TestRemoveTagCounters(t *testing.T) {
	d := newTestDispatcher(t)

	keep := "keep_0.0.0.0_443"
	gone := "gone_0.0.0.0_8443"
	for _, email := range []string{
		keep + "|a@example.test|1000",
		gone + "|b@example.test|1001",
		gone + "|c@example.test|1002",
	} {
		if u, _ := d.UserCounters(email); u == nil {
			t.Fatalf("%s must register", email)
		}
	}

	d.RemoveTagCounters(gone)

	if u, _ := d.LookupUserCounters(keep + "|a@example.test|1000"); u == nil {
		t.Fatal("other tag must be untouched")
	}
	remaining := 0
	d.userCounters.Range(func(key, value any) bool {
		if strings.HasPrefix(key.(string), gone+"|") {
			remaining++
		}
		return true
	})
	if remaining != 0 {
		t.Fatalf("tag entries must be removed, %d left", remaining)
	}
}

type fakeStatsCounter struct{ v int64 }

func (f *fakeStatsCounter) Value() int64      { return atomic.LoadInt64(&f.v) }
func (f *fakeStatsCounter) Set(n int64) int64 { return atomic.SwapInt64(&f.v, n) }
func (f *fakeStatsCounter) Add(n int64) int64 { return atomic.AddInt64(&f.v, n) }

// lostRaceManager simulates losing the GetOrRegisterCounter race: the first
// lookup misses, RegisterCounter reports the counter was registered by a
// concurrent caller, and the follow-up GetCounter finds the winner's counter.
type lostRaceManager struct {
	xfeatures.NoopManager
	mu    sync.Mutex
	looks int
	c     xfeatures.Counter
}

func (m *lostRaceManager) GetCounter(string) xfeatures.Counter {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.looks++
	if m.looks == 1 {
		return nil // initial lookup misses
	}
	return m.c // winner registered it meanwhile
}

func (m *lostRaceManager) RegisterCounter(string) (xfeatures.Counter, error) {
	return nil, errors.New("Counter already registered.")
}

// TestUserCountersLostRegistrationRace guards against caching a nil counter
// when two connections of the same user race the first registration: the
// loser must fall back to the winner's counter instead of storing nil,
// which would disable traffic accounting for the user permanently.
func TestUserCountersLostRegistrationRace(t *testing.T) {
	mgr := &lostRaceManager{c: &fakeStatsCounter{}}
	d := &DefaultDispatcher{stats: mgr}

	up, down := d.UserCounters("test|lost|1")
	if up == nil || down == nil {
		t.Fatalf("lost registration must fall back to the winner's counter, got up=%v down=%v", up, down)
	}
	v, ok := d.userCounters.Load("test|lost|1")
	if !ok {
		t.Fatal("valid pair must be cached")
	}
	pair := v.(*userCounterPair)
	if pair.up == nil || pair.down == nil {
		t.Fatal("cached pair must not contain nil counters")
	}
}

// TestUserCountersNoCacheOnNil ensures a broken pair (stats manager without
// counters) is never cached, so later connections can retry cleanly.
func TestUserCountersNoCacheOnNil(t *testing.T) {
	d := &DefaultDispatcher{stats: xfeatures.NoopManager{}}

	if up, down := d.UserCounters("test|noop|1"); up != nil || down != nil {
		t.Fatalf("noop manager must yield nil counters, got up=%v down=%v", up, down)
	}
	if _, ok := d.userCounters.Load("test|noop|1"); ok {
		t.Fatal("incomplete pair must not be cached")
	}
}
