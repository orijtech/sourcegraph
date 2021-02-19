package backend

import (
	"context"
	"strings"

	"github.com/google/zoekt/rpc"

	"github.com/google/zoekt"
	"github.com/google/zoekt/query"
	zoektstream "github.com/google/zoekt/stream"

	"github.com/sourcegraph/sourcegraph/internal/httpcli"
)

// ZoektStreamFunc is a convenience function to create a stream receiver from a
// function.
type ZoektStreamFunc func(*zoekt.SearchResult)

func (f ZoektStreamFunc) Send(event *zoekt.SearchResult) {
	f(event)
}

// Streamer is an interface which calls c.Send(result *zoekt.SearchResults) as
// results are found.
type Streamer interface {
	StreamSearch(ctx context.Context, q query.Q, opts *zoekt.SearchOptions, c zoektstream.Streamer) error
}

// StreamSearcher is the interface that groups batch zoekt methods and stream
// search.
type StreamSearcher interface {
	zoekt.Searcher
	Streamer
}

// StreamSearchEvent has fields optionally set representing events that happen
// during a search.
//
// This is a Sourcegraph extension.
type StreamSearchEvent struct {
	// SearchResult is non-nil if this event is a search result. These should be
	// combined with previous and later SearchResults.
	SearchResult *zoekt.SearchResult
}

// StreamSearchAdapter adapts a zoekt.Searcher to conform to the StreamSearch
// interface by calling zoekt.Searcher.Search.
type StreamSearchAdapter struct {
	zoekt.Searcher
}

func (s *StreamSearchAdapter) StreamSearch(ctx context.Context, q query.Q, opts *zoekt.SearchOptions, c zoektstream.Streamer) error {
	sr, err := s.Search(ctx, q, opts)
	if err != nil {
		return err
	}
	c.Send(sr)
	return nil
}

func (s *StreamSearchAdapter) String() string {
	return "streamSearchAdapter{" + s.Searcher.String() + "}"
}

func NewZoektStream(address string) StreamSearcher {
	cli, err := httpcli.NewExternalHTTPClientFactory().Client()
	if err != nil {
		panic(err)
	}
	// TODO: this should probably go in the client constructor.
	addressWithScheme := address
	if !strings.HasPrefix(addressWithScheme, "http://") {
		addressWithScheme = "http://" + addressWithScheme
	}
	return &zoektStream{
		rpc.Client(address),
		zoektstream.NewClient(addressWithScheme, cli),
	}
}

type zoektStream struct {
	zoekt.Searcher
	Streamer
}
