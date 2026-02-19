package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipTestHandler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	w.Header().Set("Content-Type", contentType)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("received: " + string(body)))
}

func TestGzipMiddleware(t *testing.T) {
	type want struct {
		statusCode      int
		contentEncoding string
		contentType     string
		bodyContains    string
	}

	tests := []struct {
		name        string
		requestBody string
		headers     map[string]string
		want        want
	}{
		{
			name:        "client accepts gzip, text/html",
			requestBody: "test request",
			headers: map[string]string{
				"Accept-Encoding": "gzip",
				"Content-Type":    "text/html",
			},
			want: want{
				statusCode:      http.StatusOK,
				contentEncoding: "gzip",
				contentType:     "text/html",
				bodyContains:    "received: test request",
			},
		},
		{
			name:        "client accepts gzip, application/json",
			requestBody: `{"test":"data"}`,
			headers: map[string]string{
				"Accept-Encoding": "gzip",
				"Content-Type":    "application/json",
			},
			want: want{
				statusCode:      http.StatusOK,
				contentEncoding: "gzip",
				contentType:     "application/json",
				bodyContains:    `received: {"test":"data"}`,
			},
		},
		{
			name:        "client does not accept gzip",
			requestBody: "plain request",
			headers: map[string]string{
				"Accept-Encoding": "",
				"Content-Type":    "text/html",
			},
			want: want{
				statusCode:      http.StatusOK,
				contentEncoding: "",
				contentType:     "text/html",
				bodyContains:    "received: plain request",
			},
		},
		{
			name:        "compressed request body",
			requestBody: "compressed request",
			headers: map[string]string{
				"Content-Encoding": "gzip",
				"Accept-Encoding":  "gzip",
				"Content-Type":     "text/html",
			},
			want: want{
				statusCode:      http.StatusOK,
				contentEncoding: "gzip",
				contentType:     "text/html",
				bodyContains:    "received: compressed request",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestBody io.Reader
			if strings.Contains(tt.headers["Content-Encoding"], "gzip") {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				_, err := gz.Write([]byte(tt.requestBody))
				if err != nil {
					t.Fatalf("write gzip: %v", err)
				}
				if err := gz.Close(); err != nil {
					t.Fatalf("close gzip: %v", err)
				}
				requestBody = &buf
			} else {
				requestBody = strings.NewReader(tt.requestBody)
			}

			req := httptest.NewRequest(http.MethodPost, "/test", requestBody)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()

			h := GzipMiddleware(http.HandlerFunc(gzipTestHandler))
			h.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.want.statusCode {
				t.Fatalf("status: got %d want %d", res.StatusCode, tt.want.statusCode)
			}

			if ct := res.Header.Get("Content-Type"); ct != tt.want.contentType {
				t.Fatalf("content-type: got %q want %q", ct, tt.want.contentType)
			}

			if ce := res.Header.Get("Content-Encoding"); ce != tt.want.contentEncoding {
				t.Fatalf("content-encoding: got %q want %q", ce, tt.want.contentEncoding)
			}

			var body []byte
			var err error
			if res.Header.Get("Content-Encoding") == "gzip" {
				gr, err := gzip.NewReader(res.Body)
				if err != nil {
					t.Fatalf("new gzip reader: %v", err)
				}
				defer gr.Close()
				body, err = io.ReadAll(gr)
			} else {
				body, err = io.ReadAll(res.Body)
			}
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			if !strings.Contains(string(body), tt.want.bodyContains) {
				t.Fatalf("body %q does not contain %q", string(body), tt.want.bodyContains)
			}
		})
	}
}

