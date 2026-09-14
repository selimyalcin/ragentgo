package huggingface

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseFeatureVectors(t *testing.T) {
	batch, _ := json.Marshal([][]float32{{0.1, 0.2}, {0.3, 0.4}})
	got, err := parseFeatureVectors(batch, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1][0] != 0.3 {
		t.Fatalf("batch: %#v", got)
	}

	one, _ := json.Marshal([]float32{1, 2, 3})
	got, err = parseFeatureVectors(one, 1)
	if err != nil || len(got) != 1 || got[0][2] != 3 {
		t.Fatalf("single: %#v err=%v", got, err)
	}

	tokens, _ := json.Marshal([][][]float32{{{1, 1}, {3, 3}}})
	got, err = parseFeatureVectors(tokens, 1)
	if err != nil || len(got) != 1 || got[0][0] != 2 {
		t.Fatalf("token mean: %#v err=%v", got, err)
	}
}

func TestEmbedderFeatureExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/intfloat/multilingual-e5-small/pipeline/feature-extraction" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([][]float32{{0.5, 0.25, 0.25}})
	}))
	defer srv.Close()

	e := NewEmbedder(Options{BaseURL: srv.URL, Model: "intfloat/multilingual-e5-small", Timeout: 0})
	vecs, err := e.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 1 || vecs[0][0] != 0.5 {
		t.Fatalf("vecs=%#v", vecs)
	}
}

func TestInferenceBaseRewritesOpenAIv1(t *testing.T) {
	if got := inferenceBase("https://router.huggingface.co/v1"); got != DefaultInferenceURL {
		t.Fatalf("got %s", got)
	}
}
