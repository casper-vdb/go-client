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
}

const defaultTimeout = 30 * time.Second

// New creates a new Casper client with a default timeout.
//
// host is a URL including scheme, e.g. "http://localhost".
func New(host string, httpPort int) (*Client, error) {
	return NewWithTimeout(host, httpPort, defaultTimeout)
}

// NewWithTimeout creates a new Casper client with a custom timeout.
func NewWithTimeout(host string, httpPort int, timeout time.Duration) (*Client, error) {
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

// UpsertVectors calls PUT /collection/{name}/vectors with a batch of vector points.
func (c *Client) UpsertVectors(ctx context.Context, collectionName string, vectors []VectorPoint) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/vectors", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid upsert URL", Cause: err}
	}

	if vectors == nil {
		vectors = []VectorPoint{}
	}
	body := UpsertVectorsRequest{Vectors: vectors}
	buf, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode upsert body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doEmpty(req)
}

// DeleteVectors calls POST /collection/{name}/vectors/delete with a list of ids.
func (c *Client) DeleteVectors(ctx context.Context, collectionName string, ids []uint32) error {
	u, err := c.baseURL.Parse(fmt.Sprintf("collection/%s/vectors/delete", url.PathEscape(collectionName)))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "invalid delete URL", Cause: err}
	}

	if ids == nil {
		ids = []uint32{}
	}
	body := DeleteVectorsRequest{Vectors: ids}
	buf, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to encode delete body", Cause: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return &Error{Kind: ErrorKindClient, Message: "failed to build request", Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")

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
	expectedLen := 4 + count*8
	if len(bodyBytes) < expectedLen {
		return nil, &Error{
			Kind:    ErrorKindInvalidResponse,
			Message: fmt.Sprintf("binary search response truncated: expected at least %d bytes, got %d", expectedLen, len(bodyBytes)),
		}
	}

	results := make(SearchResponse, 0, count)
	offset := 4
	for i := 0; i < count; i++ {
		id := binary.LittleEndian.Uint32(bodyBytes[offset : offset+4])
		offset += 4
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

// CreateHNSWIndex calls POST /collection/{name}/index. The server returns
// 202 Accepted as soon as the build has been scheduled — the index build
// itself runs asynchronously, so poll GetCollection.HasIndex when you need
// to wait for completion.
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

	var jsonErr struct {
		Error string `json:"error"`
	}
	msg := string(bodyBytes)
	if err := json.Unmarshal(bodyBytes, &jsonErr); err == nil && jsonErr.Error != "" {
		msg = jsonErr.Error
	}
	return classifyHTTPError(resp.StatusCode, msg)
}
