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

The example below mirrors `examples/main.go` and demonstrates: collection
management, batch upsert, HNSW index creation, search, reading and deleting
vectors.

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

	client, err := casper.New("http://127.0.0.1", 7222)
	if err != nil {
		log.Fatalf("failed to create Casper client: %v", err)
	}

	if err := client.Health(ctx); err != nil {
		log.Fatalf("health check failed: %v", err)
	}

	const collection = "example_collection"
	const dim = 128

	// Create collection
	if err := client.CreateCollection(ctx, collection, casper.CreateCollectionRequest{
		Dim:     dim,
		MaxSize: 10_000,
	}); err != nil {
		log.Fatalf("create collection failed: %v", err)
	}

	// Upsert vectors (batch)
	upserts := make([]casper.VectorPoint, 0, 5)
	for id := uint32(1); id <= 5; id++ {
		upserts = append(upserts, casper.VectorPoint{ID: id, Vector: genVec(dim)})
	}
	if err := client.UpsertVectors(ctx, collection, upserts); err != nil {
		log.Fatalf("upsert vectors failed: %v", err)
	}

	// Mixed insert + delete
	var batch []casper.BatchInsertOperation
	for id := uint32(6); id <= 10; id++ {
		batch = append(batch, casper.BatchInsertOperation{ID: id, Vector: genVec(dim)})
	}
	if err := client.BatchUpdate(ctx, collection, casper.BatchUpdateRequest{
		Insert: batch,
		Delete: []uint32{},
	}); err != nil {
		log.Fatalf("batch update failed: %v", err)
	}

	// Create HNSW index (build runs asynchronously on the server)
	norm := true
	if err := client.CreateHNSWIndex(ctx, collection, casper.HNSWIndex{
		HNSW: casper.HNSW{
			Metric:         "inner-product",
			Quantization:   "i8",
			M:              16,
			M0:             32,
			EfConstruction: 200,
		},
		Normalization: &norm,
	}); err != nil {
		log.Fatalf("create HNSW index failed: %v", err)
	}

	// Poll until the index is ready before searching.
	for {
		info, err := client.GetCollection(ctx, collection)
		if err != nil {
			log.Fatalf("get collection failed: %v", err)
		}
		if info.HasIndex {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Search
	results, err := client.Search(ctx, collection, genVec(dim), 30)
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}
	for i, r := range results {
		log.Printf("  %d. id=%d score=%f", i+1, r.ID, r.Score)
	}

	// Cleanup
	_ = client.DeleteIndex(ctx, collection)
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

The Go client covers all public HTTP endpoints exposed by the current Casper API:

- **Collections**: create, delete, list, get info.
- **Vectors**: `UpsertVectors` (batch PUT), `DeleteVectors` (batch by id), `BatchUpdate` (mixed insert+delete), `GetVector`.
- **Mute / unmute**: `MuteCollection`, `UnmuteCollection`.
- **Indexes**: `CreateHNSWIndex` (async build — server returns 202), `DeleteIndex`.
- **Search**: `Search`, `SearchWithEf` (overrides the HNSW `ef_search` parameter). The binary search response format is decoded automatically.
- **Health**: `/health` liveness/readiness endpoint.

## License

Licensed under the [Apache License Version 2.0](LICENSE).
