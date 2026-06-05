package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	anthropicModel   = "claude-haiku-4-5"
)

// suggestCategories asks Claude to (a) propose 1-3 categories for a printed
// item, reusing the user's existing categories where they fit, and (b) extract
// the brands/products mentioned and attach a link to each. Results are stored
// on the item: categories await the user's confirmation; links are shown
// immediately. It is a no-op (logged) when ANTHROPIC_API_KEY is unset, so the
// app works fully without AI — the user just classifies manually.
func (api *API) suggestCategories(itemID uint, text string) {
	apiKey := getEnv("ANTHROPIC_API_KEY", "")
	if apiKey == "" {
		return
	}

	known, err := api.AllCategoryNames()
	if err != nil {
		log.Printf("[classify] could not load categories: %v", err)
	}

	categories, links, err := requestAI(apiKey, text, known)
	if err != nil {
		log.Printf("[classify] AI failed for item %d: %v", itemID, err)
		return
	}
	if len(categories) == 0 && len(links) == 0 {
		return
	}
	if err := api.SetAIResults(itemID, categories, links); err != nil {
		log.Printf("[classify] could not save AI results for item %d: %v", itemID, err)
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// aiResult is the JSON shape we ask the model to return.
type aiResult struct {
	Categories []string `json:"categories"`
	Links      []Link   `json:"links"`
}

// requestAI performs the Anthropic call and parses the model's JSON reply into
// categories and links.
func requestAI(apiKey, text string, known []string) ([]string, []Link, error) {
	knownStr := "(none yet)"
	if len(known) > 0 {
		knownStr = strings.Join(known, ", ")
	}

	prompt := fmt.Sprintf(`You process short notes that someone printed on a receipt printer. A note may list one or more things (products, brands, drinks, materials, etc.).

The note text is:
"""
%s
"""

The user's existing categories are: %s

Do two things:

1. categories: 0 to 3 short, lowercase category labels describing what the note
   is about. Rules:
   - Be SPECIFIC. Choose the most precise category, never a broad catch-all.
       "Arran", "Aberlour", "Lagavulin" (whisky distilleries/bottles) -> ["scotch"]   (NOT "drinks" or "alcohol")
       cocktail recipes / bar ingredients, or references to cocktail books
       (PDT, NOMAD, D&C / Death & Co) -> ["cocktails"]. A bottled spirit may carry
       its own category too, e.g. ["liqueur","cocktails"] or ["scotch","cocktails"].
       "Robi decking", deck boards/screws -> ["decking"]                               (NOT "hardware" or "home")
       "seedwell trays", seed-starting supplies -> ["seed starting","garden"]          (NOT "home")
       a bow sight -> ["hunting","bow"]
   - NEVER use vague buckets like: misc, other, stuff, general, home, hardware,
     drinks, alcohol, software, tech, food.
   - Reuse an existing category when it is the correct specific one; only invent a
     new specific category when none fit.
   - A note may have several specific categories.
   - If you cannot confidently tell what the note refers to, return an EMPTY
     categories list — it is better to leave it unclassified than to guess. For
     example an unfamiliar token like "Goosekey.baby" with no clear meaning -> [].

2. links: identify each distinct brand/product/company mentioned in the note and
   give a link to its official website. For each, output {"label","url"} where
   label is the human-friendly name (e.g. "Aberlour Scotch") and url is the
   official homepage if you are confident of it (e.g. "https://www.aberlour.com").
   If you are NOT confident of the exact official URL, set url to a Google search
   instead: "https://www.google.com/search?q=" followed by the URL-encoded query.
   Example: a note "Arran and Aberlour" -> two links; "Robi decking" -> one link.

Reply with ONLY a JSON object, nothing else, of the form:
{"categories":["..."],"links":[{"label":"...","url":"..."}]}`,
		text, knownStr)

	reqBody, err := json.Marshal(anthropicRequest{
		Model:     anthropicModel,
		MaxTokens: 400,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodPost, anthropicURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("decode response: %w (body: %s)", err, string(body))
	}
	if parsed.Error != nil {
		return nil, nil, fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return nil, nil, fmt.Errorf("empty response")
	}

	cats, links := parseAIResult(parsed.Content[0].Text)
	cats = applyCategoryRules(text, cats)
	return cats, links, nil
}

// categoryKeywordRules force a category onto any note whose text contains one of
// the keywords, regardless of what the AI returned. Cocktail-book references and
// the literal word "cocktail" always file under "cocktails" — in addition to any
// other categories (e.g. liqueur, scotch).
var categoryKeywordRules = []struct {
	keywords []string
	category string
}{
	{[]string{"cocktail", "nomad", "pdt", "d&c"}, "cocktails"},
}

func applyCategoryRules(text string, cats []string) []string {
	lower := strings.ToLower(text)
	have := map[string]bool{}
	for _, c := range cats {
		have[strings.ToLower(c)] = true
	}
	for _, rule := range categoryKeywordRules {
		if have[rule.category] {
			continue
		}
		for _, kw := range rule.keywords {
			if strings.Contains(lower, kw) {
				cats = append(cats, rule.category)
				have[rule.category] = true
				break
			}
		}
	}
	return cats
}

// parseAIResult extracts the JSON object from the model's reply, tolerating
// surrounding prose or code fences, and returns cleaned categories and links.
func parseAIResult(s string) ([]string, []Link) {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return nil, nil
	}
	var r aiResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &r); err != nil {
		return nil, nil
	}

	cats := make([]string, 0, len(r.Categories))
	for _, c := range r.Categories {
		if c = strings.TrimSpace(c); c != "" {
			cats = append(cats, c)
		}
	}

	links := make([]Link, 0, len(r.Links))
	for _, l := range r.Links {
		l.Label = strings.TrimSpace(l.Label)
		l.URL = strings.TrimSpace(l.URL)
		// Only keep links with a label and an http(s) URL.
		if l.Label == "" || !strings.HasPrefix(l.URL, "http") {
			continue
		}
		links = append(links, l)
	}
	return cats, links
}
