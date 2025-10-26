package middleware

import (
	"compress/gzip"
	"log"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	w           http.ResponseWriter
	gw          *gzip.Writer
	wroteHeader bool
}

func (grw *gzipResponseWriter) Write(p []byte) (int, error) {
	if !grw.wroteHeader {
		grw.WriteHeader(http.StatusOK)
	}
	return grw.gw.Write(p)
}

func (grw *gzipResponseWriter) WriteHeader(statusCode int) {
	if !grw.wroteHeader {
		grw.w.Header().Set("Content-Encoding", "gzip")
		grw.w.WriteHeader(statusCode)
		grw.wroteHeader = true
		return
	}
}

func (grw *gzipResponseWriter) Header() http.Header {
	return grw.w.Header()
}

func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			cmp, err := gzip.NewReader(r.Body)
			if err != nil {
				log.Printf("Failed to create gzip reader: %v", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			r.Body = cmp
			defer cmp.Close()
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			gw := gzip.NewWriter(w)
			grw := &gzipResponseWriter{w: w, wroteHeader: false, gw: gw}
			defer grw.gw.Close()

			log.Printf("Compressed gzip response")
			next.ServeHTTP(grw, r)
			defer grw.gw.Close()
		} else {
			next.ServeHTTP(w, r)
		}
	})
}
