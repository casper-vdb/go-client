package casper

import "encoding/json"

// VectorPoint is a single (id, vector) pair used in upsert and batch operations.
type VectorPoint struct {
	ID     uint32    `json:"id"`
	Vector []float32 `json:"vector"`
}

// UpsertVectorsRequest is the body of PUT /collection/{name}/vectors.
type UpsertVectorsRequest struct {
	Vectors []VectorPoint `json:"vectors"`
}

// DeleteVectorsRequest is the body of POST /collection/{name}/vectors/delete.
type DeleteVectorsRequest struct {
	Vectors []uint32 `json:"vectors"`
}

// searchVectorBody is used as the JSON body for /search requests.
type searchVectorBody struct {
	Vector []float32 `json:"vector"`
}

// SearchResult is a single nearest–neighbour hit.
type SearchResult struct {
	ID    uint32  `json:"id"`
	Score float32 `json:"score"`
}

// SearchResponse is a convenience alias for a slice of results.
type SearchResponse []SearchResult

// CreateCollectionRequest describes parameters for a new collection.
type CreateCollectionRequest struct {
	Dim     int
	MaxSize uint32
}

// CollectionsListResponse mirrors the JSON returned from /collections.
type CollectionsListResponse struct {
	Collections []CollectionInfo `json:"collections"`
}

// CollectionInfo contains metadata about a single collection.
type CollectionInfo struct {
	Name      string     `json:"name"`
	Dimension int        `json:"dimension"`
	Mutable   bool       `json:"mutable"`
	HasIndex  bool       `json:"has_index"`
	MaxSize   uint32     `json:"max_size"`
	Size      uint32     `json:"size"`
	Status    string     `json:"status"`
	Index     *IndexInfo `json:"index,omitempty"`

	// --- Progress / replication cursors (Casper v0.1.0+) -----------------
	// WAL head: highest durably-fsynced data WAL seq + 1.
	CurrentSeq uint64 `json:"current_seq"`
	// Highest WAL seq applied to the in-memory HNSW. CurrentSeq -
	// AppliedSeq is the applier backlog.
	AppliedSeq uint64 `json:"applied_seq"`
	// WAL seq covered by the on-disk snapshot, if any. Nil until the first
	// snapshot is taken; the server omits the field then.
	IndexSeq *uint64 `json:"index_seq,omitempty"`
	// Per-collection catalog head (catalog WAL seq + 1).
	CatalogSeq uint64 `json:"catalog_seq"`
}

// IndexInfo contains metadata about a collection's index.
type IndexInfo struct {
	HNSW          *HnswIndexInfo `json:"hnsw,omitempty"`
	Normalization bool           `json:"normalization"`
}

// HnswIndexInfo describes a configured HNSW index returned by GetCollection.
type HnswIndexInfo struct {
	Metric         string `json:"metric"`
	Quantization   string `json:"quantization"`
	M              int    `json:"m"`
	M0             int    `json:"m0"`
	EfConstruction int    `json:"ef_construction"`
}

// BatchInsertOperation represents a single insert in a batch update.
type BatchInsertOperation struct {
	ID     uint32    `json:"id"`
	Vector []float32 `json:"vector"`
}

// BatchUpdateRequest wraps a batch of insert and delete operations.
type BatchUpdateRequest struct {
	Insert []BatchInsertOperation `json:"insert"`
	Delete []uint32               `json:"delete"`
}

// HNSWIndex describes an HNSW index creation request.
type HNSWIndex struct {
	HNSW          HNSW  `json:"hnsw"`
	Normalization *bool `json:"normalization,omitempty"`
}

// HNSW contains configuration options for HNSW.
type HNSW struct {
	Metric         string `json:"metric"`
	Quantization   string `json:"quantization"`
	M              int    `json:"m"`
	M0             int    `json:"m0"`
	EfConstruction int    `json:"ef_construction"`
}

// GetVectorResponse mirrors the JSON returned from /collection/{name}/vector/{id}.
type GetVectorResponse struct {
	ID     uint32    `json:"id"`
	Vector []float32 `json:"vector"`
}

// ============================================================================
//  Cluster — GET /cluster
// ============================================================================

// MasterCollectionInfo is per-collection progress reported by a master for
// one of its local collections.
type MasterCollectionInfo struct {
	Name       string `json:"name"`
	CatalogSeq uint64 `json:"catalog_seq"`
	CurrentSeq uint64 `json:"current_seq"`
	AppliedSeq uint64 `json:"applied_seq"`
	// IndexSeq is nil until a snapshot has been taken.
	IndexSeq *uint64 `json:"index_seq,omitempty"`
}

