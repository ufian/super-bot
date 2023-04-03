package bot

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// Summarizer is a helper for summarizing articles by links
type Summarizer struct {
	OpenAISummary openAISummary

	UrAPI    string
	UrToken  string
	URClient *http.Client
	cache    SummarizerCache

	debug bool
}

type openAISummary interface {
	Summary(text string) (response string, err error)
}

// NewSummarizer creates new summarizer object
// If debug is true, it loads cache from file
func NewSummarizer(openAISummary openAISummary, urAPI, urToken string, urClient *http.Client, debug bool) Summarizer {
	var cache SummarizerCache
	if debug {
		cache = loadCache()
	} else {
		cache = defaultCache()
	}

	return Summarizer{
		OpenAISummary: openAISummary,
		UrAPI:         urAPI,
		UrToken:       urToken,
		URClient:      urClient,
		cache:         cache,
		debug:         debug,
	}
}

// Summary returns summary for link
// It uses cache for links that was already summarized
// If debug is true, it saves cache to file
// Important: this isn't thread safe
func (s Summarizer) Summary(link string) (summary string, err error) {
	_, hasLink := s.cache.Summaries[link]
	if hasLink {
		log.Printf("[DEBUG] summary for link loaded by cache: %s", link)
		return s.cache.Summaries[link].Render(), nil
	}

	item, err := s.summaryInternal(link)
	if err != nil || item.IsEmpty() {
		return "", err
	}

	s.cache.Summaries[link] = item
	if s.debug {
		log.Printf("[DEBUG] summary for link saved in cache: %s", link)
		if err := s.cache.save(); err != nil {
			log.Printf("[DEBUG] Cache saving problem: %v", err)
		}
	}
	return item.Render(), nil
}

func (s Summarizer) summaryInternal(link string) (item summaryItem, err error) {
	log.Printf("[DEBUG] summary for link:%s", link)

	rl := fmt.Sprintf("%s?token=%s&url=%s", s.UrAPI, s.UrToken, link)
	resp, err := s.URClient.Get(rl)
	if err != nil {
		return summaryItem{}, fmt.Errorf("can't get summary for %s: %w", link, err)
	}
	defer resp.Body.Close() // nolint
	if resp.StatusCode != http.StatusOK {
		return summaryItem{}, fmt.Errorf("can't get summary for %s: %d", link, resp.StatusCode)
	}

	urResp := struct {
		Title   string `json:"Title"`
		Content string `json:"Content"`
	}{}
	if decErr := json.NewDecoder(resp.Body).Decode(&urResp); decErr != nil {
		return summaryItem{}, fmt.Errorf("can't decode summary for %s: %w", link, decErr)
	}

	res, err := s.OpenAISummary.Summary(urResp.Title + " - " + urResp.Content)
	if err != nil {
		return summaryItem{}, fmt.Errorf("can't get summary for %s: %w", link, err)
	}

	result := summaryItem{
		Title:   urResp.Title,
		Content: res,
	}

	return result, nil
}

type summaryItem struct {
	Title   string `json:"Title"`
	Content string `json:"Content"`
}

// Render telegram message
func (s summaryItem) Render() (title string) {
	return s.Title + "\n\n" + EscapeMarkDownV1Text(s.Content)
}

func (s summaryItem) IsEmpty() bool {
	return s.Title == "" || s.Content == ""
}

// SummarizerCache is a memory cache for OpenAI summaries
// Stored map of link -> summary
type SummarizerCache struct {
	Summaries map[string]summaryItem `json:"Summaries"`
}

func (c SummarizerCache) save() error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile("cache_openai.json", data, 0o600)
}

func loadCache() SummarizerCache {
	cache := defaultCache()

	data, err := os.ReadFile("cache_openai.json")
	if err != nil {
		log.Printf("[WARN] can't open cache file, %v", err)
		return cache
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Printf("[WARN] can't unmarshal cache file, %v", err)
		return cache
	}

	return cache
}

func defaultCache() SummarizerCache {
	return SummarizerCache{
		Summaries: make(map[string]summaryItem),
	}
}
