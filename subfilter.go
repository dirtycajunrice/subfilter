// Package subfilter a plugin to rewrite response body.
package subfilter

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
)

const contentEncodingGzip = "gzip"

// Filter holds one Filter definition.
type Filter struct {
	Regex       string `json:"regex,omitempty"`
	Replacement string `json:"replacement,omitempty"`
}

// Config holds the plugin configuration.
type Config struct {
	LastModified bool     `json:"lastModified,omitempty"`
	Filters      []Filter `json:"filters,omitempty"`
}

// CreateConfig creates and initializes the plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

type filter struct {
	regex       *regexp.Regexp
	replacement []byte
}

type subfilter struct {
	name         string
	next         http.Handler
	filters      []filter
	lastModified bool
}

// New creates and returns a new rewrite body plugin instance.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	filters := make([]filter, 0)

	for _, f := range config.Filters {
		regex, err := regexp.Compile(f.Regex)
		if err != nil {
			log.Printf("error compiling regex %q: %v", f.Regex, err)

			continue
		}

		newFilter := filter{
			regex:       regex,
			replacement: []byte(f.Replacement),
		}

		filters = append(filters, newFilter)
	}

	if len(filters) == 0 {
		return nil, errors.New("no valid filters. disabling")
	}

	sf := &subfilter{
		name:         name,
		next:         next,
		filters:      filters,
		lastModified: config.LastModified,
	}

	return sf, nil
}

func (s *subfilter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := &responseWriter{
		lastModified:   s.lastModified,
		ResponseWriter: w,
	}

	s.next.ServeHTTP(rw, r)
	ce := rw.Header().Get("Content-Encoding")

	var b []byte

	switch ce {
	case "", "identity":
		b = rw.buffer.Bytes()
	case contentEncodingGzip:
		gr, err := gzip.NewReader(&rw.buffer)
		if err != nil {
			log.Printf("unable to create gzip reader: %v", err)

			return
		}

		b, err = ioutil.ReadAll(gr)
		if err != nil {
			log.Printf("unable to read gzipped response: %v", err)

			return
		}

		err = gr.Close()
		if err != nil {
			log.Printf("unable to close gzip reader: %v", err)

			return
		}

		rw.encoding = contentEncodingGzip
	default:
		if _, err := io.Copy(w, &rw.buffer); err != nil {
			log.Printf("unable to write response: %v", err)
		}

		return
	}

	for _, f := range s.filters {
		b = f.regex.ReplaceAll(b, f.replacement)
	}

	if _, err := w.Write(b); err != nil {
		log.Printf("unable to write modified response: %v", err)
	}
}

type responseWriter struct {
	buffer       bytes.Buffer
	lastModified bool
	wroteHeader  bool
	encoding     string
	http.ResponseWriter
}

func (r *responseWriter) WriteHeader(statusCode int) {
	if !r.lastModified {
		r.ResponseWriter.Header().Del("Last-Modified")
	}

	r.wroteHeader = true
	r.ResponseWriter.Header().Del("Content-Length")
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	return r.buffer.Write(p)
}

func (r *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("%T is not a http.Hijacker", r.ResponseWriter)
	}

	return h.Hijack()
}

func (r *responseWriter) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
