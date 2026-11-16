package casper

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client is a high–level HTTP client for the Casper vector database.
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	apiKey     string
}

const defaultTimeout = 30 * time.Second

// removePeerTimeout is how long RemovePeer waits: longer than the server's
// own 30 s wait for the removal to commit, so that its 504 reaches the
// caller rather than a client timeout.
const removePeerTimeout = 60 * time.Second

// New creates a new Casper client with a default timeout.
//
// host is a URL including scheme, e.g. "http://localhost".
func New(host string, httpPort int) (*Client, error) {
	return NewWithTimeout(host, httpPort, defaultTimeout)
}

// NewWithTimeout creates a new Casper client with a custom timeout.
func NewWithTimeout(host string, httpPort int, timeout time.Duration) (*Client, error) {
	return NewWithOptions(host, httpPort, Options{Timeout: timeout})
}

// Options configures a client built by NewWithOptions.
type Options struct {
	// Timeout is the per-request timeout; 0 means the default (30s).
	Timeout time.Duration
	// APIKey, when set, is sent as the "api-key" header on every request.
	// Required when the server has API keys configured.
	APIKey string
}

// NewWithOptions creates a new Casper client with the given options.
func NewWithOptions(host string, httpPort int, opts Options) (*Client, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	baseURLStr := fmt.Sprintf("%s:%d", host, httpPort)
	u, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, &Error{
			Kind:    ErrorKindClient,
			Message: "invalid base URL",
			Cause:   err,
		}
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    u,
		apiKey:     opts.APIKey,
	}, nil
}

// BaseURL returns the base HTTP URL used by the client.
func (c *Client) BaseURL() string {
	if c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

// ListCollections calls GET /collections.
func (c *Client) ListCollections(ctx context.Context) (*CollectionsListResponse, error) {
	u, err := c.baseURL.Parse("collections")
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid collections URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	var out CollectionsListResponse
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetCollection calls GET /collections/{name}. Index.State says where
// indexing is; poll it for IndexStateReady after StartIndexing.
func (c *Client) GetCollection(ctx context.Context, collectionName string) (*CollectionInfo, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collections/%s", url.PathEscape(collectionName)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid collection URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	var out CollectionInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateCollection calls POST /collection/{name} with the collection's
// dimension, capacity and the schema and policy of its index, and returns
// the id the collection was created under. No index is built until
// StartIndexing.
func (c *Client) CreateCollection(ctx context.Context, collectionName string, request CreateCollectionRequest) (*CreateCollectionResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s", url.PathEscape(collectionName)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid collection URL", Cause: err}
	}
	buf, err := json.Marshal(request)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to encode collection body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	var out CreateCollectionResponse
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteCollection calls DELETE /collection/{name}.
func (c *Client) DeleteCollection(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid collection URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// Search performs a nearest–neighbour query and decodes the binary search response.
//
// It calls POST /collection/{name}/search?limit=.. with a JSON body { "vector": [...] }.
func (c *Client) Search(ctx context.Context, collectionName string, query []float32, limit int) (SearchResponse, error) {
	return c.SearchWithEf(ctx, collectionName, query, limit, 0)
}

// SearchWithEf is like Search but allows overriding the server-side ef_search
// HNSW parameter. Pass efSearch <= 0 to fall back to the server default.
func (c *Client) SearchWithEf(ctx context.Context, collectionName string, query []float32, limit int, efSearch int) (SearchResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/search", url.PathEscape(collectionName)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid search URL", Cause: err}
	}
	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("output", "bin")
	if efSearch > 0 {
		q.Set("ef_search", fmt.Sprintf("%d", efSearch))
	}
	u.RawQuery = q.Encode()

	body := searchVectorBody{Vector: query}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to encode search body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doScored(req)
}

// Scores scores the query against the vector each of ids holds, as a search
// scores a match, and decodes the binary response. The results are in the
// order of ids; an id the index does not hold is left out, and an id given
// twice is answered twice.
//
// It calls POST /collection/{name}/scores with a JSON body
// { "vector": [...], "ids": [...] }.
func (c *Client) Scores(ctx context.Context, collectionName string, query []float32, ids []uint64) (SearchResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/scores", url.PathEscape(collectionName)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid scores URL", Cause: err}
	}
	q := u.Query()
	q.Set("output", "bin")
	u.RawQuery = q.Encode()

	if ids == nil {
		ids = []uint64{}
	}
	body := scoresBody{Vector: query, IDs: ids}
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to encode scores body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doScored(req)
}

// scoredRecordSize is the size of one binary record: a u64 id and an f32 score.
const scoredRecordSize = 12

