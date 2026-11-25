package app

import (
	"fmt"
	"io"
	"testing"
	"time"
)

func TestLoggingContinuesWhileProxyStateIsLocked(t *testing.T) {
	a := &App{}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	doneReading := make(chan struct{})
	go func() { defer close(doneReading); a.pipeLogs(reader, func(string) {}) }()

	a.mu.Lock()
	firstWritten := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		if _, err := fmt.Fprintln(writer, "first"); err != nil {
			written <- err
			return
		}
		close(firstWritten)
		_, err := fmt.Fprintln(writer, "second")
		written <- err
	}()
	select {
	case <-firstWritten:
	case <-time.After(3 * time.Second):
		a.mu.Unlock()
		t.Fatal("first log write did not complete")
	}
	select {
	case err := <-written:
		a.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		a.mu.Unlock()
		t.Fatal("log reader blocked while proxy state was locked")
	}
	writer.Close()
	select {
	case <-doneReading:
	case <-time.After(3 * time.Second):
		t.Fatal("log reader did not stop")
	}
	if got := a.GetLogs(); got != "first\nsecond\n" {
		t.Fatalf("logs = %q", got)
	}
}
