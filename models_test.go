package casper

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// decodeStrict decodes the api.md examples refusing unknown keys, so a field
// the server returns and the models lack fails the test.
func decodeStrict(t *testing.T, body string, out any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestDecodeClusterStandalone(t *testing.T) {
	var info ClusterInfo
	decodeStrict(t, `{"role": "standalone"}`, &info)
	if info.Role != ClusterRoleStandalone || info.NodeID != "" || info.LeaderID != nil || info.Members != nil {
		t.Fatalf("unexpected %+v", info)
	}
}

func TestDecodeClusterCluster(t *testing.T) {
	// docs/api.md's example, with a member and the two indexes it omits.
	var info ClusterInfo
	decodeStrict(t, `{
  "role": "cluster",
  "node_id": "ea1ae526-cd80-47f6-9ea8-346909637655",
  "raft_state": "leader",
  "current_term": 3,
  "leader_id": "ea1ae526-cd80-47f6-9ea8-346909637655",
  "voters": ["09a656b1-c95e-46ea-87b2-721b7e6f8ebf", "ea1ae526-cd80-47f6-9ea8-346909637655"],
  "learners": [],
  "members": [
    {"node_id": "ea1ae526-cd80-47f6-9ea8-346909637655", "raft_addr": "127.0.0.1:7224", "replication_addr": "127.0.0.1:7223", "voter": true}
  ],
  "last_log_index": 4,
  "last_applied_index": 4
}`, &info)
	if info.Role != ClusterRoleCluster || info.RaftState != RaftStateLeader || info.CurrentTerm != 3 {
		t.Fatalf("unexpected %+v", info)
	}
	if info.LeaderID == nil || *info.LeaderID != info.NodeID || len(info.Voters) != 2 || len(info.Members) != 1 {
		t.Fatalf("unexpected %+v", info)
	}
	if info.Members[0].RaftAddr != "127.0.0.1:7224" || info.Members[0].ReplicationAddr != "127.0.0.1:7223" || !info.Members[0].Voter {
		t.Fatalf("unexpected member %+v", info.Members[0])
	}
	if info.LastLogIndex == nil || *info.LastLogIndex != 4 || info.LastAppliedIndex == nil || *info.LastAppliedIndex != 4 {
		t.Fatalf("unexpected indexes %+v", info)
	}
}

func TestDecodeConfig(t *testing.T) {
	// docs/api.md's example, with service.shutdown_timeout_sec it omits.
	var config AppConfig
	decodeStrict(t, `{
  "service": {"host": "127.0.0.1", "http_port": 7222, "uri": null, "shutdown_timeout_sec": 25},
  "cluster": {
    "role": "standalone",
    "bootstrap": null,
    "write_ack_timeout_ms": 5000,
    "max_inflight_entries": 16,
    "max_apply_lag_mb": 1024,
    "write_queue_mb": 128,
    "raft": {"port": 7224, "enable_tls": false, "uri": null},
    "replication": {"port": 7223, "uri": null, "max_transfer_mb_per_sec": 50}
  },
  "storage": {
    "storage_dir": "./storage",
    "min_free_disk_mb": 8192,
    "wal": {"max_batch_size_mb": 1, "log_tail_mb": 1024, "snapshot_every_mb": 256, "snapshot_every_entries": 5000},
    "index": {"snapshot": {"enabled": true}, "compaction": {"enabled": true}}
  },
  "search": {"max_search_limit": 100000, "max_scores_ids": 100000},
  "tls": {"enable": false, "verify_https_client_certificate": false, "tls_cert_ttl": null},
  "auth": {"jwt_rbac": false},
  "metrics": {"prefix": "casper_"}
}`, &config)
	if config.Service.HTTPPort != 7222 || config.Service.URI != nil || config.Service.ShutdownTimeoutSec != 25 {
		t.Fatalf("unexpected service %+v", config.Service)
	}
	if config.Cluster.Role != ClusterRoleStandalone || config.Cluster.Bootstrap != nil || config.Cluster.Raft.Port != 7224 ||
		config.Cluster.Replication.MaxTransferMBPerSec != 50 || config.Cluster.WriteQueueMB != 128 {
		t.Fatalf("unexpected cluster %+v", config.Cluster)
	}
	if config.Storage.StorageDir != "./storage" || config.Storage.WAL.SnapshotEveryEntries != 5000 || !config.Storage.Index.Compaction.Enabled {
		t.Fatalf("unexpected storage %+v", config.Storage)
	}
	if config.Search.MaxSearchLimit != 100000 || config.Search.MaxScoresIDs != 100000 {
		t.Fatalf("unexpected search %+v", config.Search)
	}
	if config.TLS.TLSCertTTL != nil || config.Metrics.Prefix != "casper_" {
		t.Fatalf("unexpected %+v", config)
	}
}

func TestDecodeCollectionInfo(t *testing.T) {
	// docs/api.md's example, with max_chain_depth turned off.
	var info CollectionInfo
	decodeStrict(t, `{
  "status": "green",
  "id": "d0d84675-4cb5-4663-9463-bb261848fc36",
  "name": "demo",
  "dimension": 96,
  "max_size": 10000000,
  "size": 9990000,
  "applied_size": 9990000,
  "has_index": true,
  "replicated": false,
  "wal": {"write_count": 52813, "applied_count": 52813, "snapshotted_count": 52813},
  "index": {
    "state": "ready",
    "hnsw": {"metric": "inner-product", "quantization": "f32", "m": 16, "m0": 32, "ef_construction": 200},
    "normalization": true,
    "storage": {
      "range_size_bytes": 134217728,
      "snapshot": {"enabled": true, "changes": 100000, "wal_ops": 200000},
      "compaction": {"enabled": true, "max_chain_depth": null, "max_tail_ratio": 0.25},
      "snapshot_active": true,
      "compaction_active": true
    }
  },
  "snapshot": {"changes": 0, "wal_ops": 0}
}`, &info)
	if info.Index.State != IndexStateReady || info.Index.HNSW.M0 != 32 || info.WAL.SnapshottedCount != 52813 || info.Snapshot == nil {
		t.Fatalf("unexpected %+v", info)
	}
	storage := info.Index.Storage
	if storage.RangeSizeBytes != 134217728 || storage.Compaction.MaxChainDepth != nil || !storage.SnapshotActive {
		t.Fatalf("unexpected storage %+v", storage)
	}
}

func TestDecodeCollectionInfoNotStarted(t *testing.T) {
	var info CollectionInfo
	decodeStrict(t, `{
  "status": "green", "id": "d0d84675-4cb5-4663-9463-bb261848fc36", "name": "demo", "dimension": 4,
  "max_size": 100, "size": 0, "applied_size": 0, "has_index": false, "replicated": false,
  "wal": {"write_count": 0, "applied_count": 0, "snapshotted_count": 0},
  "index": {
    "state": "not_started",
    "hnsw": {"metric": "euclidean", "quantization": "i8", "m": 16, "m0": 32, "ef_construction": 200},
    "normalization": false,
    "storage": {
      "range_size_bytes": 134217728,
      "snapshot": {"enabled": true, "changes": 10, "wal_ops": 20},
      "compaction": {"enabled": true, "max_chain_depth": 8, "max_tail_ratio": 0.25},
      "snapshot_active": true,
      "compaction_active": true
    }
  }
}`, &info)
	depth := info.Index.Storage.Compaction.MaxChainDepth
	if info.Index.State != IndexStateNotStarted || info.Snapshot != nil || depth == nil || *depth != 8 {
		t.Fatalf("unexpected %+v", info)
	}
}

func TestDecodeVector(t *testing.T) {
	var f32 GetVectorResponse
	decodeStrict(t, `{"id": 42, "vector": [0.1, 0.2, 1.0, -3e-8]}`, &f32)
	if f32.ID != 42 || f32.Vector.IsI8() || len(f32.Vector.F32) != 4 || f32.Vector.F32[0] != 0.1 {
		t.Fatalf("unexpected %+v", f32)
	}

	var i8 GetVectorResponse
	decodeStrict(t, `{"id": 42, "vector": [10, 20, -100, 100]}`, &i8)
	if !i8.Vector.IsI8() || i8.Vector.F32 != nil || i8.Vector.I8[2] != -100 || i8.Vector.I8[3] != 100 {
		t.Fatalf("unexpected %+v", i8)
	}

	var bad GetVectorResponse
	if err := json.Unmarshal([]byte(`{"id": 1, "vector": [1000]}`), &bad); err == nil {
		t.Fatalf("an integer outside i8 decoded: %+v", bad)
	}
}

func TestEncodeCreateCollection(t *testing.T) {
	got, err := json.Marshal(CreateCollectionRequest{
		Dim:     4,
		MaxSize: 100,
		Index: IndexSpec{
			HNSW:    HNSW{Metric: "euclidean", Quantization: "f32", M: 16, M0: 32, EfConstruction: 200},
			Storage: &IndexStorage{Compaction: &CompactionPatch{MaxChainDepth: ChainDepthOff()}},
		},
	})
	want := `{"dim":4,"max_size":100,"index":{"hnsw":{"metric":"euclidean","quantization":"f32","m":16,"m0":32,"ef_construction":200},"storage":{"compaction":{"max_chain_depth":null}}}}`
	if err != nil || string(got) != want {
		t.Fatalf("got %s (%v), want %s", got, err, want)
	}
}

func TestEncodeChainDepth(t *testing.T) {
	cases := map[string]IndexPolicyPatch{
		`{"storage":{"compaction":{"max_chain_depth":null}}}`: {Compaction: &CompactionPatch{MaxChainDepth: ChainDepthOff()}},
		`{"storage":{"compaction":{"max_chain_depth":5}}}`:    {Compaction: &CompactionPatch{MaxChainDepth: ChainDepthOf(5)}},
		`{"storage":{"compaction":{}}}`:                       {Compaction: &CompactionPatch{}},
	}
	for want, patch := range cases {
		got, err := json.Marshal(updateIndexBody{Storage: patch})
		if err != nil || string(got) != want {
			t.Fatalf("got %s (%v), want %s", got, err, want)
		}
	}
}

func TestErrorFromResponse(t *testing.T) {
	notLeader := errorFromResponse(&http.Response{StatusCode: 405, Header: http.Header{}},
		[]byte(`{"error": "Method not allowed: mutating operations require the Raft leader", "leader_addr": "http://127.0.0.1:7222"}`))
	if notLeader.Kind != ErrorKindNotLeader || notLeader.LeaderAddr != "http://127.0.0.1:7222" {
		t.Fatalf("unexpected %+v", notLeader)
	}
	refused := errorFromResponse(&http.Response{StatusCode: 405, Header: http.Header{}}, []byte(`Method not allowed: index is being built`))
	if refused.Kind != ErrorKindOperationNotAllowed || refused.Message != "Method not allowed: index is being built" {
		t.Fatalf("unexpected %+v", refused)
	}
	overloaded := errorFromResponse(&http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"3"}}}, []byte(`Too many requests: queue full`))
	if overloaded.Kind != ErrorKindTooManyRequests || overloaded.RetryAfter != 3*time.Second {
		t.Fatalf("unexpected %+v", overloaded)
	}
	peer := errorFromResponse(&http.Response{StatusCode: 409, Header: http.Header{}}, []byte(`{"error": "node x is the only voter"}`))
	if peer.Kind != ErrorKindConflict || peer.Message != "node x is the only voter" {
		t.Fatalf("unexpected %+v", peer)
	}
}

func TestEncodeZeroChainDepthIsRefused(t *testing.T) {
	patch := IndexPolicyPatch{Compaction: &CompactionPatch{MaxChainDepth: &ChainDepth{}}}
	if got, err := json.Marshal(updateIndexBody{Storage: patch}); err == nil {
		t.Fatalf("got %s, want an error", got)
	}
}
