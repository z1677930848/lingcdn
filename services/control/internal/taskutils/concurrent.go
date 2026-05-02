package taskutils

import (
	"context"

	"golang.org/x/sync/errgroup"
)

func RunConcurrent[T any](ctx context.Context, items []T, limit int, fn func(context.Context, T) error) error {
	if len(items) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = 8
	}
	if limit > len(items) {
		limit = len(items)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for _, it := range items {
		it := it
		g.Go(func() error {
			return fn(gctx, it)
		})
	}
	return g.Wait()
}

