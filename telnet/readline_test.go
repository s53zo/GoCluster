package telnet

import (
	"bufio"
	"bytes"
	"testing"
)

func TestReadLineBackspaceAndDelete(t *testing.T) {
	t.Run("backspace", func(t *testing.T) {
		input := "AB\bC\n"
		got, echoed := readLineWithEcho(t, input, true)
		if got != "AC" {
			t.Fatalf("expected line %q, got %q", "AC", got)
		}
		expectEcho := "AB" + "\b \b" + "C" + "\r\n"
		if echoed != expectEcho {
			t.Fatalf("expected echo %q, got %q", expectEcho, echoed)
		}
	})

	t.Run("delete", func(t *testing.T) {
		input := "AB" + string([]byte{0x7f}) + "C\n"
		got, echoed := readLineWithEcho(t, input, true)
		if got != "AC" {
			t.Fatalf("expected line %q, got %q", "AC", got)
		}
		expectEcho := "AB" + "\b \b" + "C" + "\r\n"
		if echoed != expectEcho {
			t.Fatalf("expected echo %q, got %q", expectEcho, echoed)
		}
	})
}

func TestReadLineCtrlUErasesLine(t *testing.T) {
	input := "HELLO" + string([]byte{0x15}) + "OK\n"
	got, echoed := readLineWithEcho(t, input, true)
	if got != "OK" {
		t.Fatalf("expected line %q, got %q", "OK", got)
	}
	expectEcho := "HELLO" + stringsRepeat("\b \b", 5) + "OK" + "\r\n"
	if echoed != expectEcho {
		t.Fatalf("expected echo %q, got %q", expectEcho, echoed)
	}
}

func TestReadLineCtrlWErasesWord(t *testing.T) {
	input := "SHOW FILTER" + string([]byte{0x17}) + "DX\n"
	got, echoed := readLineWithEcho(t, input, true)
	if got != "SHOW DX" {
		t.Fatalf("expected line %q, got %q", "SHOW DX", got)
	}
	expectEcho := "SHOW FILTER" + stringsRepeat("\b \b", 6) + "DX" + "\r\n"
	if echoed != expectEcho {
		t.Fatalf("expected echo %q, got %q", expectEcho, echoed)
	}
}

func TestReadLineNoEchoSkipsEraseOutput(t *testing.T) {
	input := "HELLO" + string([]byte{0x08}) + "X\n"
	got, echoed := readLineWithEcho(t, input, false)
	if got != "HELLX" {
		t.Fatalf("expected line %q, got %q", "HELLX", got)
	}
	if echoed != "" {
		t.Fatalf("expected no echo output, got %q", echoed)
	}
}

func readLineWithEcho(t *testing.T, input string, echo bool) (string, string) {
	t.Helper()
	reader := bufio.NewReader(bytes.NewBufferString(input))
	var out bytes.Buffer
	writer := bufio.NewWriter(&out)
	client := &Client{
		reader:    reader,
		writer:    writer,
		echoInput: echo,
	}
	line, err := client.ReadLine(128, "command", true, true, true, true)
	if err != nil {
		t.Fatalf("ReadLine failed: %v", err)
	}
	return line, out.String()
}

func stringsRepeat(s string, count int) string {
	if count <= 0 {
		return ""
	}
	var b bytes.Buffer
	for i := 0; i < count; i++ {
		b.WriteString(s)
	}
	return b.String()
}
