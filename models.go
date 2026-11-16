package casper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// searchVectorBody is used as the JSON body for /search requests.
type searchVectorBody struct {
	Vector []float32 `json:"vector"`
}

// scoresBody is used as the JSON body for /scores requests.
type scoresBody struct {
	Vector []float32 `json:"vector"`
	IDs    []uint64  `json:"ids"`
}

// SearchResult is a single scored id, as search and scores answer it.
type SearchResult struct {
	ID    uint64  `json:"id"`
	Score float32 `json:"score"`
}

// SearchResponse is a convenience alias for a slice of results.
type SearchResponse []SearchResult

// CreateCollectionRequest is the body of POST /collection/{name}: the
// collection and the schema and policy of its index.
type CreateCollectionRequest struct {
	Dim     int       `json:"dim"`
	MaxSize uint32    `json:"max_size"`
	Index   IndexSpec `json:"index"`
}

// CreateCollectionResponse is the response of POST /collection/{name}.
type CreateCollectionResponse struct {
	// ID is the uuid the collection was created under.
	ID string `json:"id"`
}

// CollectionsListResponse mirrors the JSON returned from /collections.
// The list carries names only; call GetCollection for the details of one.
type CollectionsListResponse struct {
	Collections []CollectionName `json:"collections"`
}

// CollectionName is one entry of the /collections list.
type CollectionName struct {
	Name string `json:"name"`
}

