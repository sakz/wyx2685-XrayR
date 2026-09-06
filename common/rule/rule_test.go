package rule

import (
	"fmt"
	"github.com/wyx2685/XrayR/api"
	"regexp"
	"testing"
)

func TestRuleDetection(t *testing.T) {
	r := New()
	r.UpdateRule("test", []api.DetectRule{{ID: 7, Pattern: regexp.MustCompile("blocked")}, {ID: 8, Pattern: regexp.MustCompile("blocked")}})
	if r.Detect("test", "allowed.example:443", "test|email|123") {
		t.Fatal("false positive")
	}
	for i := 0; i < 10; i++ {
		if !r.Detect("test", "blocked.example:443", "test|email|123") {
			t.Fatal("missed rule")
		}
	}
	got, _ := r.GetDetectResult("test")
	if len(*got) != 1 || (*got)[0] != (api.DetectResult{UID: 123, RuleID: 7}) {
		t.Fatalf("dedup or rule order changed: %v", got)
	}
	got, _ = r.GetDetectResult("test")
	if len(*got) != 0 {
		t.Fatal("result not drained")
	}
	if !r.Detect("test", "blocked.example:443", "invalid") {
		t.Fatal("invalid UID must still reject")
	}
	got, _ = r.GetDetectResult("test")
	if len(*got) != 0 {
		t.Fatal("invalid UID must not record")
	}
	r.UpdateRule("local", []api.DetectRule{{ID: -1, Pattern: regexp.MustCompile("blocked")}})
	if !r.Detect("local", "blocked", "123") {
		t.Fatal("local rule must reject")
	}
	got, _ = r.GetDetectResult("local")
	if len(*got) != 0 {
		t.Fatal("local rule must not report")
	}
}

func BenchmarkRuleDetect(b *testing.B) {
	for _, n := range []int{1, 20} {
		for _, hit := range []bool{false, true} {
			b.Run(fmt.Sprintf("rules=%d/hit=%t", n, hit), func(b *testing.B) {
				r := New()
				rules := make([]api.DetectRule, n)
				for i := range rules {
					rules[i] = api.DetectRule{ID: i, Pattern: regexp.MustCompile(fmt.Sprintf(`blocked%d\.`, i))}
				}
				r.UpdateRule("test", rules)
				dest := "allowed.example:443"
				if hit {
					dest = fmt.Sprintf("blocked%d.example:443", n-1)
				}
				r.Detect("test", dest, "test|email|123")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r.Detect("test", dest, "test|email|123")
				}
			})
		}
	}
}

func TestRuleConcurrentFirstHit(t *testing.T) {
	r := New()
	r.UpdateRule("test", []api.DetectRule{{ID: 7, Pattern: regexp.MustCompile("blocked")}})
	const n = 32
	done := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func() { done <- r.Detect("test", "blocked.example:443", "test|email|123") }()
	}
	for i := 0; i < n; i++ {
		if !<-done {
			t.Fatal("rule missed")
		}
	}
	got, _ := r.GetDetectResult("test")
	if len(*got) != 1 {
		t.Fatalf("expected one result, got %v", got)
	}
}
