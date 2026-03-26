package websearch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNewDuckDuckGoSearcherRequiresBaseURL(t *testing.T) {
	_, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{UserAgent: "fimi-test/1.0"})
	if err == nil || err.Error() != "duckduckgo base url is required" {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v, want duckduckgo base url is required", err)
	}
}

func TestNewDuckDuckGoSearcherRequiresUserAgent(t *testing.T) {
	_, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{BaseURL: "https://duckduckgo.example/html/"})
	if err == nil || err.Error() != "duckduckgo user agent is required" {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v, want duckduckgo user agent is required", err)
	}
}

func TestDuckDuckGoSearcherSearchBuildsRequestAndParsesResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("request method = %q, want %q", r.Method, http.MethodPost)
		}
		if got := r.Header.Get("User-Agent"); got != "fimi-test/1.0" {
			t.Fatalf("User-Agent = %q, want %q", got, "fimi-test/1.0")
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q, want %q", got, "application/x-www-form-urlencoded")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("ParseQuery() error = %v", err)
		}
		if got := values.Get("q"); got != "golang duckduckgo" {
			t.Fatalf("query = %q, want %q", got, "golang duckduckgo")
		}
		if got := values.Get("limit"); got != "2" {
			t.Fatalf("limit = %q, want %q", got, "2")
		}
		if got := values.Get("include_content"); got != "true" {
			t.Fatalf("include_content = %q, want %q", got, "true")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"DuckDuckGo","url":"https://example.com/result","snippet":"Search summary","content":"Full page text"}]}`))
	}))
	defer server.Close()

	searcher, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{
		BaseURL:   server.URL + defaultDuckDuckGoPath,
		UserAgent: "fimi-test/1.0",
		Client:    server.Client(),
	})
	if err != nil {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v", err)
	}

	results, err := searcher.Search(context.Background(), "golang duckduckgo", 2, true)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(Search()) = %d, want 1", len(results))
	}
	if results[0].Title != "DuckDuckGo" {
		t.Fatalf("results[0].Title = %q, want %q", results[0].Title, "DuckDuckGo")
	}
	if results[0].URL != "https://example.com/result" {
		t.Fatalf("results[0].URL = %q, want %q", results[0].URL, "https://example.com/result")
	}
	if results[0].Snippet != "Search summary" {
		t.Fatalf("results[0].Snippet = %q, want %q", results[0].Snippet, "Search summary")
	}
	if results[0].Content != "Full page text" {
		t.Fatalf("results[0].Content = %q, want %q", results[0].Content, "Full page text")
	}
}

func TestDuckDuckGoSearcherSearchReturnsEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	searcher, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{
		BaseURL:   server.URL + defaultDuckDuckGoPath,
		UserAgent: "fimi-test/1.0",
		Client:    server.Client(),
	})
	if err != nil {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v", err)
	}

	results, err := searcher.Search(context.Background(), "golang duckduckgo", 2, false)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(Search()) = %d, want 0", len(results))
	}
}

func TestDuckDuckGoSearcherSearchRejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":`))
	}))
	defer server.Close()

	searcher, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{
		BaseURL:   server.URL + defaultDuckDuckGoPath,
		UserAgent: "fimi-test/1.0",
		Client:    server.Client(),
	})
	if err != nil {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v", err)
	}

	_, err = searcher.Search(context.Background(), "golang duckduckgo", 2, false)
	if err == nil || err.Error() == "" {
		t.Fatalf("Search() error = %v, want non-nil", err)
	}
}

func TestDuckDuckGoSearcherSearchPropagatesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	searcher, err := NewDuckDuckGoSearcher(DuckDuckGoConfig{
		BaseURL:   "https://duckduckgo.example/html/",
		UserAgent: "fimi-test/1.0",
		Client:    http.DefaultClient,
	})
	if err != nil {
		t.Fatalf("NewDuckDuckGoSearcher() error = %v", err)
	}

	_, err = searcher.Search(ctx, "golang duckduckgo", 2, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search() error = %v, want wrapped %v", err, context.Canceled)
	}
}
