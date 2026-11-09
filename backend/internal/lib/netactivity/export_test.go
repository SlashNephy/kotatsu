package netactivity

import (
	"context"
	"log/slog"
)

// Flush は、テストのために、1 回分の書き込みを行う。
func Flush(ctx context.Context, r *Recorder, store Store, logger *slog.Logger) {
	r.flush(ctx, store, logger)
}
