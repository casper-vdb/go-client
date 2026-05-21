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

	host, httpPort := resolveConfig()

	client, err := casper.New(host, httpPort)
	if err != nil {
		log.Fatalf("failed to create Casper client: %v", err)
	}

	log.Printf("Using Casper at host=%s http=%d", host, httpPort)

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

	// 3. Upsert vectors (single batch call replaces the old per-id insert).
	log.Println("Upserting vectors...")
	upserts := make([]casper.VectorPoint, 0, 5)
	for id := uint32(1); id <= 5; id++ {
		upserts = append(upserts, casper.VectorPoint{ID: id, Vector: genVec(dim)})
	}
	if err := client.UpsertVectors(ctx, collection, upserts); err != nil {
		log.Fatalf("upsert vectors failed: %v", err)
	}
	log.Println("Upsert completed")

	// 4. Mixed insert + delete in a single batch.
	log.Println("Batch updating vectors...")
	var batch []casper.BatchInsertOperation
	for id := uint32(6); id <= 10; id++ {
		batch = append(batch, casper.BatchInsertOperation{ID: id, Vector: genVec(dim)})
	}
	err = client.BatchUpdate(ctx, collection, casper.BatchUpdateRequest{
		Insert: batch,
		Delete: []uint32{},
	})
	if err != nil {
		log.Fatalf("batch update failed: %v", err)
	}
	log.Println("Batch update completed")

	// 5. Create HNSW index. The server returns 202 immediately; the index
	//    build runs in the background.
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
	log.Println("HNSW index build scheduled")

	// Wait for the index to be ready before searching.
	log.Println("Waiting for index build to finish...")
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
	log.Println("Index ready")

	// 6. Search
	log.Println("Searching for similar vectors...")
	query := genVec(dim)
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

	// 8. Delete vectors by id
	log.Println("Deleting vectors [10]...")
	if err := client.DeleteVectors(ctx, collection, []uint32{10}); err != nil {
		log.Fatalf("delete vectors failed: %v", err)
	}
	log.Println("Vector 10 deleted")

	// 9. Mute / unmute roundtrip
	log.Println("Muting and unmuting collection...")
	if err := client.MuteCollection(ctx, collection); err != nil {
		log.Fatalf("mute failed: %v", err)
	}
	if err := client.UnmuteCollection(ctx, collection); err != nil {
		log.Fatalf("unmute failed: %v", err)
	}
	log.Println("Mute/unmute roundtrip ok")

	// 10. List collections
	log.Println("Listing collections...")
	list, err := client.ListCollections(ctx)
	if err != nil {
		log.Fatalf("list collections failed: %v", err)
	}
	for _, col := range list.Collections {
		log.Printf("  - %s (dim=%d size=%d status=%s mutable=%v has_index=%v)",
			col.Name, col.Dimension, col.Size, col.Status, col.Mutable, col.HasIndex)
	}

	// 11. Delete index and collection
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

func resolveConfig() (host string, httpPort int) {
	host = os.Getenv("CASPER_HOST")
	if host == "" {
		host = "http://127.0.0.1"
	}
	httpPort = parseEnvInt("CASPER_HTTP_PORT", 8080)
	return host, httpPort
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
