# Casper Go client

Client library for the [Casper Vector Database](https://github.com/casper-vdb/Casper) written in Go.

### Installation

Add the dependency via `go get`:

```bash
go get github.com/casper-vdb/go-client@latest
```

Import the package:

```go
import casper "github.com/casper-vdb/go-client"
```

### Quick start

The example below is a shorter version of `examples/main.go` and demonstrates:
creating a collection with its index schema, writing vectors, starting
indexing and waiting for it, search, scores, reading a vector and deleting the
collection.

```go
package main

import (
	"context"
	"log"
	"math"
	"math/rand/v2"
	"time"

	casper "github.com/casper-vdb/go-client"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// APIKey is needed only when the server has API keys configured.
	client, err := casper.NewWithOptions("http://127.0.0.1", 7222, casper.Options{
		APIKey: "my-api-key",
	})
	if err != nil {
		log.Fatalf("failed to create Casper client: %v", err)
	}

	if err := client.Health(ctx); err != nil {
		log.Fatalf("health check failed: %v", err)
	}

	const collection = "example_collection"
	const dim = 128

	// Create the collection with the schema of its index. Storage is
	// optional; every setting left out keeps its default.
	norm := true
	created, err := client.CreateCollection(ctx, collection, casper.CreateCollectionRequest{
		Dim:     dim,
		MaxSize: 10_000,
		Index: casper.IndexSpec{
			HNSW: casper.HNSW{
				Metric:         "inner-product",
				Quantization:   "f32",
				M:              16,
				M0:             32,
				EfConstruction: 200,
			},
			Normalization: &norm,
		},
	})
	if err != nil {
		log.Fatalf("create collection failed: %v", err)
	}
	log.Printf("collection id: %s", created.ID)

	// Write: upserts and deletes in one batch.
	var upserts []casper.UpsertOperation
	for id := uint64(1); id <= 10; id++ {
		upserts = append(upserts, casper.UpsertOperation{ID: id, Vector: genVec(dim)})
	}
	if err := client.BatchUpdate(ctx, collection, casper.BatchUpdateRequest{
		Upserts: upserts,
		Delete:  []uint64{},
	}); err != nil {
		log.Fatalf("batch update failed: %v", err)
	}

	// Start indexing (the build runs asynchronously on the server) and wait
	// until index.state is ready. Back at not_started means the build failed.
	if err := client.StartIndexing(ctx, collection); err != nil {
		log.Fatalf("start indexing failed: %v", err)
	}
	for ready := false; !ready; {
		info, err := client.GetCollection(ctx, collection)
		if err != nil {
			log.Fatalf("get collection failed: %v", err)
		}
		switch info.Index.State {
		case casper.IndexStateReady:
			ready = true
			continue
		case casper.IndexStateNotStarted:
			log.Fatalf("index build failed")
		}
		select {
		case <-ctx.Done():
			log.Fatalf("index not ready: %v", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}

	// Search
	query := genVec(dim)
	results, err := client.Search(ctx, collection, query, 5)
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}
	for i, r := range results {
		log.Printf("  %d. id=%d score=%f", i+1, r.ID, r.Score)
	}

	// Scores of the same query against chosen ids
	scores, err := client.Scores(ctx, collection, query, []uint64{1, 2, 3})
	if err != nil {
		log.Fatalf("scores failed: %v", err)
	}
	for _, r := range scores {
		log.Printf("  id=%d score=%f", r.ID, r.Score)
	}

	// Read a vector back: nil means the id is not in the index.
	v, err := client.GetVector(ctx, collection, 1)
	if err != nil {
		log.Fatalf("get vector failed: %v", err)
	}
	if v != nil {
		log.Printf("vector 1 has %d components", len(v.Vector.F32))
	}

	// Cleanup (the index goes with its collection)
	_ = client.DeleteCollection(ctx, collection)
}

func genVec(dim int) []float32 {
	var sum float32 = 0
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = 2*rand.Float32() - 1
		sum += vec[i] * vec[i]
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}
```

### Features

The Go client covers the current Casper HTTP API (server 0.2.0):

- **Collections**: `CreateCollection` (with the index schema and policy; returns the collection's id), `DeleteCollection`, `ListCollections` (names only), `GetCollection` (details, WAL counters, snapshot counters, the index's state, schema and policy).
- **Vectors**: `BatchUpdate` (the one write: upserts and deletes in one batch), `GetVector` (the vector as the index stores it: `F32`, or `I8` quantized components for an i8 index; nil when the id is absent). Ids are `uint64`.
- **Indexes**: `StartIndexing` (async build — server returns 202; poll `GetCollection(..).Index.State`), `UpdateIndexPolicy` (change the snapshot/compaction policy, before indexing starts as well), `TriggerSnapshot`, `TriggerCompaction`. An index lives until its collection is deleted.
- **Search**: `Search`, `SearchWithEf` (overrides the HNSW `ef_search` parameter), `Scores` (the query's score against chosen ids). The binary response format is decoded automatically.
- **Cluster**: `GetCluster` (role, Raft state, membership with published addresses), `RemovePeer` (send to the leader), `SetTransferLimit` (in memory until restart).
- **Configuration**: `GetConfig` (the node's startup configuration, grouped as in the config file).
- **Metrics**: `GetMetrics` (Prometheus text format, returned as a string).
- **Health**: `Health` (`/health`, liveness) and `Ready` (`/readyz`, readiness; optionally also requires the node to count for quorum).
- **Auth**: `Options.APIKey` is sent as the `api-key` header.
- **Errors**: every failure is a `*casper.Error` with a `Kind` per status code; a request that got no answer (connection refused, timeout, cancelled context) is `ErrorKindTransport`. A mutation sent to a cluster node that is not the leader fails with `ErrorKindNotLeader` and `LeaderAddr` naming the leader; `429` and `503` carry the server's `Retry-After` in `RetryAfter`. The client never retries on its own.

## License

Licensed under the [Apache License Version 2.0](LICENSE).
