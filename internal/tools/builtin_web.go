package tools

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type webSearchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Topics   []ddgTopic `json:"Topics"`
}

type ddgResponse struct {
	Heading      string     `json:"Heading"`
	AbstractText string     `json:"AbstractText"`
	AbstractURL  string     `json:"AbstractURL"`
	Related      []ddgTopic `json:"RelatedTopics"`
}

func (e *Executor) webSearch(raw json.RawMessage) (map[string]any, error) {
	var in webSearchArgs
	if err := decodeArgs(raw, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, errors.New("query is required")
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 5
	}

	endpoint := "https://api.duckduckgo.com/?format=json&no_html=1&no_redirect=1&q=" + url.QueryEscape(in.Query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, errors.New(strings.TrimSpace(string(body)))
	}

	var parsed ddgResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	results := []map[string]any{}
	if strings.TrimSpace(parsed.AbstractText) != "" && strings.TrimSpace(parsed.AbstractURL) != "" {
		results = append(results, map[string]any{
			"title":        firstNonEmpty(parsed.Heading, in.Query),
			"snippet":      parsed.AbstractText,
			"source_url":   parsed.AbstractURL,
			"retrieved_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
	for _, topic := range flattenTopics(parsed.Related) {
		if len(results) >= in.MaxResults {
			break
		}
		if strings.TrimSpace(topic.Text) == "" || strings.TrimSpace(topic.FirstURL) == "" {
			continue
		}
		results = append(results, map[string]any{
			"title":        topic.Text,
			"snippet":      topic.Text,
			"source_url":   topic.FirstURL,
			"retrieved_at": time.Now().UTC().Format(time.RFC3339Nano),
		})
	}

	return map[string]any{
		"query":   in.Query,
		"count":   len(results),
		"results": results,
	}, nil
}

func flattenTopics(in []ddgTopic) []ddgTopic {
	out := []ddgTopic{}
	for _, topic := range in {
		if len(topic.Topics) > 0 {
			out = append(out, flattenTopics(topic.Topics)...)
			continue
		}
		out = append(out, topic)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
