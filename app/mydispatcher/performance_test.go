package mydispatcher

import (
	"context"
	"runtime"
	"testing"

	"github.com/wyx2685/XrayR/api"
	"github.com/wyx2685/XrayR/common/limiter"
	apppolicy "github.com/xtls/xray-core/app/policy"
	appstats "github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/transport"
)

func fixtureDispatcher(t testing.TB) (*DefaultDispatcher, context.Context) {
	t.Helper()
	pm, err := apppolicy.New(context.Background(), &apppolicy.Config{Level: map[uint32]*apppolicy.Policy{0: {Stats: &apppolicy.Policy_Stats{UserUplink: true, UserDownlink: true}}}})
	if err != nil {
		t.Fatal(err)
	}
	sm, err := appstats.NewManager(context.Background(), &appstats.Config{})
	if err != nil {
		t.Fatal(err)
	}
	d := &DefaultDispatcher{policy: pm, stats: sm, Limiter: limiter.New()}
	users := []api.UserInfo{{UID: 1000, Email: "user@example.test", SpeedLimit: 1000000}}
	if err := d.Limiter.AddInboundLimiter("test", 0, &users, nil); err != nil {
		t.Fatal(err)
	}
	ctx := session.ContextWithInbound(context.Background(), &session.Inbound{Tag: "test", User: &protocol.MemoryUser{Email: "test|user@example.test|1000"}, Source: net.TCPDestination(net.ParseAddress("192.0.2.1"), 1234)})
	ctx = policy.ContextWithBufferPolicy(ctx, policy.Buffer{PerConnection: 64 * 1024})
	return d, ctx
}

func closeLinks(in, out *transport.Link) {
	common.Close(in.Writer)
	common.Close(out.Writer)
	common.Interrupt(in.Reader)
	common.Interrupt(out.Reader)
}

func TestDispatcherTrafficAndLimiter(t *testing.T) {
	d, ctx := fixtureDispatcher(t)
	in, out, err := d.getLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLinks(in, out)
	payload := []byte("authenticated payload")
	if err := in.Writer.WriteMultiBuffer(buf.MergeBytes(nil, payload)); err != nil {
		t.Fatal(err)
	}
	mb, err := out.Reader.ReadMultiBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if mb.Len() != int32(len(payload)) {
		t.Fatal("payload length changed")
	}
	buf.ReleaseMulti(mb)
	if err := out.Writer.WriteMultiBuffer(buf.MergeBytes(nil, payload)); err != nil {
		t.Fatal(err)
	}
	mb, err = in.Reader.ReadMultiBuffer()
	if err != nil {
		t.Fatal(err)
	}
	buf.ReleaseMulti(mb)
	for _, direction := range []string{"uplink", "downlink"} {
		c := d.stats.GetCounter("user>>>test|user@example.test|1000>>>traffic>>>" + direction)
		if c == nil || c.Value() != int64(len(payload)) {
			t.Fatalf("%s traffic incorrect", direction)
		}
	}
	online, err := d.Limiter.GetOnlineDevice("test")
	if err != nil || len(*online) != 1 {
		t.Fatal("online IP not recorded")
	}
}

func BenchmarkDispatcherGetLink(b *testing.B) {
	d, ctx := fixtureDispatcher(b)
	in, out, err := d.getLink(ctx)
	if err != nil {
		b.Fatal(err)
	}
	closeLinks(in, out)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in, out, err := d.getLink(ctx)
		if err != nil {
			b.Fatal(err)
		}
		closeLinks(in, out)
	}
}

func TestDispatcherRetainedLinks(t *testing.T) {
	d, ctx := fixtureDispatcher(t)
	in, out, err := d.getLink(ctx)
	if err != nil {
		t.Fatal(err)
	}
	closeLinks(in, out)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	g := runtime.NumGoroutine()
	const n = 1000
	links := make([][2]*transport.Link, n)
	for i := range links {
		a, b, err := d.getLink(ctx)
		if err != nil {
			t.Fatal(err)
		}
		links[i] = [2]*transport.Link{a, b}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("links=%d heap_delta=%d heap_per_link=%d object_delta=%d goroutine_delta=%d; getLink only, no socket/relay/TLS/payload", n, int64(after.HeapAlloc)-int64(before.HeapAlloc), (int64(after.HeapAlloc)-int64(before.HeapAlloc))/n, int64(after.HeapObjects)-int64(before.HeapObjects), runtime.NumGoroutine()-g)
	for _, pair := range links {
		closeLinks(pair[0], pair[1])
	}
	runtime.KeepAlive(d)
}
