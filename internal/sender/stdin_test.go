package sender

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStdinFinalPartialAndReplacement(t *testing.T) {
	ch := make(chan Item, 3)
	err := ReadStdin(context.Background(), strings.NewReader("one\n"+string([]byte{'x', 0xff, 'y'})+"\nlast"), ch)
	if err != nil {
		t.Fatal(err)
	}
	one, bad, last := <-ch, <-ch, <-ch
	if one.Record.Line != "one" || bad.Record.Line != "x\ufffdy" || last.Record.Line != "last" || last.Position.Input != "stdin" {
		t.Fatalf("items: %+v %+v %+v", one, bad, last)
	}
}

func TestStdinOversizedThenGood(t *testing.T) {
	ch := make(chan Item, 2)
	err := ReadStdin(context.Background(), strings.NewReader(strings.Repeat("x", 20<<10)+"\ngood\n"), ch)
	if err != nil {
		t.Fatal(err)
	}
	big, good := <-ch, <-ch
	if !big.Record.Truncated || len(big.Record.Line) != 16<<10 || good.Record.Line != "good" {
		t.Fatalf("items: %+v %+v", big, good)
	}
}

func TestStdinInvalidBytesStayWithinWireLimit(t *testing.T) {
	ch := make(chan Item, 1)
	input := strings.Repeat("a\xff", 8<<10) + "\n"
	if err := ReadStdin(context.Background(), strings.NewReader(input), ch); err != nil {
		t.Fatal(err)
	}
	row := (<-ch).Record
	if !row.Truncated || len(row.Line) > 16<<10 || !utf8.ValidString(row.Line) {
		t.Fatalf("invalid bounded line: %+v", row)
	}
}

func TestStdinExactLimitIsNotTruncated(t *testing.T) {
	ch := make(chan Item, 1)
	if err := ReadStdin(context.Background(), strings.NewReader(strings.Repeat("x", 16<<10)+"\n"), ch); err != nil {
		t.Fatal(err)
	}
	row := (<-ch).Record
	if row.Truncated || len(row.Line) != 16<<10 {
		t.Fatalf("exact limit: %+v", row)
	}
}
