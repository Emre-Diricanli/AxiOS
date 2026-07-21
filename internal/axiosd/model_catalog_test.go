package axiosd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHandleModelsSearchReturnsRunnableHuggingFaceModels(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("search"); got != "coder" {
			t.Errorf("search = %q, want coder", got)
		}
		for key, want := range map[string]string{
			"filter":       "gguf",
			"pipeline_tag": "text-generation",
			"apps":         "ollama",
			"gated":        "false",
			"limit":        "10",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		json.NewEncoder(w).Encode([]map[string]any{
			{
				"id":           "acme/Coder-7B-GGUF",
				"gated":        false,
				"private":      false,
				"downloads":    1234,
				"likes":        56,
				"lastModified": "2026-07-20T12:00:00.000Z",
				"tags":         []string{"gguf", "text-generation", "license:apache-2.0"},
				"siblings": []map[string]string{
					{"rfilename": "README.md"},
					{"rfilename": "coder-7b-Q5_K_M-00001-of-00002.gguf"},
					{"rfilename": "coder-7b-Q4_K_M.gguf"},
				},
			},
			{
				"id":       "acme/Private-7B-GGUF",
				"private":  true,
				"siblings": []map[string]string{{"rfilename": "private-Q4_K_M.gguf"}},
			},
			{
				"id":       "acme/Gated-7B-GGUF",
				"gated":    "manual",
				"siblings": []map[string]string{{"rfilename": "gated-Q4_K_M.gguf"}},
			},
			{
				"id":       "acme/No-Weights-7B",
				"siblings": []map[string]string{{"rfilename": "README.md"}},
			},
			{
				"id":       "acme/Coder-7B-layers",
				"siblings": []map[string]string{{"rfilename": "layer-01-Q4_K_M.gguf"}},
			},
		})
	}))
	defer upstream.Close()

	s := &Server{
		logger:             testLogger(),
		modelCatalogClient: upstream.Client(),
		modelCatalogURL:    upstream.URL,
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/models/search?q=coder&limit=10", nil)
		rec := httptest.NewRecorder()
		s.handleModelsSearch(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var response struct {
			Source string             `json:"source"`
			Models []MarketplaceModel `json:"models"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Source != "huggingface" {
			t.Errorf("source = %q, want huggingface", response.Source)
		}
		if len(response.Models) != 1 {
			t.Fatalf("model count = %d, want 1", len(response.Models))
		}
		model := response.Models[0]
		if model.Name != "acme/Coder-7B-GGUF" || model.PullName != "hf.co/acme/Coder-7B-GGUF" {
			t.Errorf("unexpected model identity: name=%q pull_name=%q", model.Name, model.PullName)
		}
		if !reflect.DeepEqual(model.Tags, []string{"Q4_K_M", "Q5_K_M"}) {
			t.Errorf("quantizations = %v, want [Q4_K_M Q5_K_M]", model.Tags)
		}
		if model.Parameters != "7B" || model.Category != "code" || model.License != "apache-2.0" {
			t.Errorf("unexpected metadata: parameters=%q category=%q license=%q", model.Parameters, model.Category, model.License)
		}
	}

	if requests != 1 {
		t.Errorf("upstream requests = %d, want 1 cached request", requests)
	}
}

func TestHandleModelsSearchValidatesRequest(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		status int
	}{
		{"requires GET", http.MethodPost, "/api/models/search", http.StatusMethodNotAllowed},
		{"rejects invalid limit", http.MethodGet, "/api/models/search?limit=0", http.StatusBadRequest},
		{"rejects excessive limit", http.MethodGet, "/api/models/search?limit=51", http.StatusBadRequest},
		{"rejects long query", http.MethodGet, "/api/models/search?q=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{logger: testLogger()}
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()
			s.handleModelsSearch(rec, req)
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.status, rec.Body.String())
			}
		})
	}
}

func TestHuggingFaceParameters(t *testing.T) {
	tests := []struct {
		modelID string
		want    string
	}{
		{"acme/Model-7B-GGUF", "7B"},
		{"acme/MoE-30B-A3B-GGUF", "30B"},
		{"acme/Embed-137M-GGUF", "137M"},
		{"acme/Model-GGUF", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := huggingFaceParameters(tt.modelID); got != tt.want {
				t.Errorf("huggingFaceParameters(%q) = %q, want %q", tt.modelID, got, tt.want)
			}
		})
	}
}
