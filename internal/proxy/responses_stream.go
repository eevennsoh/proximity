package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func convertResponseStream(res *http.Response, convert func(io.Reader, io.Writer) error) {
	source := res.Body
	pr, pw := io.Pipe()
	res.Body = pr
	res.ContentLength = -1
	res.Header.Del("Content-Length")
	res.Header.Del("Content-Encoding")
	go func() {
		defer source.Close()
		_ = pw.CloseWithError(convert(source, pw))
	}()
}

func readResponsesEvents(source io.Reader, consume func(jsonObject) (bool, error)) error {
	reader := bufio.NewReader(source)
	var data strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line[len("data:"):], " "))
			if data.String() == "[DONE]" {
				return io.EOF
			}
		case strings.HasPrefix(line, ":"):
			if done, err := consume(nil); done || err != nil {
				return err
			}
		}
		// Gateway responses can also delimit complete JSON events with a single newline.
		if data.Len() > 0 && (line == "" || json.Valid([]byte(data.String()))) {
			event, err := decodeObject(strings.NewReader(data.String()))
			if err != nil {
				return fmt.Errorf("invalid AI Gateway SSE event")
			}
			data.Reset()
			if done, err := consume(event); done || err != nil {
				return err
			}
		}
		if readErr != nil {
			return readErr
		}
	}
}
