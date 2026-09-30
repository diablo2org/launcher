package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// NewsItem is one post from a server's JSON Feed.
type NewsItem struct {
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Summary string    `json:"summary"`
	Date    time.Time `json:"date"`
}

// jsonFeed is the part of JSON Feed 1.1 the launcher shows.
type jsonFeed struct {
	Version string `json:"version"`
	Items   []struct {
		ID            string    `json:"id"`
		Title         string    `json:"title"`
		URL           string    `json:"url"`
		Summary       string    `json:"summary"`
		ContentText   string    `json:"content_text"`
		DatePublished time.Time `json:"date_published"`
	} `json:"items"`
}

// maxNews is how many posts the launcher keeps.
const maxNews = 20

// News returns a server's latest posts, newest first. Only plain text is used:
// HTML from a feed is never rendered.
func (m *Manager) News(ctx context.Context, id string) ([]NewsItem, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.News == "" {
		return []NewsItem{}, nil
	}

	data, err := m.cachedDocument(ctx, id, p.Hosts, p.News, "news.json")
	if err != nil {
		return nil, err
	}

	var feed jsonFeed
	if err := json.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("%s news: %w", p.Name, err)
	}
	if !strings.HasPrefix(feed.Version, "https://jsonfeed.org/version/") {
		return nil, fmt.Errorf("%s news is not a JSON Feed", p.Name)
	}

	items := make([]NewsItem, 0, len(feed.Items))
	for _, it := range feed.Items {
		summary := it.Summary
		if summary == "" {
			summary = it.ContentText
		}

		item := NewsItem{
			Title:   clip(it.Title, 120),
			Summary: clip(summary, 600),
			Date:    it.DatePublished,
		}
		// Links only open in the browser, and only if they're https.
		if strings.HasPrefix(it.URL, "https://") {
			item.URL = it.URL
		}
		if item.Title != "" {
			items = append(items, item)
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })
	if len(items) > maxNews {
		items = items[:maxNews]
	}

	return items, nil
}

// Ladder is a server's ladder: one or more boards, such as softcore and
// hardcore. See docs/SPEC.md section 3.
type Ladder struct {
	Boards []Board `json:"boards"`
}

// Board is one ranked list.
type Board struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Entries []LadderEntry `json:"entries"`
}

// LadderEntry is one character on a board.
type LadderEntry struct {
	Rank   int    `json:"rank"`
	Name   string `json:"name"`
	Class  string `json:"class"`
	Level  int    `json:"level"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

const maxLadderEntries = 200

// Ladder returns a server's ladder.
func (m *Manager) Ladder(ctx context.Context, id string) (*Ladder, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Ladder == "" {
		return &Ladder{Boards: []Board{}}, nil
	}

	data, err := m.cachedDocument(ctx, id, p.Hosts, p.Ladder, "ladder.json")
	if err != nil {
		return nil, err
	}

	var doc struct {
		Schema int     `json:"schema"`
		Boards []Board `json:"boards"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s ladder: %w", p.Name, err)
	}
	if doc.Schema != 1 {
		return nil, fmt.Errorf("%s ladder: unsupported schema %d", p.Name, doc.Schema)
	}

	l := &Ladder{Boards: []Board{}}
	for _, b := range doc.Boards {
		if b.ID == "" || b.Name == "" {
			continue
		}
		if len(b.Entries) > maxLadderEntries {
			b.Entries = b.Entries[:maxLadderEntries]
		}
		for i := range b.Entries {
			e := &b.Entries[i]
			e.Name, e.Class, e.Title, e.Status = clip(e.Name, 32), clip(e.Class, 16), clip(e.Title, 24), clip(e.Status, 16)
		}
		b.Name = clip(b.Name, 32)
		l.Boards = append(l.Boards, b)
	}

	return l, nil
}

// cachedDocument fetches a document, falling back to the last copy
// when offline.
func (m *Manager) cachedDocument(ctx context.Context, id string, hosts []string, rawURL, cache string) ([]byte, error) {
	data, err := m.client(hosts).Document(ctx, rawURL)
	if err != nil {
		if cached, _ := m.store.ReadCache(id, cache); cached != nil {
			return cached, nil
		}
		return nil, err
	}

	m.store.WriteCache(id, cache, data)
	return data, nil
}

// clip shortens text for display, on a rune boundary.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}

	return strings.TrimSpace(string(r[:max-1])) + "…"
}
