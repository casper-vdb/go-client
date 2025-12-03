# Casper Go client

Client library for the [Casper Vector Database](https://github.com/casper-vdb/Casper) written in Golang.

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

The example below mirrors `examples/main.go` and demonstrates:
collection management, single and batch inserts, HNSW index creation, search, reading and deleting vectors.

```go
package main

import (
	"context"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	casper "github.com/casper-vdb/go-client"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host, httpPort, grpcPort := resolveConfig()

	client, err := casper.New(host, httpPort, grpcPort)
	if err != nil {
		log.Fatalf("failed to create Casper client: %v", err)
	}

	log.Printf("Using Casper at host=%s http=%d grpc=%d", host, httpPort, grpcPort)

	// 1. Health check
	if err := client.Health(ctx); err != nil {
		log.Fatalf("casper health check failed: %v", err)
	}
	log.Println("Health check OK")

	const collection = "example_collection"
	const dim = 128

	// 2. Create collection
	log.Println("Creating collection...")
	err = client.CreateCollection(ctx, collection, casper.CreateCollectionRequest{
		Dim:     dim,
		MaxSize: 10_000,
	})
	if err != nil {
		log.Fatalf("create collection failed: %v", err)
	}
	log.Printf("Collection %q created", collection)

	// 3. Insert some vectors
	log.Println("Inserting vectors...")
	for id := uint32(1); id <= 5; id++ {
		vec := genVec(dim)
		err := client.InsertVector(ctx, collection, casper.InsertRequest{
			ID:     id,
			Vector: vec,
		})
		if err != nil {
			log.Fatalf("insert vector %d failed: %v", id, err)
		}
	}
	log.Println("Single inserts completed")

	// 4. Batch insert more vectors
	log.Println("Batch inserting vectors...")
	var batch []casper.BatchInsertOperation
	for id := uint32(6); id <= 10; id++ {
		vec := genVec(dim)
		batch = append(batch, casper.BatchInsertOperation{
			ID:     id,
			Vector: vec,
		})
	}
	err = client.BatchUpdate(ctx, collection, casper.BatchUpdateRequest{
		Insert: batch,
		Delete: []uint32{},
	})
	if err != nil {
		log.Fatalf("batch update failed: %v", err)
	}
	log.Println("Batch insert completed")

	// 5. Create HNSW index
	log.Println("Creating HNSW index...")
	normalization := true
	err = client.CreateHNSWIndex(ctx, collection, casper.HNSWIndex{
		HNSW: casper.HNSW{
			Metric:         "inner-product",
			Quantization:   "i8",
			M:              16,
			M0:             32,
			EfConstruction: 200,
		},
		Normalization: &normalization,
	})
	if err != nil {
		log.Fatalf("create HNSW index failed: %v", err)
	}
	log.Println("HNSW index created")

	// 6. Search
	log.Println("Searching for similar vectors...")
	query := genVec(128)
	results, err := client.Search(ctx, collection, query, 30)
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}

	log.Printf("Found %d results:", len(results))
	for i, r := range results {
		log.Printf("  %d. id=%d score=%f", i+1, r.ID, r.Score)
	}

	// 7. Get a specific vector
	log.Println("Getting vector with id=1...")
	v, err := client.GetVector(ctx, collection, 1)
	if err != nil {
		log.Fatalf("get vector failed: %v", err)
	}
	log.Printf("Vector 1 has %d dimensions", len(v.Vector))

	// 8. Delete a vector
	log.Println("Deleting vector with id=10...")
	if err := client.DeleteVector(ctx, collection, casper.DeleteRequest{ID: 10}); err != nil {
		log.Fatalf("delete vector failed: %v", err)
	}
	log.Println("Vector 10 deleted")

	// 9. List collections
	log.Println("Listing collections...")
	list, err := client.ListCollections(ctx)
	if err != nil {
		log.Fatalf("list collections failed: %v", err)
	}
	for _, col := range list.Collections {
		log.Printf("  - %s (dim=%d mutable=%v has_index=%v)",
			col.Name, col.Dimension, col.Mutable, col.HasIndex)
	}

	// 10. Delete index and collection
	log.Println("Deleting index...")
	if err := client.DeleteIndex(ctx, collection); err != nil {
		log.Fatalf("delete index failed: %v", err)
	}
	log.Println("Index deleted")

	log.Println("Deleting collection...")
	if err := client.DeleteCollection(ctx, collection); err != nil {
		log.Fatalf("delete collection failed: %v", err)
	}
	log.Println("Collection deleted")

	log.Println("Example completed successfully")
}

func resolveConfig() (host string, httpPort, grpcPort int) {
	host = os.Getenv("CASPER_HOST")
	if host == "" {
		host = "http://127.0.0.1"
	}

	httpPort = parseEnvInt("CASPER_HTTP_PORT", 8080)
	grpcPort = parseEnvInt("CASPER_GRPC_PORT", 50051)

	return host, httpPort, grpcPort
}

func parseEnvInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func genVec(dim int) []float32 {
	var sum float32 = 0
	vec := make([]float32, dim)
	for i := 0; i < len(vec); i++ {
		vec[i] = 2*rand.Float32() - 1
		sum = sum + vec[i]*vec[i]
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := 0; i < len(vec); i++ {
		vec[i] = vec[i] / norm
	}
	return vec
}
```

[//]: # (### gRPC UploadMatrix)

[//]: # ()
[//]: # (For uploading large matrices of vectors, the client exposes the gRPC method `UploadMatrix`,)

[//]: # (generated from `proto/matrix_service.proto`.)

[//]: # ()
[//]: # (```go)

[//]: # (ctx, cancel := context.WithTimeout&#40;context.Background&#40;&#41;, 60*time.Second&#41;)

[//]: # (defer cancel&#40;&#41;)

[//]: # ()
[//]: # (client, err := casper.New&#40;"http://localhost", 8080, 50051&#41;)

[//]: # (if err != nil {)

[//]: # (	log.Fatalf&#40;"failed to create client: %v", err&#41;)

[//]: # (})

[//]: # ()
[//]: # (dim := uint32&#40;3&#41;)

[//]: # (vectors := []float32{)

[//]: # (	1.0, 2.0, 3.0,)

[//]: # (	4.0, 5.0, 6.0,)

[//]: # (})

[//]: # ()
[//]: # (res, err := client.UploadMatrix&#40;ctx, "demo_matrix", dim, vectors, 6&#41;)

[//]: # (if err != nil {)

[//]: # (	log.Fatalf&#40;"upload matrix failed: %v", err&#41;)

[//]: # (})

[//]: # ()
[//]: # (log.Printf&#40;"%s &#40;total_vectors=%d, total_chunks=%d&#41;",)

[//]: # (	res.Message, res.TotalVectors, res.TotalChunks&#41;)

[//]: # (```)

### Features

The Go client covers all public HTTP endpoints described in `api.md`:

- **Collections**: create, delete, list, get info.
- **Vectors**: insert, batch update, search, get by id, delete.
- **Indexes**: create/delete HNSW index.
- **Matrices**: HTTP operations (list/info/delete) + gRPC `UploadMatrix`.
- **PQ**: create, delete, list, get info.
- **Health**: `/health` liveness/readiness endpoint.

[//]: # (See `api.md` or the main Casper project documentation for complete details on request/response formats.)

[//]: # (### gRPC code generation)

[//]: # ()
[//]: # (The repository includes a `generate-casper-grpc` Make target:)

[//]: # ()
[//]: # (```bash)

[//]: # (make generate-casper-grpc)

[//]: # (```)

[//]: # ()
[//]: # (It installs the required plugins &#40;`protoc-gen-go`, `protoc-gen-go-grpc`&#41;)

[//]: # (and generates Go code into `pkg/proto` from `proto/matrix_service.proto`.)

[//]: # (Generated files are committed so end users of `go get` do not need `protoc` installed.)

## License

Licensed under the [Apache License Version 2.0](LICENSE).
