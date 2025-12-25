package peer

import (
	"net"
	"time"
)

type lineReader struct {
	conn     net.Conn
	readFn   func([]byte) (int, error)
	parser   *telnetParser
	buf      []byte
	replyFn  func([]byte)
	maxLine  int
	pc92Max  int
	dropping bool
	readBuf  []byte
}

// errLineTooLong carries a preview and length when a frame exceeds maxLine.
type errLineTooLong struct {
	preview string
	length  int
}

func (e errLineTooLong) Error() string {
	return "line too long"
}

func newLineReader(conn net.Conn, maxLine int, pc92Max int, replyFn func([]byte)) *lineReader {
	return newLineReaderWithTransport(conn, maxLine, pc92Max, conn.Read, &telnetParser{}, replyFn)
}

// newLineReaderWithTransport allows callers to supply a read function that already
// strips IAC sequences (e.g., external telnet library). When parser is nil, data
// is treated as already-clean payload bytes.
func newLineReaderWithTransport(conn net.Conn, maxLine int, pc92Max int, readFn func([]byte) (int, error), parser *telnetParser, replyFn func([]byte)) *lineReader {
	if readFn == nil {
		readFn = conn.Read
	}
	return &lineReader{
		conn:    conn,
		readFn:  readFn,
		parser:  parser,
		buf:     make([]byte, 0, maxLine),
		replyFn: replyFn,
		maxLine: maxLine,
		pc92Max: pc92Max,
		readBuf: make([]byte, 4096),
	}
}

func (r *lineReader) ReadLine(deadline time.Time) (string, error) {
	if err := r.conn.SetReadDeadline(deadline); err != nil {
		return "", err
	}
	for {
		if !r.dropping {
			line, err, ready := r.tryReadLine()
			if ready {
				return line, err
			}
		}
		n, err := r.readFn(r.readBuf)
		if n > 0 {
			data := r.readBuf[:n]
			if r.parser != nil {
				out, replies := r.parser.Feed(data)
				if len(replies) > 0 && r.replyFn != nil {
					for _, rep := range replies {
						r.replyFn(rep)
					}
				}
				data = out
			}
			if r.dropping {
				if idx, size := bytesIndexTerminator(data); idx >= 0 {
					r.dropping = false
					r.buf = append(r.buf[:0], data[idx+size:]...)
				}
			} else {
				r.buf = append(r.buf, data...)
			}
		}
		if err != nil {
			return "", err
		}
	}
}

func (r *lineReader) tryReadLine() (string, error, bool) {
	for {
		r.buf = trimLeadingTerminators(r.buf)
		if len(r.buf) == 0 {
			return "", nil, false
		}
		// Prefer explicit terminators (~, CRLF, CR, LF) when present.
		if idx, size := bytesIndexTerminator(r.buf); idx >= 0 {
			if r.pc92Max > 0 && idx > r.pc92Max && frameTypeFromBuffer(r.buf) == "PC92" {
				preview := string(r.buf[:idx])
				r.buf = append([]byte{}, r.buf[idx+size:]...)
				return "", errLineTooLong{preview: preview, length: idx}, true
			}
			line := string(trimLine(r.buf[:idx]))
			r.buf = append([]byte{}, r.buf[idx+size:]...)
			return line, nil, true
		}
		// Resync: discard leading noise until a valid PCxx^ frame start that follows a terminator.
		// This avoids splitting on "^PC" sequences that might appear inside payload fields.
		if start := bytesIndexFrameStart(r.buf); start > 0 {
			r.buf = r.buf[start:]
			continue
		}
		if r.pc92Max > 0 && len(r.buf) > r.pc92Max && frameTypeFromBuffer(r.buf) == "PC92" {
			preview := string(r.buf)
			r.buf = r.buf[:0]
			r.dropping = true
			return "", errLineTooLong{preview: preview, length: len(preview)}, true
		}
		if len(r.buf) > r.maxLine && r.maxLine > 0 {
			// Drop the current buffer to avoid unbounded growth; caller can choose to continue.
			preview := string(r.buf)
			r.buf = r.buf[:0]
			return "", errLineTooLong{preview: preview, length: len(preview)}, true
		}
		return "", nil, false
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

func frameTypeFromBuffer(b []byte) string {
	if !isFrameStartAt(b, 0) {
		return ""
	}
	return string(b[:4])
}