// ReplicaCollectionInfo is per-collection progress reported by a replica for
// one of the collections it tracks.
type ReplicaCollectionInfo struct {
	Name              string `json:"name"`
	AppliedCatalogSeq uint64 `json:"applied_catalog_seq"`
	AppliedDataSeq    uint64 `json:"applied_data_seq"`
}

// ReplicaCursorInfo is one connected replica's last-known cursors, as
// reported by ACKs to the master.
type ReplicaCursorInfo struct {
	ReplicaID               string                  `json:"replica_id"`
	AppliedGlobalCatalogSeq uint64                  `json:"applied_global_catalog_seq"`
	Collections             []ReplicaCollectionInfo `json:"collections"`
}

// ClusterRole identifies the node's role in the cluster.
type ClusterRole string

const (
	ClusterRoleStandalone ClusterRole = "standalone"
	ClusterRoleMaster     ClusterRole = "master"
	ClusterRoleReplica    ClusterRole = "replica"
)

// ClusterInfo is the response payload for GET /cluster. The server tags the
// payload with "role"; the irrelevant fields stay at their zero values for
// the other roles. Always inspect Role before reading role-specific fields.
type ClusterInfo struct {
	Role   ClusterRole `json:"role"`
	NodeID string      `json:"node_id"`

	// master-only ----------------------------------------------------------
	GlobalCatalogSeq  uint64                 `json:"global_catalog_seq,omitempty"`
	MasterCollections []MasterCollectionInfo `json:"-"`
	Replicas          []ReplicaCursorInfo    `json:"replicas,omitempty"`

	// replica-only ---------------------------------------------------------
	MasterAddr              string                  `json:"master_addr,omitempty"`
	AppliedGlobalCatalogSeq uint64                  `json:"applied_global_catalog_seq,omitempty"`
	ReplicaCollections      []ReplicaCollectionInfo `json:"-"`

	// Both master and replica use the field name "collections" but with
	// different element shapes. We capture the raw bytes and decode into
	// the right slice based on Role in UnmarshalJSON.
}

// UnmarshalJSON dispatches the polymorphic "collections" field based on Role.
func (c *ClusterInfo) UnmarshalJSON(data []byte) error {
	// First decode the shared part + raw collections.
	type shared struct {
		Role                    ClusterRole         `json:"role"`
		NodeID                  string              `json:"node_id"`
		GlobalCatalogSeq        uint64              `json:"global_catalog_seq"`
		Replicas                []ReplicaCursorInfo `json:"replicas"`
		MasterAddr              string              `json:"master_addr"`
		AppliedGlobalCatalogSeq uint64              `json:"applied_global_catalog_seq"`
		Collections             json.RawMessage     `json:"collections"`
	}
	var s shared
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	c.Role = s.Role
	c.NodeID = s.NodeID
	c.GlobalCatalogSeq = s.GlobalCatalogSeq
	c.Replicas = s.Replicas
	c.MasterAddr = s.MasterAddr
	c.AppliedGlobalCatalogSeq = s.AppliedGlobalCatalogSeq

	if len(s.Collections) == 0 || string(s.Collections) == "null" {
		return nil
	}
	switch s.Role {
	case ClusterRoleMaster:
		return json.Unmarshal(s.Collections, &c.MasterCollections)
	case ClusterRoleReplica:
		return json.Unmarshal(s.Collections, &c.ReplicaCollections)
	default:
		return nil
	}
}

// ============================================================================
//  Configuration — GET /config
// ============================================================================

// AppConfig mirrors the runtime configuration the server process started
// with, returned from GET /config. Immutable after startup; restart the
// server to change a value.
type AppConfig struct {
	Host                 string      `json:"host"`
	HTTPPort             uint16      `json:"http_port"`
	StorageDir           string      `json:"storage_dir"`
	ClusterRole          ClusterRole `json:"cluster_role"`
	MasterAddr           *string     `json:"master_addr,omitempty"`
	GRPCPort             uint16      `json:"grpc_port"`
	SnapshotEveryWrites  uint64      `json:"snapshot_every_writes"`
	SnapshotEverySeconds uint64      `json:"snapshot_every_seconds"`
	SnapshotEnabled      bool        `json:"snapshot_enabled"`
	SyncReplicas         uint        `json:"sync_replicas"`
	WriteAckTimeoutMS    uint64      `json:"write_ack_timeout_ms"`
}