// doScored executes a search or scores request sent with output=bin and
// decodes its binary body: a u32 count, then that many (u64 id, f32 score)
// records, little-endian.
func (c *Client) doScored(req *http.Request) (SearchResponse, error) {
	resp, err := c.do(req)
	if err != nil {
		return nil, &Error{Kind: ErrorKindTransport, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Kind: ErrorKindInvalidResponse, Message: "failed to read response body", Cause: err}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errorFromResponse(resp, bodyBytes)
	}

	if len(bodyBytes) < 4 {
		return nil, &Error{
			Kind:    ErrorKindInvalidResponse,
			Message: "binary response too short (missing count)",
		}
	}

	// In uint64 so the product cannot overflow: a u32 count times 12 fits.
	count := uint64(binary.LittleEndian.Uint32(bodyBytes[:4]))
	expectedLen := 4 + count*scoredRecordSize
	if uint64(len(bodyBytes)) != expectedLen {
		return nil, &Error{
			Kind:    ErrorKindInvalidResponse,
			Message: fmt.Sprintf("binary response of %d records should be %d bytes, got %d", count, expectedLen, len(bodyBytes)),
		}
	}

	results := make(SearchResponse, 0, count)
	offset := 4
	for i := uint64(0); i < count; i++ {
		id := binary.LittleEndian.Uint64(bodyBytes[offset : offset+8])
		offset += 8
		scoreBits := binary.LittleEndian.Uint32(bodyBytes[offset : offset+4])
		offset += 4

		score := math.Float32frombits(scoreBits)
		results = append(results, SearchResult{
			ID:    id,
			Score: score,
		})
	}

	return results, nil
}

// GetVector calls GET /collection/{name}/vector/{id} and returns the vector
// exactly as the index stores it (see Components). It returns nil and no
// error when the collection does not hold id; a missing collection or any
// other failure is an error.
func (c *Client) GetVector(ctx context.Context, collectionName string, id uint64) (*GetVectorResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/vector/%d", url.PathEscape(collectionName), id))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid vector URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, &Error{Kind: ErrorKindTransport, Message: "get vector request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, &Error{Kind: ErrorKindInvalidResponse, Message: "failed to read response body", Cause: err}
		}
		// A missing vector is the JSON {"error": "Vector not found", "id": ..};
		// a missing collection is a plain-text 404.
		var notFound struct {
			Error string  `json:"error"`
			ID    *uint64 `json:"id"`
		}
		if err := json.Unmarshal(bodyBytes, &notFound); err == nil && notFound.ID != nil {
			return nil, nil
		}
		return nil, errorFromResponse(resp, bodyBytes)
	}

	var out GetVectorResponse
	if err := decodeJSONResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BatchUpdate calls POST /collection/{name}/update, the one write of
// vectors: upserts insert or replace vectors by id, then deletes remove ids.
func (c *Client) BatchUpdate(ctx context.Context, collectionName string, request BatchUpdateRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/update", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid batch update URL", Cause: err}
	}

	// Ensure we always send both fields as JSON arrays (never null / missing),
	// because the server expects "upserts" and "delete" to be present.
	payload := request
	if payload.Upserts == nil {
		payload.Upserts = []UpsertOperation{}
	}
	if payload.Delete == nil {
		payload.Delete = []uint64{}
	}

	buf, err := json.Marshal(payload)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode batch update body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// StartIndexing calls POST /collection/{name}/index/start and starts
// indexing the collection on the schema it was created with. The server
// returns 202 Accepted and builds in the background; poll
// GetCollection(..).Index.State for IndexStateReady. A repeated call changes
// nothing; after a failed build in standalone, where the state is back to
// IndexStateNotStarted, it starts one again.
func (c *Client) StartIndexing(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index/start", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid index start URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// UpdateIndexPolicy calls PATCH /collection/{name}/index and changes the
// snapshot and compaction policy of the collection's index, before indexing
// starts as well. Only the fields set in patch change.
func (c *Client) UpdateIndexPolicy(ctx context.Context, collectionName string, patch IndexPolicyPatch) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid index URL", Cause: err}
	}
	body := updateIndexBody{Storage: patch}
	buf, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode index policy body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// TriggerSnapshot calls POST /collection/{name}/index/snapshot/trigger and
