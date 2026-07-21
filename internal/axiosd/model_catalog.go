package axiosd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHuggingFaceModelsURL = "https://huggingface.co/api/models"
	modelCatalogCacheTTL        = 10 * time.Minute
	modelCatalogMaxResponse     = 8 << 20
	modelCatalogDefaultLimit    = 20
	modelCatalogMaxLimit        = 50
)

var (
	huggingFaceModelIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	quantizationPattern       = regexp.MustCompile(`(?i)(?:^|[-_.])(IQ[1-4](?:_[A-Z0-9]+)+|Q[2-8](?:_[A-Z0-9]+)+|BF16|F16|F32)(?:[-_.]|$)`)
	parameterPattern          = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)[-_ ]?([BM])(?:[-_]|$)`)
)

type modelCatalogCacheEntry struct {
	models    []MarketplaceModel
	expiresAt time.Time
}

type huggingFaceModel struct {
	ID           string   `json:"id"`
	Gated        any      `json:"gated"`
	Private      bool     `json:"private"`
	Downloads    int64    `json:"downloads"`
	Likes        int64    `json:"likes"`
	LastModified string   `json:"lastModified"`
	Tags         []string `json:"tags"`
	Siblings     []struct {
		Filename string `json:"rfilename"`
	} `json:"siblings"`
}

func (s *Server) handleModelsSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.jsonError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 100 {
		s.jsonError(w, "search query must be at most 100 characters", http.StatusBadRequest)
		return
	}
	limit := modelCatalogDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > modelCatalogMaxLimit {
			s.jsonError(w, fmt.Sprintf("limit must be between 1 and %d", modelCatalogMaxLimit), http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	models, err := s.searchHuggingFaceModels(r.Context(), query, limit)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("failed to search Hugging Face models", "query", query, "error", err)
		}
		s.jsonError(w, "failed to search Hugging Face models", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"source": "huggingface",
		"models": models,
	})
}

func (s *Server) searchHuggingFaceModels(ctx context.Context, query string, limit int) ([]MarketplaceModel, error) {
	cacheKey := strings.ToLower(query) + "\x00" + strconv.Itoa(limit)
	now := time.Now()
	s.modelCatalogMu.Lock()
	if entry, ok := s.modelCatalogCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		models := append([]MarketplaceModel(nil), entry.models...)
		s.modelCatalogMu.Unlock()
		return models, nil
	}
	s.modelCatalogMu.Unlock()

	endpoint := s.modelCatalogURL
	if endpoint == "" {
		endpoint = defaultHuggingFaceModelsURL
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Hugging Face models URL: %w", err)
	}
	params := u.Query()
	if query != "" {
		params.Set("search", query)
	}
	params.Set("filter", "gguf")
	params.Set("pipeline_tag", "text-generation")
	params.Set("apps", "ollama")
	params.Set("gated", "false")
	params.Set("sort", "downloads")
	params.Set("direction", "-1")
	params.Set("limit", strconv.Itoa(limit))
	for _, field := range []string{"downloads", "likes", "lastModified", "tags", "gated", "private", "siblings"} {
		params.Add("expand", field)
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Hugging Face request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AxiOS/1.0")

	client := s.modelCatalogClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query Hugging Face: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hugging Face returned %s", resp.Status)
	}

	var remote []huggingFaceModel
	decoder := json.NewDecoder(io.LimitReader(resp.Body, modelCatalogMaxResponse))
	if err := decoder.Decode(&remote); err != nil {
		return nil, fmt.Errorf("decode Hugging Face response: %w", err)
	}

	models := make([]MarketplaceModel, 0, len(remote))
	for _, model := range remote {
		mapped, ok := mapHuggingFaceModel(model)
		if ok {
			models = append(models, mapped)
		}
	}

	s.modelCatalogMu.Lock()
	if s.modelCatalogCache == nil {
		s.modelCatalogCache = make(map[string]modelCatalogCacheEntry)
	}
	// Evict expired entries so distinct queries can't grow the map forever.
	for key, entry := range s.modelCatalogCache {
		if !now.Before(entry.expiresAt) {
			delete(s.modelCatalogCache, key)
		}
	}
	s.modelCatalogCache[cacheKey] = modelCatalogCacheEntry{
		models:    append([]MarketplaceModel(nil), models...),
		expiresAt: now.Add(modelCatalogCacheTTL),
	}
	s.modelCatalogMu.Unlock()
	return models, nil
}

func mapHuggingFaceModel(model huggingFaceModel) (MarketplaceModel, bool) {
	if model.Private || isGatedModel(model.Gated) || !huggingFaceModelIDPattern.MatchString(model.ID) || isAuxiliaryGGUFRepo(model.ID) {
		return MarketplaceModel{}, false
	}

	quantizations := huggingFaceQuantizations(model.Siblings)
	hasGGUF := false
	for _, sibling := range model.Siblings {
		if strings.EqualFold(path.Ext(sibling.Filename), ".gguf") {
			hasGGUF = true
			break
		}
	}
	if !hasGGUF {
		return MarketplaceModel{}, false
	}

	author := strings.SplitN(model.ID, "/", 2)[0]
	return MarketplaceModel{
		Name:         model.ID,
		Description:  fmt.Sprintf("Community GGUF model published by %s. Review its model card and license before installing.", author),
		Tags:         quantizations,
		Category:     huggingFaceCategory(model),
		Parameters:   huggingFaceParameters(model.ID),
		Source:       "huggingface",
		PullName:     "hf.co/" + model.ID,
		Author:       author,
		Downloads:    model.Downloads,
		Likes:        model.Likes,
		LastModified: model.LastModified,
		License:      huggingFaceLicense(model.Tags),
		URL:          "https://huggingface.co/" + model.ID,
	}, true
}

func isAuxiliaryGGUFRepo(modelID string) bool {
	repository := strings.ToLower(strings.SplitN(modelID, "/", 2)[1])
	return strings.HasSuffix(repository, "-layers") || strings.HasSuffix(repository, "_layers")
}

func isGatedModel(gated any) bool {
	switch value := gated.(type) {
	case bool:
		return value
	case string:
		return value != "" && value != "false"
	default:
		return false
	}
}

func huggingFaceQuantizations(siblings []struct {
	Filename string `json:"rfilename"`
}) []string {
	unique := make(map[string]struct{})
	for _, sibling := range siblings {
		if !strings.EqualFold(path.Ext(sibling.Filename), ".gguf") {
			continue
		}
		match := quantizationPattern.FindStringSubmatch(sibling.Filename)
		if len(match) > 1 {
			unique[strings.ToUpper(match[1])] = struct{}{}
		}
	}

	quantizations := make([]string, 0, len(unique))
	for quantization := range unique {
		quantizations = append(quantizations, quantization)
	}
	priority := map[string]int{
		"Q4_K_M": 0,
		"Q4_K_S": 1,
		"Q5_K_M": 2,
		"Q5_K_S": 3,
		"Q6_K":   4,
		"Q8_0":   5,
		"BF16":   100,
		"F16":    101,
		"F32":    102,
	}
	sort.Slice(quantizations, func(i, j int) bool {
		left, leftOK := priority[quantizations[i]]
		if !leftOK {
			left = 50
		}
		right, rightOK := priority[quantizations[j]]
		if !rightOK {
			right = 50
		}
		if left != right {
			return left < right
		}
		return quantizations[i] < quantizations[j]
	})
	return quantizations
}

func huggingFaceParameters(modelID string) string {
	matches := parameterPattern.FindAllStringSubmatch(modelID, -1)
	if len(matches) == 0 {
		return "Unknown"
	}
	bestValue := -1.0
	best := ""
	for _, match := range matches {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		if strings.EqualFold(match[2], "M") {
			value /= 1000
		}
		if value > bestValue {
			bestValue = value
			best = strings.ToUpper(match[1] + match[2])
		}
	}
	if best == "" {
		return "Unknown"
	}
	return best
}

func huggingFaceCategory(model huggingFaceModel) string {
	joined := strings.ToLower(model.ID + " " + strings.Join(model.Tags, " "))
	switch {
	case strings.Contains(joined, "embed"):
		return "embedding"
	case strings.Contains(joined, "vision") || strings.Contains(joined, "multimodal"):
		return "vision"
	case strings.Contains(joined, "code") || strings.Contains(joined, "coder"):
		return "code"
	default:
		return "general"
	}
}

func huggingFaceLicense(tags []string) string {
	for _, tag := range tags {
		if license, ok := strings.CutPrefix(tag, "license:"); ok {
			return license
		}
	}
	return "unknown"
}