// CollectionInfo contains metadata about a single collection, returned from
// GET /collections/{name}.
type CollectionInfo struct {
	// Status is "yellow" while an index is being built, "green" otherwise.
	Status    string `json:"status"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Dimension int    `json:"dimension"`
	MaxSize   uint32 `json:"max_size"`
	// Size is the number of documents durably stored.
	Size uint32 `json:"size"`
	// AppliedSize is the number of documents visible to search; it lags
	// Size by the applier backlog.
	AppliedSize uint32 `json:"applied_size"`
	HasIndex    bool   `json:"has_index"`
	// Replicated is true when the collection was created in cluster mode.
	Replicated bool        `json:"replicated"`
	WAL        WALCounters `json:"wal"`
	// Index is always there: the schema and policy the collection was
	// created with, and where indexing is.
	Index IndexInfo `json:"index"`
	// Snapshot holds the counters the snapshot thresholds are compared
	// against. Nil while no index is attached.
	Snapshot *SnapshotCounters `json:"snapshot,omitempty"`
}

// WALCounters counts writes, one WAL record each, from the collection's
// creation. While no index is attached AppliedCount and SnapshottedCount
// are 0.
type WALCounters struct {
	// WriteCount is the number of writes durably in the WAL.
	WriteCount uint64 `json:"write_count"`
	// AppliedCount is the number of writes applied to the in-memory index,
	// so visible to search.
	AppliedCount uint64 `json:"applied_count"`
	// SnapshottedCount is the number of writes the index on disk covers.
	SnapshottedCount uint64 `json:"snapshotted_count"`
}

// SnapshotCounters is what has accumulated since the last snapshot.
type SnapshotCounters struct {
	// Changes is the number of ids changed since the last snapshot.
	Changes uint64 `json:"changes"`
	// WalOps is the number of WAL operations since the last snapshot.
	WalOps uint64 `json:"wal_ops"`
}

// IndexState is where indexing of a collection is.
type IndexState string

const (
	// IndexStateNotStarted: indexing has not been started. After a failed
	// build in standalone the state returns to it.
	IndexStateNotStarted IndexState = "not_started"
	// IndexStateBuilding: the index is being built here or, in a cluster,
	// not fetched yet.
	IndexStateBuilding IndexState = "building"
	// IndexStateReady: an index is attached and serves search.
	IndexStateReady IndexState = "ready"
)

// IndexInfo contains metadata about a collection's index.
type IndexInfo struct {
	State         IndexState       `json:"state"`
	HNSW          HnswIndexInfo    `json:"hnsw"`
	Normalization bool             `json:"normalization"`
	Storage       IndexStorageInfo `json:"storage"`
}

// HnswIndexInfo describes a configured HNSW index returned by GetCollection.
type HnswIndexInfo struct {
	Metric         string `json:"metric"`
	Quantization   string `json:"quantization"`
	M              int    `json:"m"`
	M0             int    `json:"m0"`
	EfConstruction int    `json:"ef_construction"`
}

// IndexStorageInfo is how an index is kept on disk: the width of its
// payload ranges and its snapshot and compaction policy.
type IndexStorageInfo struct {
	RangeSizeBytes uint64           `json:"range_size_bytes"`
	Snapshot       SnapshotPolicy   `json:"snapshot"`
	Compaction     CompactionPolicy `json:"compaction"`
	// SnapshotActive is the node's snapshot flag AND the policy's.
	SnapshotActive bool `json:"snapshot_active"`
	// CompactionActive is the node's compaction flag AND the policy's.
	CompactionActive bool `json:"compaction_active"`
}

// SnapshotPolicy is the effective snapshot policy of an index.
type SnapshotPolicy struct {
	Enabled bool   `json:"enabled"`
	Changes uint64 `json:"changes"`
	WalOps  uint64 `json:"wal_ops"`
}

// CompactionPolicy is the effective compaction policy of an index.
type CompactionPolicy struct {
	Enabled bool `json:"enabled"`
	// MaxChainDepth is nil when the depth rule is off.
	MaxChainDepth *uint32 `json:"max_chain_depth"`
	MaxTailRatio  float64 `json:"max_tail_ratio"`
}

// UpsertOperation inserts or replaces the vector of one id in a batch update.
type UpsertOperation struct {
	ID     uint64    `json:"id"`
	Vector []float32 `json:"vector"`
}

// BatchUpdateRequest wraps a batch of upserts and deletes. Either list may
// be empty, not both.
type BatchUpdateRequest struct {
	Upserts []UpsertOperation `json:"upserts"`
	Delete  []uint64          `json:"delete"`
}

// IndexSpec is the schema and policy of a collection's index, given when
// the collection is created.
type IndexSpec struct {
	HNSW HNSW `json:"hnsw"`
	// Normalization is optional; nil leaves the server default (false).
	Normalization *bool `json:"normalization,omitempty"`
	// Storage is optional; nil leaves every storage setting at its default.
	Storage *IndexStorage `json:"storage,omitempty"`
}

// HNSW contains configuration options for HNSW.
type HNSW struct {
	// Metric is one of "euclidean", "l2sq", "cosine", "inner-product".
	Metric string `json:"metric"`
	// Quantization is one of "f32", "i8".
	Quantization   string `json:"quantization"`
	M              int    `json:"m"`
	M0             int    `json:"m0"`
	EfConstruction int    `json:"ef_construction"`
}

// IndexStorage is the optional storage part of IndexSpec. Every
// field is optional; a nil one is not sent and keeps its default.
type IndexStorage struct {
	// RangeSizeBytes is the width of a payload range on disk, 128 MiB..8 GiB
	// (default 128 MiB). Fixed for the life of the index.
	RangeSizeBytes *uint64          `json:"range_size_bytes,omitempty"`
	Snapshot       *SnapshotPatch   `json:"snapshot,omitempty"`
	Compaction     *CompactionPatch `json:"compaction,omitempty"`
}

// IndexPolicyPatch is the storage part of PATCH /collection/{name}/index:
// only the named keys change.
type IndexPolicyPatch struct {
	Snapshot   *SnapshotPatch   `json:"snapshot,omitempty"`
	Compaction *CompactionPatch `json:"compaction,omitempty"`
}

// updateIndexBody is the JSON body of PATCH /collection/{name}/index.
type updateIndexBody struct {
	Storage IndexPolicyPatch `json:"storage"`
}

// SnapshotPatch sets fields of an index's snapshot policy. A nil field is
// not sent and leaves the current value.
type SnapshotPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
	// Changes (>= 1) is how many ids changed since the last snapshot trigger
	// the next one.
	Changes *uint64 `json:"changes,omitempty"`
	// WalOps (>= Changes) is the backstop: WAL ops since the last snapshot.
	WalOps *uint64 `json:"wal_ops,omitempty"`
}

// CompactionPatch sets fields of an index's compaction policy. A nil field
// is not sent and leaves the current value.
type CompactionPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
	// MaxChainDepth is nil to leave it, ChainDepthOff() to turn the depth
	// rule off, or ChainDepthOf(n) (n >= 3) to set it.
	MaxChainDepth *ChainDepth `json:"max_chain_depth,omitempty"`
	// MaxTailRatio is in (0, 1).
	MaxTailRatio *float64 `json:"max_tail_ratio,omitempty"`
}

// ChainDepth is the value of CompactionPatch.MaxChainDepth: either a depth
// or "off", which is sent as JSON null.
type ChainDepth struct {
	depth uint32
	off   bool
}

// ChainDepthOf sets the depth rule to depth.
func ChainDepthOf(depth uint32) *ChainDepth {
	return &ChainDepth{depth: depth}
}

// ChainDepthOff turns the depth rule off.
func ChainDepthOff() *ChainDepth {
	return &ChainDepth{off: true}
}

// Off reports whether this value turns the depth rule off.
func (d ChainDepth) Off() bool {
	return d.off
}

// Depth returns the depth; it is meaningless when Off is true.
func (d ChainDepth) Depth() uint32 {
	return d.depth
}

// MarshalJSON writes null for "off" and the number otherwise. A depth of 0,
// which is also what a ChainDepth{} literal holds, is refused: the server
// takes at least 3, and "off" is ChainDepthOff().
func (d ChainDepth) MarshalJSON() ([]byte, error) {
	if d.off {
		return []byte("null"), nil
	}
	if d.depth == 0 {
		return nil, errors.New("max_chain_depth 0: use ChainDepthOf(n) with n >= 3, or ChainDepthOff()")
	}
	return json.Marshal(d.depth)
}

// GetVectorResponse mirrors the JSON returned from /collection/{name}/vector/{id}.
type GetVectorResponse struct {
	ID     uint64     `json:"id"`
	Vector Components `json:"vector"`
}

// Components is a vector exactly as the index stores it. Exactly one of the
// two is set: F32 for an f32 index (normalized if the index normalizes), I8
// for an i8 index, whose components are the quantized integers in
// [-100, 100] (round(x * 100)), not dequantized.
type Components struct {
	F32 []float32
	I8  []int8
}

// IsI8 reports whether the vector came from an i8 index.
func (c Components) IsI8() bool {
	return c.I8 != nil
}

// UnmarshalJSON decodes an array of floats into F32 and an array of
// integers into I8. The server prints every f32 with a fraction or an
// exponent, so an array with none is the i8 form.
func (c *Components) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var numbers []json.Number
	if err := dec.Decode(&numbers); err != nil {
		return err
	}

	isFloat := false
	for _, n := range numbers {
		if bytes.ContainsAny([]byte(n), ".eE") {
			isFloat = true
			break
		}
	}

	if isFloat {
		f32 := make([]float32, len(numbers))
		for i, n := range numbers {
			v, err := strconv.ParseFloat(string(n), 32)
			if err != nil {
				return fmt.Errorf("vector component %d: %w", i, err)
			}
			f32[i] = float32(v)
		}
		*c = Components{F32: f32}
		return nil
	}

	i8 := make([]int8, len(numbers))
	for i, n := range numbers {
		v, err := strconv.ParseInt(string(n), 10, 8)
		if err != nil {
			return fmt.Errorf("vector component %d: %w", i, err)
		}
		i8[i] = int8(v)
	}
	*c = Components{I8: i8}
	return nil
}

// ============================================================================
//  Cluster — GET /cluster, DELETE /cluster/peer/{id}, PUT /cluster/transfer-limit
// ============================================================================

// ClusterRole identifies how the node runs.
type ClusterRole string

const (
	ClusterRoleStandalone ClusterRole = "standalone"
	ClusterRoleCluster    ClusterRole = "cluster"
)

// RaftState is a cluster node's Raft state.
type RaftState string

const (
	RaftStateLeader    RaftState = "leader"
	RaftStateFollower  RaftState = "follower"
	RaftStateCandidate RaftState = "candidate"
	RaftStateLearner   RaftState = "learner"
	RaftStateShutdown  RaftState = "shutdown"
)

// ClusterInfo is the response payload for GET /cluster. A standalone node
// has no cluster identity and reports only Role; the other fields are set
// for ClusterRoleCluster and stay at their zero values otherwise.
type ClusterInfo struct {
	Role ClusterRole `json:"role"`

	// cluster-only ---------------------------------------------------------
	NodeID      string    `json:"node_id,omitempty"`
	RaftState   RaftState `json:"raft_state,omitempty"`
	CurrentTerm uint64    `json:"current_term,omitempty"`
	// LeaderID is nil while no leader is known.
	LeaderID *string         `json:"leader_id,omitempty"`
	Voters   []string        `json:"voters,omitempty"`
	Learners []string        `json:"learners,omitempty"`
	Members  []ClusterMember `json:"members,omitempty"`
	// LastLogIndex is nil while the log is empty.
	LastLogIndex *uint64 `json:"last_log_index,omitempty"`
	// LastAppliedIndex is nil while nothing has been applied.
	LastAppliedIndex *uint64 `json:"last_applied_index,omitempty"`
}

// ClusterMember is one member of the Raft membership, with the addresses it
// published for peers to dial.
type ClusterMember struct {
	NodeID string `json:"node_id"`
	// RaftAddr is where peers dial the member for consensus.
	RaftAddr string `json:"raft_addr"`
	// ReplicationAddr is where peers dial it for data-WAL replication.
	ReplicationAddr string `json:"replication_addr"`
	Voter           bool   `json:"voter"`
}

// RemovePeerResponse is the response of DELETE /cluster/peer/{node_id}.
type RemovePeerResponse struct {
	Removed string `json:"removed"`
}

// transferLimitBody is the JSON body of PUT /cluster/transfer-limit.
type transferLimitBody struct {
	MBPerSec uint64 `json:"mb_per_sec"`
}

// ============================================================================
//  Configuration — GET /config
// ============================================================================

// AppConfig mirrors the startup configuration the node runs with, returned
// from GET /config and grouped the way the config file is. Immutable after
// startup; restart the server to change a value. Paths to TLS key material
// and credentials are never included.
type AppConfig struct {
	Service ServiceConfig `json:"service"`
	Cluster ClusterConfig `json:"cluster"`
	Storage StorageConfig `json:"storage"`
	Search  SearchConfig  `json:"search"`
	TLS     TLSConfig     `json:"tls"`
	Auth    AuthConfig    `json:"auth"`
	Metrics MetricsConfig `json:"metrics"`
}

// ServiceConfig is the "service" section of AppConfig.
type ServiceConfig struct {
	Host     string `json:"host"`
	HTTPPort uint16 `json:"http_port"`
	// URI is the HTTP address the node publishes to peers; nil means it is
	// derived from Host and HTTPPort.
	URI                *string `json:"uri"`
	ShutdownTimeoutSec uint64  `json:"shutdown_timeout_sec"`
}

// ClusterConfig is the "cluster" section of AppConfig.
type ClusterConfig struct {
	Role ClusterRole `json:"role"`
	// Bootstrap is the address of the node this one joined through, nil if none.
	Bootstrap          *string           `json:"bootstrap"`
	WriteAckTimeoutMS  uint64            `json:"write_ack_timeout_ms"`
	MaxInflightEntries uint64            `json:"max_inflight_entries"`
	MaxApplyLagMB      uint64            `json:"max_apply_lag_mb"`
	WriteQueueMB       uint64            `json:"write_queue_mb"`
	Raft               RaftConfig        `json:"raft"`
	Replication        ReplicationConfig `json:"replication"`
}

// RaftConfig is the "cluster.raft" section of AppConfig.
type RaftConfig struct {
	Port      uint16 `json:"port"`
	EnableTLS bool   `json:"enable_tls"`
	// URI is the Raft address the node publishes to peers; nil means it is
	// derived from the service host and Port.
	URI *string `json:"uri"`
}

// ReplicationConfig is the "cluster.replication" section of AppConfig.
type ReplicationConfig struct {
	Port uint16 `json:"port"`
	// URI is the replication address the node publishes to peers; nil means
	// it is derived from the service host and Port.
	URI *string `json:"uri"`
	// MaxTransferMBPerSec is the configured transfer limit; SetTransferLimit
	// overrides it in memory without changing this value.
	MaxTransferMBPerSec uint64 `json:"max_transfer_mb_per_sec"`
}

// StorageConfig is the "storage" section of AppConfig.
type StorageConfig struct {
	StorageDir    string          `json:"storage_dir"`
	MinFreeDiskMB uint64          `json:"min_free_disk_mb"`
	WAL           WALConfig       `json:"wal"`
	Index         NodeIndexConfig `json:"index"`
}

// WALConfig is the "storage.wal" section of AppConfig.
type WALConfig struct {
	MaxBatchSizeMB       uint64 `json:"max_batch_size_mb"`
	LogTailMB            uint64 `json:"log_tail_mb"`
	SnapshotEveryMB      uint64 `json:"snapshot_every_mb"`
	SnapshotEveryEntries uint64 `json:"snapshot_every_entries"`
}

// NodeIndexConfig is the "storage.index" section of AppConfig: only the
// node's switches. The thresholds are each index's own, in GetCollection.
type NodeIndexConfig struct {
	Snapshot   NodeSwitch `json:"snapshot"`
	Compaction NodeSwitch `json:"compaction"`
}

// NodeSwitch says whether a mechanism is on for this node.
type NodeSwitch struct {
	Enabled bool `json:"enabled"`
}

// SearchConfig is the "search" section of AppConfig: the node's limits on
// queries.
type SearchConfig struct {
	// MaxSearchLimit is the highest limit a search may ask for.
	MaxSearchLimit uint64 `json:"max_search_limit"`
	// MaxScoresIDs is the most ids one Scores call may name.
	MaxScoresIDs uint64 `json:"max_scores_ids"`
}

// TLSConfig is the "tls" section of AppConfig.
type TLSConfig struct {
	Enable                       bool `json:"enable"`
	VerifyHTTPSClientCertificate bool `json:"verify_https_client_certificate"`
	// TLSCertTTL is nil when not set.
	TLSCertTTL *uint64 `json:"tls_cert_ttl"`
}

// AuthConfig is the "auth" section of AppConfig.
type AuthConfig struct {
	JWTRBAC bool `json:"jwt_rbac"`
}

// MetricsConfig is the "metrics" section of AppConfig.
type MetricsConfig struct {
	Prefix string `json:"prefix"`
}
