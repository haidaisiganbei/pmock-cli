package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/haidaisiganbei/pmock-cli/internal/auth"
)

const testAK = "AKID"
const testSK = "SECRET"

func tokenOK(got string) bool {
	for _, delta := range []time.Duration{0, -time.Minute, time.Minute} {
		_, want := auth.Sign(testAK, testSK, time.Now().Add(delta))
		if got == want {
			return true
		}
	}
	return false
}

func TestDoSignsAndDecodes(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotKey = r.Header.Get("pmock-access-key")
		if !tokenOK(r.Header.Get("pmock-access-token")) {
			t.Errorf("bad signature %q", r.Header.Get("pmock-access-token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"traceId":"t1","code":0,"msg":"success","data":{"ok":true},"status":"finish"}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/api", testAK, testSK, 5*time.Second)
	env, err := c.Do(context.Background(), Request{
		Method: "GET",
		Path:   "/open/api/phone/list",
		Query:  url.Values{"tagIds": {"1", "2"}, "pageSize": {"5"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 || env.TraceID != "t1" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if gotPath != "/api/open/api/phone/list" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != testAK {
		t.Errorf("access key = %q", gotKey)
	}
	if gotQuery != "pageSize=5&tagIds=1&tagIds=2" {
		t.Errorf("query = %q", gotQuery)
	}
}

func TestBusinessError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"traceId":"t2","code":10123,"msg":"无效的开发者编号","data":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testAK, testSK, time.Second)
	env, err := c.Do(context.Background(), Request{Method: "GET", Path: "/open/api/product/list"})
	if env == nil || env.Code != 10123 {
		t.Fatalf("envelope = %+v", env)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if !IsAuthError(err) {
		t.Error("10123 should be classified as an auth error")
	}
}

func TestRetryOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"traceId":"t3","code":0,"msg":"ok","data":{}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testAK, testSK, 5*time.Second)
	c.MaxRetries = 1
	if _, err := c.Do(context.Background(), Request{Method: "GET", Path: "/open/api/product/list"}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestNoRetryOnPost500(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := New(srv.URL, testAK, testSK, 5*time.Second)
	c.MaxRetries = 2
	_, err := c.Do(context.Background(), Request{Method: "POST", Path: "/open/api/phone/create", Body: map[string]any{"a": 1}})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 500 {
		t.Fatalf("err = %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("POST 500 retried %d times, want 1 call", got)
	}
}

func TestHTTP4xxWithEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"traceId":"t4","code":10123,"msg":"forbidden"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testAK, testSK, time.Second)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/open/api/product/list"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 10123 {
		t.Fatalf("err = %v, want APIError 10123", err)
	}
}

func TestBuildURL(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://user.pmock.com/api", "/open/api/phone/list", "https://user.pmock.com/api/open/api/phone/list"},
		{"https://user.pmock.com/api/", "/open/api/phone/list", "https://user.pmock.com/api/open/api/phone/list"},
		{"https://user.pmock.com/api/open/api", "/open/api/phone/list", "https://user.pmock.com/api/phone/list"},
	}
	for _, tc := range cases {
		if got := BuildURL(tc.base, tc.path, nil); got != tc.want {
			t.Errorf("BuildURL(%q,%q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

func TestHTTP500WithEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"traceId":"t5","code":10123,"msg":"invalid developer key"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, testAK, testSK, time.Second)
	_, err := c.Do(context.Background(), Request{Method: "GET", Path: "/open/api/product/list"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 10123 {
		t.Fatalf("err = %v, want APIError 10123", err)
	}
}
