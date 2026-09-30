package core

import (
	"context"
	"strings"
	"testing"
)

func TestNewsAndLadder(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.site.put("/feed.json", []byte(`{
		"version": "https://jsonfeed.org/version/1.1",
		"title": "Slash",
		"items": [
			{"id": "1", "title": "Old post", "content_text": "older", "date_published": "2026-08-01T00:00:00Z", "url": "https://slash.test/1"},
			{"id": "2", "title": "Ladder reset", "summary": "New season", "date_published": "2026-09-20T00:00:00Z", "url": "javascript:alert(1)"},
			{"id": "3", "title": "", "content_text": "untitled is dropped"}
		]
	}`))
	h.site.put("/ladder.json", []byte(`{
		"schema": 1,
		"boards": [
			{"id": "sc", "name": "Softcore", "entries": [{"rank": 1, "name": "Meanski", "class": "Sorceress", "level": 99}]},
			{"id": "", "name": "No id, dropped", "entries": []}
		]
	}`))

	h.profile["news"] = h.site.url("/feed.json")
	h.profile["ladder"] = h.site.url("/ladder.json")
	h.profile["version"] = 3
	h.list(t)

	if _, err := h.m.Servers(ctx); err != nil {
		t.Fatal(err)
	}

	news, err := h.m.News(ctx, "slash")
	if err != nil {
		t.Fatal(err)
	}
	if len(news) != 2 || news[0].Title != "Ladder reset" || news[1].Summary != "older" {
		t.Fatalf("news = %+v", news)
	}
	if news[0].URL != "" {
		t.Errorf("non-https link kept: %q", news[0].URL)
	}

	ladder, err := h.m.Ladder(ctx, "slash")
	if err != nil {
		t.Fatal(err)
	}
	if len(ladder.Boards) != 1 || ladder.Boards[0].Entries[0].Name != "Meanski" {
		t.Errorf("ladder = %+v", ladder)
	}

	// Offline, the last copy is used.
	h.site.mu.Lock()
	h.site.down = true
	h.site.mu.Unlock()

	if news, err := h.m.News(ctx, "slash"); err != nil || len(news) != 2 {
		t.Errorf("offline news = %+v, %v", news, err)
	}
}

func TestLadderSchemaChecked(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.site.put("/ladder.json", []byte(`{"characters": []}`))
	h.profile["ladder"] = h.site.url("/ladder.json")
	h.profile["version"] = 3
	h.list(t)
	h.m.Servers(ctx)

	if _, err := h.m.Ladder(ctx, "slash"); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Errorf("ladder without schema: %v", err)
	}
}

func TestClip(t *testing.T) {
	if got := clip("  héllo wörld  ", 6); got != "héllo…" {
		t.Errorf("clip = %q", got)
	}
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip = %q", got)
	}
}
