package web

import (
	"encoding/json"
	"testing"
)

func TestShoreOSExtensionAssetsEmbedded(t *testing.T) {
	paths := []string{
		"static/index.html",
		"static/app.html",
		"static/shoreos-extension.css",
		"static/shoreos-extension.js",
		"static/knowledge-index.json",
	}
	for _, path := range paths {
		if _, err := StaticFS.ReadFile(path); err != nil {
			t.Fatalf("embedded asset %s: %v", path, err)
		}
	}
}

func TestKnowledgeIndexContract(t *testing.T) {
	raw, err := StaticFS.ReadFile("static/knowledge-index.json")
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Version int `json:"version"`
		Entries []struct {
			Type string `json:"type"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("parse knowledge index: %v", err)
	}
	if index.Version != 1 {
		t.Fatalf("knowledge index version = %d, want 1", index.Version)
	}
	allowed := map[string]bool{
		"raw": true, "domain": true, "concept": true,
		"entity": true, "decision": true, "synthesis": true,
	}
	for i, entry := range index.Entries {
		if !allowed[entry.Type] {
			t.Fatalf("entry %d has unsupported type %q", i, entry.Type)
		}
	}
}
