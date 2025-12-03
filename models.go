package casper

// InsertRequest describes parameters required to insert or upsert a single vector.
type InsertRequest struct {
	ID     uint32
	Vector []float32
}

// DeleteRequest describes parameters required to delete a single vector by ID.
type DeleteRequest struct {
	ID uint32
}

// searchVectorBody is used as JSON body for /search requests.
type searchVectorBody struct {
	Vector []float32 `json:"vector"`
}

// insertVectorBody is used as JSON body for /insert requests.
type insertVectorBody struct {
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
	Index     *IndexInfo `json:"index,omitempty"`
}

// IndexInfo contains metadata about a collection index.
type IndexInfo struct {
	IndexType      string `json:"type"`
	Metric         string `json:"metric"`
	Quantization   string `json:"quantization"`
	Normalization  bool   `json:"normalization"`
	MaxElements    *int   `json:"max_elements,omitempty"`
	M              *int   `json:"m,omitempty"`
	M0             *int   `json:"m0,omitempty"`
	EfConstruction *int   `json:"ef_construction,omitempty"`
	Ef             *int   `json:"ef,omitempty"`
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
	// PQName is optional and only applicable when using PQ quantization (e.g. "pq8").
	PQName *string `json:"pq_name,omitempty"`
}

// GetVectorResponse mirrors the JSON returned from /collection/{name}/vector/{id}.
type GetVectorResponse struct {
	ID     uint32    `json:"id"`
	Vector []float32 `json:"vector"`
}

// MatrixInfo describes a matrix entry returned by /matrix APIs.
type MatrixInfo struct {
	Name    string `json:"name"`
	Dim     int    `json:"dim"`
	Len     int    `json:"len"`
	Enabled bool   `json:"enabled"`
}

// UploadMatrixResult contains a summary of a successful gRPC matrix upload.
type UploadMatrixResult struct {
	Success      bool
	Message      string
	TotalVectors uint32
	TotalChunks  uint32
}

// CreatePQRequest describes a PQ configuration.
type CreatePQRequest struct {
	Dim       int      `json:"dim"`
	Codebooks []string `json:"codebooks"`
}

// PQInfo describes a PQ configuration as returned from /pq APIs.
type PQInfo struct {
	Name      string   `json:"name"`
	Dim       int      `json:"dim"`
	Codebooks []string `json:"codebooks"`
	Enabled   bool     `json:"enabled"`
}
