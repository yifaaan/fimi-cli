package websearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
		if r.Method != http.MethodGet {
			t.Fatalf("request method = %q, want %q", r.Method, http.MethodGet)
		}
		if got := r.Header.Get("User-Agent"); got != "fimi-test/1.0" {
			t.Fatalf("User-Agent = %q, want %q", got, "fimi-test/1.0")
		}
		if got := r.URL.Query().Get("q"); got != "golang duckduckgo" {
			t.Fatalf("query = %q, want %q", got, "golang duckduckgo")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>
			<div class="result results_links results_links_deep web-result">
			  <div class="links_main links_deep result__body">
			    <h2 class="result__title">
			      <a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fresult&amp;rut=test">DuckDuckGo</a>
			    </h2>
			    <a class="result__snippet" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fresult&amp;rut=test">Search <b>summary</b></a>
			  </div>
			</div>
		</body></html>`))
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
	if results[0].Content != "" {
		t.Fatalf("results[0].Content = %q, want empty", results[0].Content)
	}
}

func TestDuckDuckGoSearcherSearchReturnsEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><p>No results</p></body></html>`))
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

func TestDuckDuckGoSearcherSearchTreatsMalformedHTMLAsEmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html`))
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
		t.Fatalf("Search() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(Search()) = %d, want 0", len(results))
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
