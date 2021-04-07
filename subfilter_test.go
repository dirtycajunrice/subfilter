package subfilter

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// nolint
func TestServeHTTP(t *testing.T) {
	tests := []struct {
		desc            string
		contentEncoding string
		filters         []Filter
		lastModified    bool
		resBody         string
		expResBody      string
		expLastModified bool
	}{
		{
			desc: "should replace foo by bar",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			resBody:    "foo is the new bar",
			expResBody: "bar is the new bar",
		},
		{
			desc: "should replace foo by bar, then by foo",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
				{
					Regex:       "bar",
					Replacement: "foo",
				},
			},
			resBody:    "foo is the new bar",
			expResBody: "foo is the new foo",
		},
		{
			desc: "should not replace anything if content encoding is not gzip, identity, or empty",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			contentEncoding: "br",
			resBody:         "foo is the new bar",
			expResBody:      "foo is the new bar",
		},
		{
			desc: "should unzip, replace foo by bar, then zip",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			contentEncoding: "gzip",
			resBody:         "foo is the new bar",
			expResBody:      "bar is the new bar",
		},
		{
			desc: "should replace foo by bar if content encoding is identity",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			contentEncoding: "identity",
			resBody:         "foo is the new bar",
			expResBody:      "bar is the new bar",
		},
		{
			desc: "should not remove the last modified header",
			filters: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
			},
			contentEncoding: "identity",
			lastModified:    true,
			resBody:         "foo is the new bar",
			expResBody:      "bar is the new bar",
			expLastModified: true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			config := CreateConfig()
			config.LastModified = test.lastModified
			config.Filters = test.filters

			next := func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", test.contentEncoding)
				w.Header().Set("Last-Modified", "Thu, 02 Jun 2016 06:01:08 GMT")
				w.Header().Set("Content-Length", strconv.Itoa(len(test.resBody)))
				w.WriteHeader(http.StatusOK)
				if test.contentEncoding == "gzip" {
					t.Logf("Original body to send: %v", test.resBody)
					b := bytes.Buffer{}
					gw := gzip.NewWriter(&b)
					if _, err := gw.Write([]byte(test.resBody)); err != nil {
						t.Fatal(err)
					}
					if err := gw.Close(); err != nil {
						t.Fatal(err)
					}
					gb := b.Bytes()
					t.Logf("body after gzip: %v", gb)
					if _, err := w.Write(gb); err != nil {
						t.Fatal(err)
					}
					test.resBody = string(gb)
				} else {
					_, _ = fmt.Fprintf(w, test.resBody)
				}
			}

			rewriteBody, err := New(context.Background(), http.HandlerFunc(next), config, "subfilter")
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			rewriteBody.ServeHTTP(recorder, req)

			if _, exists := recorder.Result().Header["Last-Modified"]; exists != test.expLastModified {
				t.Errorf("got last-modified header %v, want %v", exists, test.expLastModified)
			}

			if _, exists := recorder.Result().Header["Content-Length"]; exists {
				t.Error("The Content-Length Header must be deleted")
			}
			if test.contentEncoding == contentEncodingGzip {
				t.Logf("received gzipped page: %v", recorder.Body.String())
				var gr *gzip.Reader
				gr, err = gzip.NewReader(bytes.NewReader(recorder.Body.Bytes()))
				if err != nil {
					t.Errorf("could not create a gzip reader: %v", err)

					return
				}

				var b []byte
				b, err = ioutil.ReadAll(gr)
				if err != nil {
					t.Errorf("unable to read unzipped response: %v", err)

					return
				}

				if err = gr.Close(); err != nil {
					t.Errorf("problem closing gzip test reader: %v", err)

					return
				}

				if !bytes.Equal([]byte(test.expResBody), b) {
					t.Errorf("got unzipped body %q, want %q", b, test.expResBody)
				}

				return
			}
			if !bytes.Equal([]byte(test.expResBody), recorder.Body.Bytes()) {
				t.Errorf("got body %q, want %q", recorder.Body.Bytes(), test.expResBody)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		desc     string
		rewrites []Filter
		expErr   bool
	}{
		{
			desc: "should return no error",
			rewrites: []Filter{
				{
					Regex:       "foo",
					Replacement: "bar",
				},
				{
					Regex:       "bar",
					Replacement: "foo",
				},
			},
			expErr: false,
		},
		{
			desc: "should return an error",
			rewrites: []Filter{
				{
					Regex:       "*",
					Replacement: "bar",
				},
			},
			expErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			config := &Config{
				Filters: test.rewrites,
			}

			_, err := New(context.Background(), nil, config, "rewriteBody")
			if test.expErr && err == nil {
				t.Fatal("expected error on bad regexp format")
			}
		})
	}
}
