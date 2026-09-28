package sender

import (
	"bufio"
	"bytes"
	"io"
	"unicode/utf8"

	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

type lineReader struct {
	reader    *bufio.Reader
	prefix    []byte
	size      int64
	total     int64
	truncated bool
}

func newLineReader(r io.Reader) *lineReader { return &lineReader{reader: bufio.NewReaderSize(r, 4096)} }

// read returns complete records only. An EOF leaves a partial line buffered for a later read.
func (l *lineReader) read() (ingest.Record, bool, error) {
	for {
		part, err := l.reader.ReadSlice('\n')
		l.size += int64(len(part))
		l.total += int64(len(part))
		content := part
		if err == nil && len(content) > 0 {
			content = content[:len(content)-1] // ReadSlice includes the delimiter.
		}
		keep := len(content)
		if room := ingest.MaxLineBytes - len(l.prefix); keep > room {
			keep = room
			l.truncated = true
		}
		if keep > 0 {
			l.prefix = append(l.prefix, content[:keep]...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == nil {
			return l.finish(), true, nil
		}
		if err == io.EOF {
			return ingest.Record{}, false, nil
		}
		return ingest.Record{}, false, err
	}
}

func (l *lineReader) finish() ingest.Record {
	data := l.prefix
	data = bytes.ToValidUTF8(data, []byte("\ufffd"))
	truncated := l.truncated || len(data) > ingest.MaxLineBytes
	if len(data) > ingest.MaxLineBytes {
		data = data[:ingest.MaxLineBytes]
		for !utf8.Valid(data) {
			data = data[:len(data)-1]
		}
	}
	row := ingest.Record{Line: string(data), Truncated: truncated}
	l.prefix = nil
	l.size = 0
	l.truncated = false
	return row
}

func (l *lineReader) partial() bool { return l.size > 0 }
