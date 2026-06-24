package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"blog-backend/internal/config"

	yaml "go.yaml.in/yaml/v3"
)

// MomentText is the bilingual body of a moment, keyed by UI locale so the
// frontend can follow the language toggle.
type MomentText struct {
	ZhCN string `yaml:"zh-CN" json:"zh-CN"`
	En   string `yaml:"en"    json:"en"`
}

// Moment is a single one-line personal update shown on the home timeline.
type Moment struct {
	Date string     `yaml:"date" json:"date"`
	Text MomentText `yaml:"text" json:"text"`
}

// MomentsService reads the moments timeline from the content repository.
// Like blog markdown, the source of truth lives on disk (moments/moments.yaml)
// so new entries are published by editing the file and pushing — no DB, no auth.
type MomentsService struct {
	repoDir string
}

// NewMomentsService creates a MomentsService bound to the content repo dir.
func NewMomentsService(cfg *config.Config) *MomentsService {
	return &MomentsService{repoDir: cfg.Content.RepoDir}
}

// ListMomentsRequest is empty — no input needed.
type ListMomentsRequest struct{}

// ListMomentsResponse wraps the moments list (newest first).
type ListMomentsResponse struct {
	Items []Moment `json:"items"`
}

// ListMoments reads moments/moments.yaml from the content repo and returns the
// entries sorted by date descending (newest first). A missing file yields an
// empty list rather than an error.
func (s *MomentsService) ListMoments(
	_ context.Context,
	_ *ListMomentsRequest,
) (*ListMomentsResponse, error) {
	path := filepath.Join(s.repoDir, "moments", "moments.yaml")

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ListMomentsResponse{Items: []Moment{}}, nil
		}
		return nil, fmt.Errorf("read moments %s: %w", path, err)
	}

	var items []Moment
	if err := yaml.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("parse moments yaml: %w", err)
	}

	// Dates are ISO (YYYY-MM-DD), so lexicographic order == chronological order.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Date > items[j].Date
	})

	if items == nil {
		items = []Moment{}
	}
	return &ListMomentsResponse{Items: items}, nil
}
