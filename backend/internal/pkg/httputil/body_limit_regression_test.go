package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/klauspost/compress/zstd"
)

type countingLimitReader struct{ count int }

func (r *countingLimitReader) Read(p []byte) (int, error) {
	clear(p)
	r.count += len(p)
	return len(p), nil
}

func TestReadRequestBodyLimit_BoundariesAcrossEncodings(t *testing.T) {
	for _, encoding := range []string{"", "identity", "preread", "gzip", "zstd", "deflate"} {
		for _, length := range []int{63, 64, 65} {
			t.Run(encoding+string(rune(length)), func(t *testing.T) {
				body := bytes.Repeat([]byte{'a'}, length)
				wire := body
				var buf bytes.Buffer
				switch encoding {
				case "gzip":
					w := gzip.NewWriter(&buf)
					_, _ = w.Write(body)
					_ = w.Close()
					wire = buf.Bytes()
				case "deflate":
					w := zlib.NewWriter(&buf)
					_, _ = w.Write(body)
					_ = w.Close()
					wire = buf.Bytes()
				case "zstd":
					w, err := zstd.NewWriter(nil)
					if err != nil {
						t.Fatal(err)
					}
					wire = w.EncodeAll(body, nil)
					_ = w.Close()
				}
				req := newRequestWithBody(t, wire, encoding)
				if encoding == "preread" {
					req.Body = NewPrereadBody(body)
					req.Header.Del("Content-Encoding")
				}
				// Chunked requests must obey the same limit as known-length bodies.
				req.ContentLength = -1
				got, err := ReadRequestBodyWithPreallocLimit(req, 64)
				if length > 64 {
					var maxErr *http.MaxBytesError
					if !errors.As(err, &maxErr) || maxErr.Limit != 64 {
						t.Fatalf("want typed limit error, got %v", err)
					}
					return
				}
				if err != nil || !bytes.Equal(got, body) {
					t.Fatalf("body changed or rejected: %v", err)
				}
			})
		}
	}
}

func TestReadRequestBodyLimit_StopsUnboundedWireReader(t *testing.T) {
	for _, encoding := range []string{"", "gzip"} {
		t.Run(encoding, func(t *testing.T) {
			r := &countingLimitReader{}
			req := newRequestWithBody(t, nil, encoding)
			req.Body = io.NopCloser(r)
			req.ContentLength = -1
			_, err := ReadRequestBodyWithPreallocLimit(req, 64)
			var maxErr *http.MaxBytesError
			if !errors.As(err, &maxErr) {
				t.Fatalf("want limit error, got %v", err)
			}
			bound := 65
			if encoding != "" {
				bound += 64/16 + 1024
			}
			if r.count != bound {
				t.Fatalf("read %d bytes, expected bounded read %d", r.count, bound)
			}
		})
	}
}
