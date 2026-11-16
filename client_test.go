package casper

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// testClient returns a client of a server that answers every request with handler.
func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("server URL: %v", err)
	}
	port, err := strconv.Atoi(serverURL.Port())
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	client, err := New("http://"+serverURL.Hostname(), port)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return client
}

func scoredBin(results ...SearchResult) []byte {
	out := make([]byte, 4+len(results)*12)
	binary.LittleEndian.PutUint32(out, uint32(len(results)))
	for i, r := range results {
		off := 4 + i*12
		binary.LittleEndian.PutUint64(out[off:], r.ID)
		binary.LittleEndian.PutUint32(out[off+8:], math.Float32bits(r.Score))
	}
	return out
}

func TestScoresDecodesTwelveByteRecords(t *testing.T) {
	want := []SearchResult{{ID: 1 << 40, Score: 0.5}, {ID: 2, Score: -1.25}}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/collection/demo/scores" || r.URL.Query().Get("output") != "bin" || string(body) != `{"vector":[0.5],"ids":[1099511627776,2]}` {
			t.Errorf("unexpected request %s %s", r.URL, body)
		}
		_, _ = w.Write(scoredBin(want...))
	})
	got, err := client.Scores(context.Background(), "demo", []float32{0.5}, []uint64{1 << 40, 2})
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v (%v)", got, err)
	}
}

func TestSearchRefusesAWrongLength(t *testing.T) {
	for name, body := range map[string][]byte{
		"truncated":    scoredBin(SearchResult{ID: 1})[:10],
		"huge count":   {0xff, 0xff, 0xff, 0xff},
		"8-byte pairs": append(scoredBin()[:0:0], 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0),
	} {
		client := testClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) })
		_, err := client.Search(context.Background(), "demo", []float32{0.5}, 1)
		var e *Error
		if !errors.As(err, &e) || e.Kind != ErrorKindInvalidResponse {
			t.Fatalf("%s: got %v", name, err)
		}
	}
}

func TestGetVectorTellsAMissingVectorFromAMissingCollection(t *testing.T) {
	missingVector := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "Vector not found", "id": 7}`))
	})
	v, err := missingVector.GetVector(context.Background(), "demo", 7)
	if v != nil || err != nil {
		t.Fatalf("missing vector: got %+v, %v", v, err)
	}

	missingCollection := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`Resource not found: collection 'demo' not found`))
	})
	_, err = missingCollection.GetVector(context.Background(), "demo", 7)
	var e *Error
	if !errors.As(err, &e) || e.Kind != ErrorKindNotFound {
		t.Fatalf("missing collection: got %v", err)
	}
}

func TestCreateCollectionReturnsTheID(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "c8c3f16f-0f4e-45af-a4eb-d4969ec74fef"}`))
	})
	created, err := client.CreateCollection(context.Background(), "demo", CreateCollectionRequest{Dim: 4, MaxSize: 10})
	if err != nil || created.ID != "c8c3f16f-0f4e-45af-a4eb-d4969ec74fef" {
		t.Fatalf("got %+v (%v)", created, err)
	}
}

func TestBatchUpdateSendsBothLists(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"upserts":[{"id":10,"vector":[0.5]}],"delete":[]}` {
			t.Errorf("unexpected body %s", body)
		}
	})
	err := client.BatchUpdate(context.Background(), "demo", BatchUpdateRequest{Upserts: []UpsertOperation{{ID: 10, Vector: []float32{0.5}}}})
	if err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestAFailedConnectionIsATransportError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	serverURL, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(serverURL.Port())
	server.Close()

	client, err := New("http://"+serverURL.Hostname(), port)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	_, err = client.ListCollections(context.Background())
	var casperErr *Error
	if !errors.As(err, &casperErr) || casperErr.Kind != ErrorKindTransport || casperErr.Cause == nil {
		t.Fatalf("got %v, want a transport error", err)
	}
}

func TestACollectionNameIsOnePathSegment(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/collection/a%2Fb%3Fc/index/start" {
			t.Errorf("path %s", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusAccepted)
	})
	if err := client.StartIndexing(context.Background(), "a/b?c"); err != nil {
		t.Fatal(err)
	}
}
