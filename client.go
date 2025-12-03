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

// Client is a high–level HTTP/gRPC client for Casper vector database.
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
	grpcAddr   string
}

const defaultTimeout = 30 * time.Second

// New creates a new Casper client with a default timeout.
//
// host is a URL including scheme, for example "http://localhost".
func New(host string, httpPort, grpcPort int) (*Client, error) {
	return NewWithTimeout(host, httpPort, grpcPort, defaultTimeout)
}

// NewWithTimeout creates a new Casper client with a custom timeout.
func NewWithTimeout(host string, httpPort, grpcPort int, timeout time.Duration) (*Client, error) {
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

	grpcAddr := fmt.Sprintf("%s:%d", host, grpcPort)

	return &Client{
		httpClient: httpClient,
		baseURL:    u,
		grpcAddr:   grpcAddr,
	}, nil
}

// BaseURL returns the base HTTP URL used by the client.
func (c *Client) BaseURL() string {
	if c.baseURL == nil {
		return ""
	}
	return c.baseURL.String()
}

// GRPCAddr returns the gRPC address used by the client.
func (c *Client) GRPCAddr() string {
	return c.grpcAddr
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

// GetCollection calls GET /collection/{name}.
func (c *Client) GetCollection(ctx context.Context, collectionName string) (*CollectionInfo, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s", url.PathEscape(collectionName)))
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

// CreateCollection calls POST /collection/{name}?dim=..&max_size=..
func (c *Client) CreateCollection(ctx context.Context, collectionName string, reqParams CreateCollectionRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid collection URL", Cause: err}
	}

	q := u.Query()
	q.Set("dim", fmt.Sprintf("%d", reqParams.Dim))
	q.Set("max_size", fmt.Sprintf("%d", reqParams.MaxSize))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
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

// InsertVector calls POST /collection/{name}/insert?id=..
func (c *Client) InsertVector(ctx context.Context, collectionName string, request InsertRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/insert", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid insert URL", Cause: err}
	}
	q := u.Query()
	q.Set("id", fmt.Sprintf("%d", request.ID))
	u.RawQuery = q.Encode()

	body := insertVectorBody{Vector: request.Vector}
	buf, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode insert body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// DeleteVector calls DELETE /collection/{name}/delete?id=..
func (c *Client) DeleteVector(ctx context.Context, collectionName string, request DeleteRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/delete", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid delete URL", Cause: err}
	}
	q := u.Query()
	q.Set("id", fmt.Sprintf("%d", request.ID))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// Search performs a nearest–neighbour query and decodes the binary search response.
//
// It calls POST /collection/{name}/search?limit=.. with a JSON body { "vector": [...] }
// and expects the default binary response format described in api.md.
func (c *Client) Search(ctx context.Context, collectionName string, query []float32, limit int) (SearchResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/search", url.PathEscape(collectionName)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid search URL", Cause: err}
	}
	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("output", "bin")
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "search request failed", Cause: err}
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Kind: ErrorKindInvalidResponse, Message: "failed to read search response body", Cause: err}
	}

	if status < 200 || status >= 300 {
		// Attempt to extract "error" field from JSON, fall back to raw body.
		var jsonErr struct {
			Error string `json:"error"`
		}
		msg := string(bodyBytes)
		if err := json.Unmarshal(bodyBytes, &jsonErr); err == nil && jsonErr.Error != "" {
			msg = jsonErr.Error
		}
		return nil, classifyHTTPError(status, msg)
	}

	// Binary format: u32 count, then for each result: u32 id, f32 score.
	if len(bodyBytes) < 4 {
		return nil, &Error{
			Kind:    ErrorKindInvalidResponse,
			Message: "binary search response too short (missing count)",
		}
	}

	count := int(binary.LittleEndian.Uint32(bodyBytes[:4]))
	expectedLen := 4 + count*(4+4)
	if len(bodyBytes) < expectedLen {
		return nil, &Error{
			Kind:    ErrorKindInvalidResponse,
			Message: fmt.Sprintf("binary search response truncated: expected at least %d bytes, got %d", expectedLen, len(bodyBytes)),
		}
	}

	results := make(SearchResponse, 0, count)
	offset := 4
	for i := 0; i < count; i++ {
		if offset+8 > len(bodyBytes) {
			return nil, &Error{
				Kind:    ErrorKindInvalidResponse,
				Message: "binary search response truncated while reading results",
			}
		}
		id := binary.LittleEndian.Uint32(bodyBytes[offset : offset+4])
		offset += 4
		scoreBits := binary.LittleEndian.Uint32(bodyBytes[offset : offset+4])
		offset += 4

		score := float32FromBits(scoreBits)
		results = append(results, SearchResult{
			ID:    id,
			Score: score,
		})
	}

	return results, nil
}

