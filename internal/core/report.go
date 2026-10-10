package core

import (
	"context"
	"errors"
)

// PostReport sends a bug report to the server's report URL and returns its
// answer.
func (m *Manager) PostReport(ctx context.Context, id, contentType string, body []byte) ([]byte, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Report.URL == "" {
		return nil, errors.New("this server doesn't take bug reports")
	}
	if len(body) > p.Report.Limit() {
		return nil, errors.New("the report is larger than the server accepts")
	}

	return m.client(p.Hosts).Post(ctx, p.Report.URL, contentType, body)
}
