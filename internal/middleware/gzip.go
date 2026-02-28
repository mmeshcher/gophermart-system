package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// GzipMiddleware добавляет поддержку gzip-сжатия запросов и ответов.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gzReader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Invalid gzip body", http.StatusBadRequest)
				return
			}
			defer gzReader.Close()
			r.Body = gzReader
		}

		acceptsGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")

		if !acceptsGzip {
			next.ServeHTTP(w, r)
			return
		}

		contentType := r.Header.Get("Content-Type")
		shouldCompress := strings.Contains(contentType, "application/json") ||
			strings.Contains(contentType, "text/html")

		if !shouldCompress {
			next.ServeHTTP(w, r)
			return
		}

		gzWriter := gzip.NewWriter(w)
		defer gzWriter.Close()

		w.Header().Set("Content-Encoding", "gzip")

		grw := &gzipResponseWriter{
			ResponseWriter: w,
			Writer:         gzWriter,
		}

		next.ServeHTTP(grw, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (grw *gzipResponseWriter) Write(b []byte) (int, error) {
	return grw.Writer.Write(b)
}
