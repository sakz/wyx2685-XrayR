package controller

import (
	"sync/atomic"
	"testing"

	"github.com/wyx2685/XrayR/api"
	"github.com/xtls/xray-core/features/stats"
)

type fakeCounter struct{ v int64 }

func (f *fakeCounter) Value() int64      { return atomic.LoadInt64(&f.v) }
func (f *fakeCounter) Set(n int64) int64 { return atomic.SwapInt64(&f.v, n) }
func (f *fakeCounter) Add(n int64) int64 { return atomic.AddInt64(&f.v, n) }

func TestDeductTraffic(t *testing.T) {
	c := &Controller{}

	up, down := &fakeCounter{}, &fakeCounter{}
	up.Add(100)
	down.Add(300)
	// Traffic arriving while the report is in flight must survive deduction
	up.Add(5)

	traffic := []api.UserTraffic{{UID: 1, Upload: 100, Download: 300}}
	upList := []stats.Counter{up}
	downList := []stats.Counter{down}

	c.deductTraffic(&traffic, &upList, &downList)

	if got := up.Value(); got != 5 {
		t.Fatalf("uplink: want in-flight remainder 5, got %d", got)
	}
	if got := down.Value(); got != 0 {
		t.Fatalf("downlink: want 0, got %d", got)
	}

	// Next cycle reports the remainder plus new traffic exactly once
	up.Add(20)
	if up.Value() != 25 {
		t.Fatalf("precondition failed: %d", up.Value())
	}
	traffic = []api.UserTraffic{{UID: 1, Upload: 25, Download: 0}}
	c.deductTraffic(&traffic, &upList, &downList)
	if got := up.Value(); got != 0 {
		t.Fatalf("second deduction must drain the counter, got %d", got)
	}
}

func TestDeductTrafficTolerance(t *testing.T) {
	c := &Controller{}
	ctr := &fakeCounter{}
	ctr.Add(42)
	// Nil counters and shorter lists must not panic
	traffic := []api.UserTraffic{{UID: 1, Upload: 42, Download: 0}}
	nilList := []stats.Counter{nil}
	emptyList := []stats.Counter{}
	c.deductTraffic(&traffic, &nilList, &emptyList)
	if ctr.Value() != 42 {
		t.Fatal("nil counter slot must be skipped")
	}
}
