package casper

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