// GetVector calls GET /collection/{name}/vector/{id}.
func (c *Client) GetVector(ctx context.Context, collectionName string, id uint32) (*GetVectorResponse, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/vector/%d", url.PathEscape(collectionName), id))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid vector URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "get vector request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, classifyHTTPError(resp.StatusCode, "vector not found")
	}

	var out GetVectorResponse
	if err := decodeJSONResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// BatchUpdate calls POST /collection/{name}/update.
func (c *Client) BatchUpdate(ctx context.Context, collectionName string, request BatchUpdateRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/update", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid batch update URL", Cause: err}
	}

	// Ensure we always send both fields as JSON arrays (never null / missing),
	// because the server expects "insert" and "delete" to be present.
	payload := struct {
		Insert []BatchInsertOperation `json:"insert"`
		Delete []uint32               `json:"delete"`
	}{
		Insert: request.Insert,
		Delete: request.Delete,
	}
	if payload.Insert == nil {
		payload.Insert = []BatchInsertOperation{}
	}
	if payload.Delete == nil {
		payload.Delete = []uint32{}
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

// CreateHNSWIndex calls POST /collection/{name}/index.
func (c *Client) CreateHNSWIndex(ctx context.Context, collectionName string, index HNSWIndex) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid index URL", Cause: err}
	}
	buf, err := json.Marshal(index)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode index body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// DeleteIndex calls DELETE /collection/{name}/index.
func (c *Client) DeleteIndex(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/index", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid index URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// MuteCollection calls POST /collection/{name}/mute.
func (c *Client) MuteCollection(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/mute", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid mute URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// UnmuteCollection calls POST /collection/{name}/unmute.
func (c *Client) UnmuteCollection(ctx context.Context, collectionName string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/unmute", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid unmute URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	return c.doEmpty(req)
}

// DeleteMatrix calls DELETE /matrix/{name}.
func (c *Client) DeleteMatrix(ctx context.Context, name string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("matrix/%s", url.PathEscape(name)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid matrix URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doEmpty(req)
}

// ListMatrices calls GET /matrix/list.
func (c *Client) ListMatrices(ctx context.Context) ([]MatrixInfo, error) {
	u, err := c.baseURL.Parse("matrix/list")
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid matrix list URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	var out []MatrixInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetMatrixInfo calls GET /matrix/{name}.
func (c *Client) GetMatrixInfo(ctx context.Context, name string) (*MatrixInfo, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("matrix/%s", url.PathEscape(name)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid matrix URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	var out MatrixInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreatePQ calls POST /pq/{name}.
func (c *Client) CreatePQ(ctx context.Context, name string, request CreatePQRequest) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("pq/%s", url.PathEscape(name)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid PQ URL", Cause: err}
	}
	buf, err := json.Marshal(request)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode PQ body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// DeletePQ calls DELETE /pq/{name}.
func (c *Client) DeletePQ(ctx context.Context, name string) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("pq/%s", url.PathEscape(name)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid PQ URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// ListPQs calls GET /pq/list.
func (c *Client) ListPQs(ctx context.Context) ([]PQInfo, error) {
	u, err := c.baseURL.Parse("pq/list")
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid PQ list URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	var out []PQInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPQ calls GET /pq/{name}.
func (c *Client) GetPQ(ctx context.Context, name string) (*PQInfo, error) {
	u, err := c.baseURL.Parse(fmt.Sprintf("pq/%s", url.PathEscape(name)))
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "invalid PQ URL", Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	var out PQInfo
	if err := c.doJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "health request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return classifyHTTPError(resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// float32FromBits converts a uint32 into a float32.
func float32FromBits(b uint32) float32 {
	return math.Float32frombits(b)
}

// doJSON executes an HTTP request and decodes a JSON response body on success.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()
	return decodeJSONResponse(resp, out)
}

// doEmpty executes an HTTP request that is expected to return an empty body on success.
func (c *Client) doEmpty(req *http.Request) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "request failed", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	// Try to parse JSON {"error": "..."} first
	var jsonErr struct {
		Error string `json:"error"`
	}
	msg := string(bodyBytes)
	if err := json.Unmarshal(bodyBytes, &jsonErr); err == nil && jsonErr.Error != "" {
		msg = jsonErr.Error
	}
	return classifyHTTPError(resp.StatusCode, msg)
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

	// Try to parse JSON error { "error": "..." }.
	var jsonErr struct {
		Error string `json:"error"`
	}
	msg := string(bodyBytes)
	if err := json.Unmarshal(bodyBytes, &jsonErr); err == nil && jsonErr.Error != "" {
		msg = jsonErr.Error
	}
	return classifyHTTPError(resp.StatusCode, msg)
}
