package limiter

import (
	"context"

	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	"golang.org/x/time/rate"
)

type Writer struct {
	writer  buf.Writer
	limiter *rate.Limiter
	ctx     context.Context // connection context, cancels pending waits on teardown
}

func (l *Limiter) RateWriter(ctx context.Context, writer buf.Writer, limiter *rate.Limiter) buf.Writer {
	return &Writer{
		writer:  writer,
		limiter: limiter,
		ctx:     ctx,
	}
}

func (w *Writer) Close() error {
	return common.Close(w.writer)
}

func (w *Writer) WriteMultiBuffer(mb buf.MultiBuffer) error {
	if err := w.waitN(int(mb.Len())); err != nil {
		buf.ReleaseMulti(mb)
		return err
	}
	return w.writer.WriteMultiBuffer(mb)
}

// waitN waits for n tokens, splitting requests larger than the burst so they
// are still throttled instead of failing with "n exceeds burst" (which used
// to bypass the limit entirely for low-rate users).
func (w *Writer) waitN(n int) error {
	for n > 0 {
		take := n
		if burst := w.limiter.Burst(); burst > 0 && burst < take {
			take = burst
		}
		if err := w.limiter.WaitN(w.ctx, take); err != nil {
			if w.ctx.Err() != nil {
				// Connection torn down while waiting.
				return err
			}
			// The only remaining WaitN failure is "n exceeds limiter's
			// burst": a concurrent speed-limit update shrank the burst
			// between our read and WaitN's own check. Re-read the burst
			// and retry instead of tearing down the connection.
			continue
		}
		n -= take
	}
	return nil
}