// snapshots the index on this node now, regardless of the thresholds. The
// server returns 202 Accepted; the snapshot runs asynchronously.
func (c *Client) TriggerSnapshot(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index/snapshot/trigger", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid snapshot trigger URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// TriggerCompaction calls POST /collection/{name}/index/compaction/trigger
// and starts a fold pass on this node now. It folds only the ranges the
// policy's rules hold for. The server returns 202 Accepted; the pass runs
// asynchronously.
func (c *Client) TriggerCompaction(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index/compaction/trigger", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid compaction trigger URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// GetCluster calls GET /cluster and returns the node's role and, in
// cluster mode, its Raft state and the membership. Inspect
// ClusterInfo.Role before reading the cluster-only fields.
func (c *Client) GetCluster(ctx context.Context) (*ClusterInfo, error) {
	u, err := c.baseURL.Parse("cluster")
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid cluster URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	var out ClusterInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RemovePeer calls DELETE /cluster/peer/{node_id} and removes a node from
// the cluster membership: a voter is demoted first, then its entry is
// dropped. It must be sent to the leader; elsewhere it fails with
// ErrorKindNotLeader. A Timeout error means the change is not committed yet
// but goes on; GetCluster shows when it is done.
func (c *Client) RemovePeer(ctx context.Context, nodeID string) (*RemovePeerResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("cluster/peer/%s", url.PathEscape(nodeID)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid peer URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	httpClient := *c.httpClient
	if httpClient.Timeout != 0 && httpClient.Timeout < removePeerTimeout {
		httpClient.Timeout = removePeerTimeout
	}
	var out RemovePeerResponse
	if err := c.doJSONWith(&httpClient, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetTransferLimit calls PUT /cluster/transfer-limit and sets the rate limit
// of index and WAL transfers sent by this node (usually the leader). The
// limit is held in memory only: a restart returns to the configured value.
// 0 turns the limit off.
func (c *Client) SetTransferLimit(ctx context.Context, mbPerSec uint64) error {
	u, err := c.baseURL.Parse("cluster/transfer-limit")
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid transfer limit URL", Cause: err}
	}
	body := transferLimitBody{MBPerSec: mbPerSec}
	buf, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode transfer limit body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// GetConfig calls GET /config and returns the startup configuration the
// node runs with. Configuration is immutable after startup; restart the
// server to change a value.
func (c *Client) GetConfig(ctx context.Context) (*AppConfig, error) {
	u, err := c.baseURL.Parse("config")
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid config URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	var out AppConfig
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMetrics calls GET /metrics and returns the Prometheus metrics in the
// text exposition format, unparsed.
func (c *Client) GetMetrics(ctx context.Context) (string, error) {
	u, err := c.baseURL.Parse("metrics")
	if err != nil {
		return "", &Error{Kind: ErrorKindClient, Message: "invalid metrics URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	resp, err := c.do(req)
	if err != nil {
		return "", &Error{Kind: ErrorKindTransport, Message: "metrics request failed", Cause: err}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", &Error{Kind: ErrorKindInvalidResponse, Message: "failed to read metrics response body", Cause: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errorFromResponse(resp, bodyBytes)
	}
	return string(bodyBytes), nil
}

// Health calls GET /health and returns an error if the service is not healthy.
func (c *Client) Health(ctx context.Context) error {
	u, err := c.baseURL.Parse("health")
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid health URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	resp, err := c.do(req)
	if err != nil {
		return &Error{Kind: ErrorKindTransport, Message: "health request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return errorFromResponse(resp, bodyBytes)
	}
	return nil
}

// Ready calls GET /readyz and returns nil when this node should get reads,
// or an Error with ErrorKindUnavailable when it should not (joining,
// leaderless or stopping; a stopping node says so in Message). With quorum
// set, the node must also count for quorum, as a rolling restart needs.
func (c *Client) Ready(ctx context.Context, quorum bool) error {
	u, err := c.baseURL.Parse("readyz")
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid readyz URL", Cause: err}
	}
	if quorum {
		q := u.Query()
		q.Set("quorum", "true")
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	resp, err := c.do(req)
	if err != nil {
		return &Error{Kind: ErrorKindTransport, Message: "readyz request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	e := errorFromResponse(resp, bodyBytes)
	var notReady struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(bodyBytes, &notReady); err == nil && notReady.Reason != "" {
		e.Message = notReady.Reason
	}
	return e
}

// do sends a request, adding the API key when the client has one.
func (c *Client) do(req *http.Request) (*http.Response, error) {
	return c.doWith(c.httpClient, req)
}

// doWith is do over the given http.Client.
func (c *Client) doWith(httpClient *http.Client, req *http.Request) (*http.Response, error) {
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
	return httpClient.Do(req)
}

// doJSON executes an HTTP request and decodes a JSON response body on success.
func (c *Client) doJSON(req *http.Request, out any) error {
	return c.doJSONWith(c.httpClient, req, out)
}

// doJSONWith is doJSON over the given http.Client.
func (c *Client) doJSONWith(httpClient *http.Client, req *http.Request, out any) error {
	resp, err := c.doWith(httpClient, req)
	if err != nil {
		return &Error{Kind: ErrorKindTransport, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()
	return decodeJSONResponse(resp, out)
}

// doEmpty executes an HTTP request that is expected to return an empty body on success.
func (c *Client) doEmpty(req *http.Request) error {
	resp, err := c.do(req)
	if err != nil {
		return &Error{Kind: ErrorKindTransport, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Drain the body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	return errorFromResponse(resp, bodyBytes)
}

// decodeJSONResponse decodes a JSON body if the status is success, otherwise
// returns a structured Error extracted from the body.
func decodeJSONResponse(resp *http.Response, out any) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return &Error{Kind: ErrorKindInvalidResponse, Message: "failed to read response body", Cause: err}
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.Unmarshal(bodyBytes, out); err != nil {
			return &Error{
				Kind:    ErrorKindInvalidResponse,
				Message: fmt.Sprintf("failed to parse response JSON: %v; body=%s", err, string(bodyBytes)),
				Cause:   err,
			}
		}
		return nil
	}

	return errorFromResponse(resp, bodyBytes)
}
