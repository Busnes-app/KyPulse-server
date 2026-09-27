package sender

import (
	"context"
	"io"
)

// ReadStdin reads a finite stream. Its final unterminated record is delivered on EOF.
func ReadStdin(ctx context.Context, r io.Reader, out chan<- Item) error {
	lines := newLineReader(r)
	emit := func(row Item) error {
		select {
		case out <- row:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	for {
		row, ok, err := lines.read()
		if err != nil {
			return err
		}
		if !ok {
			if lines.partial() {
				return emit(Item{Record: lines.finish(), Position: Position{Kind: "stdin", Input: "stdin", Offset: lines.total}})
			}
			return nil
		}
		if err := emit(Item{Record: row, Position: Position{Kind: "stdin", Input: "stdin", Offset: lines.total}}); err != nil {
			return err
		}
	}
}
