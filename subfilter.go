// Package subfilter a plugin to rewrite response body.
package subfilter

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
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
		buffer:         &bytes.Buffer{},
	}

	s.next.ServeHTTP(rw, r)

	ce := rw.Header().Get("Content-Encoding")
	b := rw.buffer.Bytes()

	if ce != "" && ce != "identity" && ce != contentEncodingGzip {
		if _, err := w.Write(b); err != nil {
			log.Printf("unable to write response: %v", err)
		}

		return
	}

	for _, f := range s.filters {
		b = f.regex.ReplaceAll(b, f.replacement)
	}
	// fmt.Printf("Regexed Page: %v\n", string(b))
	if ce == "gzip" {
		// fmt.Printf("Gzipping regexed page: %s\n", string(b))
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)

		_, err := gz.Write(b)
		if err != nil {
			log.Printf("unable to write gzipped modified response: %v", err)

			return
		}

		if err = gz.Close(); err != nil {
			log.Printf("unable to close gzip writer: %v", err)

			return
		}

		b = buf.Bytes()
	}

	// log.Printf("regexed page Gzipped: %s\n", b)
	if _, err := w.Write(b); err != nil {
		log.Printf("unable to write modified response: %v", err)
	}
}

type responseWriter struct {
	lastModified bool
	wroteHeader  bool
	buffer       *bytes.Buffer

	http.ResponseWriter
}

func (r *responseWriter) WriteHeader(status int) {
	if !r.lastModified {
		r.Header().Del("Last-Modified")
	}

	r.wroteHeader = true
	r.Header().Del("Content-Length")
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseWriter) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}

	if r.Header().Get("Content-Encoding") == "gzip" {
		// fmt.Printf("Received GZIP encoded page: %s\n", b)
		gr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return 0, fmt.Errorf("unable to create gzip reader: %w", err)
		}

		var cleanBytes []byte

		cleanBytes, err = ioutil.ReadAll(gr)
		if err != nil {
			return 0, fmt.Errorf("unable to read gzipped response: %w", err)
		}
		// fmt.Printf("Decoded page: %s\n", cleanBytes)

		var i int

		i, err = r.buffer.Write(cleanBytes)
		if err != nil {
			return i, fmt.Errorf("could not write buffer: %w", err)
		}

		return i, nil
	}

	i, err := r.buffer.Write(b)
	if err != nil {
		return i, fmt.Errorf("could not write buffer: %w", err)
	}

	return i, nil
}

func (r *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("%T is not a http.Hijacker", r.ResponseWriter)
	}

	c, w, err := h.Hijack()
	if err != nil {
		return c, w, fmt.Errorf("hijack error: %w", err)
	}

	return c, w, nil
}

func (r *responseWriter) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
