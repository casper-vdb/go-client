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
