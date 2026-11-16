package main

import (
	"context"
	"fmt"
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

	host, httpPort, apiKey := resolveConfig()

	client, err := casper.NewWithOptions(host, httpPort, casper.Options{APIKey: apiKey})
	if err != nil {
		log.Fatalf("failed to create Casper client: %v", err)
	}

	log.Printf("Using Casper at host=%s http=%d", host, httpPort)

	// 1. Health check
	if err := client.Health(ctx); err != nil {
		log.Fatalf("casper health check failed: %v", err)
	}
	log.Println("Health check OK")

	if err := client.Ready(ctx, false); err != nil {
		log.Fatalf("casper readiness check failed: %v", err)
	}
	log.Println("Readiness check OK")

	cluster, err := client.GetCluster(ctx)
	if err != nil {
		log.Fatalf("get cluster failed: %v", err)
	}
	if cluster.Role == casper.ClusterRoleCluster {
		log.Printf("Node %s: role=%s raft_state=%s members=%d",
			cluster.NodeID, cluster.Role, cluster.RaftState, len(cluster.Members))
	} else {
		log.Printf("Node: role=%s", cluster.Role)
	}

	config, err := client.GetConfig(ctx)
	if err != nil {
		log.Fatalf("get config failed: %v", err)
	}
	log.Printf("Storage dir: %s", config.Storage.StorageDir)

	const collection = "example_collection"
	const dim = 128

	// 2. Create the collection with the schema and policy of its index.
	//    Storage is optional; every setting left out keeps its default.
	log.Println("Creating collection...")
	normalization := true
	snapshotChanges := uint64(1_000)
	created, err := client.CreateCollection(ctx, collection, casper.CreateCollectionRequest{
		Dim:     dim,
		MaxSize: 10_000,
		Index: casper.IndexSpec{
			HNSW: casper.HNSW{
				Metric:         "inner-product",
				Quantization:   "i8",
				M:              16,
				M0:             32,
				EfConstruction: 200,
			},
			Normalization: &normalization,
			Storage: &casper.IndexStorage{
				Snapshot: &casper.SnapshotPatch{Changes: &snapshotChanges},
			},
		},
	})
	if err != nil {
		log.Fatalf("create collection failed: %v", err)
	}
	log.Printf("Collection %q created with id %s", collection, created.ID)

	// 3. Write: upserts and deletes go in one batch.
	log.Println("Writing vectors...")
	var upserts []casper.UpsertOperation
	for id := uint64(1); id <= 10; id++ {
		upserts = append(upserts, casper.UpsertOperation{ID: id, Vector: genVec(dim)})
	}
	err = client.BatchUpdate(ctx, collection, casper.BatchUpdateRequest{
		Upserts: upserts,
		Delete:  []uint64{},
	})
	if err != nil {
		log.Fatalf("batch update failed: %v", err)
	}
	log.Println("Batch update completed")

	// 4. Start indexing. The server returns 202; the build runs in the
	//    background, so wait for index.state to become ready.
	log.Println("Starting indexing...")
	if err := client.StartIndexing(ctx, collection); err != nil {
		log.Fatalf("start indexing failed: %v", err)
	}
	if err := waitForIndex(ctx, client, collection); err != nil {
		log.Fatalf("wait for index failed: %v", err)
	}
	log.Println("Index ready")

	// 5. Change the policy of the index: turn the chain-depth rule off and
	//    raise the tail ratio. Unset fields keep their values.
	log.Println("Updating index policy...")
	tailRatio := 0.5
	err = client.UpdateIndexPolicy(ctx, collection, casper.IndexPolicyPatch{
		Compaction: &casper.CompactionPatch{
			MaxChainDepth: casper.ChainDepthOff(),
			MaxTailRatio:  &tailRatio,
		},
	})
	if err != nil {
		log.Fatalf("update index policy failed: %v", err)
	}

	// 6. Snapshot and compact on this node now, if the node runs them.
	if config.Storage.Index.Snapshot.Enabled {
		if err := client.TriggerSnapshot(ctx, collection); err != nil {
			log.Fatalf("trigger snapshot failed: %v", err)
		}
		log.Println("Snapshot triggered")
	}
	if config.Storage.Index.Compaction.Enabled {
		if err := client.TriggerCompaction(ctx, collection); err != nil {
			log.Fatalf("trigger compaction failed: %v", err)
		}
		log.Println("Compaction triggered")
	}

	// 7. Search
	log.Println("Searching for similar vectors...")
	query := genVec(dim)
	results, err := client.Search(ctx, collection, query, 5)
	if err != nil {
		log.Fatalf("search failed: %v", err)
	}
	log.Printf("Found %d results:", len(results))
	for i, r := range results {
		log.Printf("  %d. id=%d score=%f", i+1, r.ID, r.Score)
	}

	// 8. Scores: the same query against chosen ids. Id 42 was never
	//    written, so it is left out of the answer.
	scores, err := client.Scores(ctx, collection, query, []uint64{1, 2, 42})
	if err != nil {
		log.Fatalf("scores failed: %v", err)
	}
	for _, r := range scores {
		log.Printf("  id=%d score=%f", r.ID, r.Score)
	}

	// 9. Get a specific vector. An i8 index answers with its quantized
	//    components.
	v, err := client.GetVector(ctx, collection, 1)
	if err != nil {
		log.Fatalf("get vector failed: %v", err)
	}
	if v == nil {
		log.Fatalf("vector 1 not found")
	}
	if v.Vector.IsI8() {
		log.Printf("Vector 1 has %d i8 components", len(v.Vector.I8))
	} else {
		log.Printf("Vector 1 has %d f32 components", len(v.Vector.F32))
	}

	// 10. List collections and show this one
	list, err := client.ListCollections(ctx)
	if err != nil {
		log.Fatalf("list collections failed: %v", err)
	}
	for _, col := range list.Collections {
		log.Printf("  - %s", col.Name)
	}
	info, err := client.GetCollection(ctx, collection)
	if err != nil {
		log.Fatalf("get collection failed: %v", err)
	}
	log.Printf("%s: dim=%d size=%d applied_size=%d status=%s index=%s writes=%d applied=%d",
		info.Name, info.Dimension, info.Size, info.AppliedSize, info.Status,
		info.Index.State, info.WAL.WriteCount, info.WAL.AppliedCount)

	// 11. Delete collection (its index goes with it)
	log.Println("Deleting collection...")
	if err := client.DeleteCollection(ctx, collection); err != nil {
		log.Fatalf("delete collection failed: %v", err)
	}
	log.Println("Collection deleted")

	log.Println("Example completed successfully")
}

// waitForIndex polls the collection until its index is ready. It gives up
// when ctx is done, and fails when the state is back to not_started, which
// after a start means the build failed.
func waitForIndex(ctx context.Context, client *casper.Client, collection string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := client.GetCollection(ctx, collection)
		if err != nil {
			return err
		}
		switch info.Index.State {
		case casper.IndexStateReady:
			return nil
		case casper.IndexStateNotStarted:
			return fmt.Errorf("index build of %q failed: state is %s", collection, info.Index.State)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func resolveConfig() (host string, httpPort int, apiKey string) {
	host = os.Getenv("CASPER_HOST")
	if host == "" {
		host = "http://127.0.0.1"
	}
	httpPort = parseEnvInt("CASPER_HTTP_PORT", 7222)
	apiKey = os.Getenv("CASPER_API_KEY")
	return host, httpPort, apiKey
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
