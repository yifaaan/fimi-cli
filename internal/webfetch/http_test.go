package webfetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetcherFetchExtractsReadableMainTextFromHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Example Article</title></head><body>
			<header>Site Header</header>
			<nav>Docs Pricing Blog</nav>
			<main>
			  <h1>Go Fetch URL</h1>
			  <p>Readable paragraph one.</p>
			  <p>Readable paragraph two.</p>
			</main>
			<footer>Footer Links</footer>
		</body></html>`))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	for _, want := range []string{"Title: Example Article", "URL: " + server.URL, "Go Fetch URL", "Readable paragraph one.", "Readable paragraph two."} {
		if !strings.Contains(got, want) {
			t.Fatalf("Fetch() output %q missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"Site Header", "Docs Pricing Blog", "Footer Links", "<main>"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("Fetch() output %q unexpectedly contains %q", got, unwanted)
		}
	}
}

func TestHTTPFetcherFetchPrefersArticleOverBoilerplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Article Page</title></head><body>
			<div>
			  <a href="#">home</a><a href="#">pricing</a><a href="#">docs</a><a href="#">blog</a>
			</div>
			<article>
			  <h1>Article Title</h1>
			  <p>The real content starts here.</p>
			</article>
		</body></html>`))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	for _, want := range []string{"Title: Article Page", "URL: " + server.URL, "Article Title", "The real content starts here."} {
		if !strings.Contains(got, want) {
			t.Fatalf("Fetch() output = %q, want %q", got, want)
		}
	}
}

func TestHTTPFetcherFetchReturnsPlainTextUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("plain text response"))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "URL: "+server.URL+"\n\nplain text response" {
		t.Fatalf("Fetch() = %q, want %q", got, "URL: "+server.URL+"\n\nplain text response")
	}
}

func TestHTTPFetcherFetchReturnsEmptyMessageForEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "URL: "+server.URL+"\n\nFetched page is empty." {
		t.Fatalf("Fetch() = %q, want %q", got, "URL: "+server.URL+"\n\nFetched page is empty.")
	}
}

func TestHTTPFetcherFetchReturnsFallbackWhenNoReadableContentExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Empty Article</title></head><body><script>ignored()</script><nav>menu</nav></body></html>`))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client()})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	for _, want := range []string{"Title: Empty Article", "URL: " + server.URL, "No readable main text could be extracted from the page."} {
		if !strings.Contains(got, want) {
			t.Fatalf("Fetch() = %q, want to contain %q", got, want)
		}
	}
}

func TestHTTPFetcherFetchPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: http.DefaultClient})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	_, err = fetcher.Fetch(ctx, "https://example.com")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch() error = %v, want wrapped %v", err, context.Canceled)
	}
}

func TestHTTPFetcherFetchRespectsBodySizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("1234567890"))
	}))
	defer server.Close()

	fetcher, err := NewHTTPFetcher(HTTPFetcherConfig{Client: server.Client(), MaxSize: 5})
	if err != nil {
		t.Fatalf("NewHTTPFetcher() error = %v", err)
	}

	got, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got != "URL: "+server.URL+"\n\n12345" {
		t.Fatalf("Fetch() = %q, want %q", got, "URL: "+server.URL+"\n\n12345")
	}
}
