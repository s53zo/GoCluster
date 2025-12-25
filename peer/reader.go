package peer

import (
	"net"
	"time"
)

type lineReader struct {
	conn    net.Conn
	parser  *telnetParser
	buf     []byte
	replyFn func([]byte)
	maxLine int
}

// errLineTooLong carries a preview and length when a frame exceeds maxLine.
type errLineTooLong struct {
	preview string
	length  int
}

func (e errLineTooLong) Error() string {
	return "line too long"
}

func newLineReader(conn net.Conn, maxLine int, replyFn func([]byte)) *lineReader {
	return &lineReader{
		conn:    conn,
		parser:  &telnetParser{},
		buf:     make([]byte, 0, maxLine),
		replyFn: replyFn,
		maxLine: maxLine,
	}
}

func (r *lineReader) ReadLine(deadline time.Time) (string, error) {
	if err := r.conn.SetReadDeadline(deadline); err != nil {
		return "", err
	}
	for {
		chunk := make([]byte, 1024)
		n, err := r.conn.Read(chunk)
		if n > 0 {
			out, replies := r.parser.Feed(chunk[:n])
			if len(replies) > 0 && r.replyFn != nil {
				for _, rep := range replies {
					r.replyFn(rep)
				}
			}
			r.buf = append(r.buf, out...)
			for {
				r.buf = trimLeadingTerminators(r.buf)
				if len(r.buf) == 0 {
					break
				}
				// Prefer explicit terminators (~, CRLF, CR, LF) when present.
				if idx, size := bytesIndexTerminator(r.buf); idx >= 0 {
					line := string(trimLine(r.buf[:idx]))
					r.buf = append([]byte{}, r.buf[idx+size:]...)
					return line, nil
				}
				// Resync: discard leading noise until a valid PCxx^ frame start that follows a terminator.
				// This avoids splitting on "^PC" sequences that might appear inside payload fields.
				if start := bytesIndexFrameStart(r.buf); start > 0 {
					r.buf = r.buf[start:]
					continue
				}
				if len(r.buf) > r.maxLine && r.maxLine > 0 {
					// Drop the current buffer to avoid unbounded growth; caller can choose to continue.
					preview := string(r.buf)
					r.buf = r.buf[:0]
					return "", errLineTooLong{preview: preview, length: len(preview)}
				}
				break
			}
		}
		if err != nil {
			return "", err
		}
	}
}

func trimLine(b []byte) []byte {
	for len(b) > 0 {
		if b[len(b)-1] == '\n' || b[len(b)-1] == '\r' {
			b = b[:len(b)-1]
		} else {
			break
		}
	}
	return b
}

// trimLeadingTerminators discards any leading CR/LF/~ bytes so frames start cleanly.
func trimLeadingTerminators(b []byte) []byte {
	for len(b) > 0 {
		if isTerminator(b[0]) {
			b = b[1:]
			continue
		}
		break
	}
	return b
}

func isTerminator(b byte) bool {
	return b == '\n' || b == '\r' || b == '~'
}

// bytesIndexTerminator returns the index and width of the first terminator (~, CRLF, CR, LF).
// We prefer ~ as a hard frame end; CR/LF are legacy telnet line ends.
func bytesIndexTerminator(b []byte) (int, int) {
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '~':
			return i, 1
		case '\n':
			return i, 1
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				return i, 2
			}
			return i, 1
		}
	}
	return -1, 0
}

// bytesIndexFrameStart finds a valid PCxx^ frame start at buffer start or after a terminator.
func bytesIndexFrameStart(b []byte) int {
	if isFrameStartAt(b, 0) {
		return 0
	}
	for i := 1; i < len(b); i++ {
		if !isTerminator(b[i-1]) {
			continue
		}
		if isFrameStartAt(b, i) {
			return i
		}
	}
	return -1
}

func isFrameStartAt(b []byte, i int) bool {
	if i+4 >= len(b) {
		return false
	}
	if b[i] != 'P' || b[i+1] != 'C' {
		return false
	}
	if b[i+2] < '0' || b[i+2] > '9' || b[i+3] < '0' || b[i+3] > '9' {
		return false
	}
	return b[i+4] == '^'
}
