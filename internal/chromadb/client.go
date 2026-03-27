// Package chromadb provides a Go client for the Chroma vector database HTTP API.
package chromadb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a local Chroma HTTP server.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Chroma client for the given base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Ping checks if the Chroma server is reachable.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/v2/heartbeat")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chroma ping: status %d", resp.StatusCode)
	}
	return nil
}

// Document is a single item to embed.
type Document struct {
	ID       string            `json:"id"`
	Content  string            `json:"document"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// QueryResult is a single search result from Chroma.
type QueryResult struct {
	ID       string
	Content  string
	Metadata map[string]string
	Distance float64
}

// EnsureCollection creates a collection if it doesn't exist.
func (c *Client) EnsureCollection(name string) error {
	body := map[string]any{"name": name, "get_or_create": true}
	data, _ := json.Marshal(body)
	resp, err := c.httpClient.Post(c.baseURL+"/api/v2/collections", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ensure collection %q: status %d: %s", name, resp.StatusCode, b)
	}
	return nil
}

// Upsert adds or updates documents in a collection.
func (c *Client) Upsert(collection string, docs []Document) error {
	ids := make([]string, len(docs))
	documents := make([]string, len(docs))
	metadatas := make([]map[string]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
		documents[i] = d.Content
		metadatas[i] = d.Metadata
	}
	body := map[string]any{
		"ids":       ids,
		"documents": documents,
		"metadatas": metadatas,
	}
	data, _ := json.Marshal(body)
	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v2/collections/"+collection+"/upsert",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert %q: status %d: %s", collection, resp.StatusCode, b)
	}
	return nil
}

// Query searches a collection for the top-k most similar documents.
func (c *Client) Query(collection, text string, topK int) ([]QueryResult, error) {
	body := map[string]any{
		"query_texts": []string{text},
		"n_results":   topK,
		"include":     []string{"documents", "metadatas", "distances"},
	}
	data, _ := json.Marshal(body)
	resp, err := c.httpClient.Post(
		c.baseURL+"/api/v2/collections/"+collection+"/query",
		"application/json",
		bytes.NewReader(data),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query %q: status %d: %s", collection, resp.StatusCode, b)
	}
	var result struct {
		IDs       [][]string            `json:"ids"`
		Documents [][]string            `json:"documents"`
		Metadatas [][]map[string]string `json:"metadatas"`
		Distances [][]float64           `json:"distances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.IDs) == 0 {
		return nil, nil
	}
	out := make([]QueryResult, len(result.IDs[0]))
	for i := range result.IDs[0] {
		out[i] = QueryResult{
			ID:       result.IDs[0][i],
			Content:  result.Documents[0][i],
			Metadata: result.Metadatas[0][i],
			Distance: result.Distances[0][i],
		}
	}
	return out, nil
}
