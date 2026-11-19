package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

type runOutput struct {
	stdout          string
	stderr          string
	stdoutTruncated bool
	stderrTruncated bool
}

type boundedCapture struct {
	mu        sync.Mutex
	limit     int
	data      []byte
	truncated bool
}

func newBoundedCapture(limit int) *boundedCapture {
	return &boundedCapture{limit: limit}
}

func (c *boundedCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limit <= 0 {
		return len(p), nil
	}
	if len(p) >= c.limit {
		c.data = append(c.data[:0], p[len(p)-c.limit:]...)
		c.truncated = true
		return len(p), nil
	}
	if excess := len(c.data) + len(p) - c.limit; excess > 0 {
		copy(c.data, c.data[excess:])
		c.data = c.data[:len(c.data)-excess]
		c.truncated = true
	}
	c.data = append(c.data, p...)
	return len(p), nil
}

func (c *boundedCapture) snapshot() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.ToValidUTF8(string(append([]byte(nil), c.data...)), "�"), c.truncated
}

func captureWriter(original io.Writer, capture io.Writer) io.Writer {
	if original == nil {
		return capture
	}
	return io.MultiWriter(original, capture)
}

func newRunOutputCapture(limit int) (*boundedCapture, *boundedCapture) {
	streamLimit := limit / 2
	return newBoundedCapture(streamLimit), newBoundedCapture(limit - streamLimit)
}

func snapshotRunOutput(stdout, stderr *boundedCapture) runOutput {
	out, outTruncated := stdout.snapshot()
	err, errTruncated := stderr.snapshot()
	return runOutput{stdout: out, stderr: err, stdoutTruncated: outTruncated, stderrTruncated: errTruncated}
}

func writeRunFailureLog(out io.Writer, output runOutput) error {
	if _, err := fmt.Fprintln(out, "\n--- command failure log (captured tail; terminal controls escaped) ---"); err != nil {
		return fmt.Errorf("write command failure log header: %w", err)
	}
	if output.stdout == "" && output.stderr == "" {
		if _, err := fmt.Fprintln(out, "(no output captured)"); err != nil {
			return fmt.Errorf("write empty command failure log: %w", err)
		}
	}
	for _, stream := range []struct {
		name      string
		data      string
		truncated bool
	}{{"stdout", output.stdout, output.stdoutTruncated}, {"stderr", output.stderr, output.stderrTruncated}} {
		if stream.data == "" && !stream.truncated {
			continue
		}
		marker := ""
		if stream.truncated {
			marker = " [earlier bytes omitted]"
		}
		if _, err := fmt.Fprintf(out, "%s%s: %s\n", stream.name, marker, strconv.QuoteToASCII(stream.data)); err != nil {
			return fmt.Errorf("write command failure %s: %w", stream.name, err)
		}
	}
	if _, err := fmt.Fprintln(out, "--- end command failure log ---"); err != nil {
		return fmt.Errorf("write command failure log footer: %w", err)
	}
	return nil
}
