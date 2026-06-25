package service

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"blog-backend/internal/column/repository"
	"blog-backend/internal/column/schema"
)

// ColumnService contains the business logic for column (专栏) operations.
type ColumnService struct {
	repo repository.IColumnRepository
}

// NewColumnService creates a new ColumnService.
func NewColumnService(repo repository.IColumnRepository) *ColumnService {
	return &ColumnService{repo: repo}
}

// chapterPrefixRe strips a leading order prefix like "01-" / "02_" from a name.
var chapterPrefixRe = regexp.MustCompile(`^\d+[-_]?`)

// deriveChapterSlug turns a chapter filename into a URL slug, e.g.
// "01-intro.md" -> "intro", "pairings.md" -> "pairings".
func deriveChapterSlug(file string) string {
	base := filepath.Base(strings.TrimSpace(file))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return chapterPrefixRe.ReplaceAllString(base, "")
}

// ChapterInput is a chapter entry as submitted by the CLI (from meta.yaml).
type ChapterInput struct {
	File  string `json:"file"  binding:"required"`
	Title string `json:"title" binding:"required"`
}

func toSchemaChapters(inputs []ChapterInput) []schema.Chapter {
	chapters := make([]schema.Chapter, 0, len(inputs))
	for i, in := range inputs {
		chapters = append(chapters, schema.Chapter{
			Slug:  deriveChapterSlug(in.File),
			Title: strings.TrimSpace(in.Title),
			File:  strings.TrimSpace(in.File),
			Sort:  i,
		})
	}
	return chapters
}

// ──────────────────────────────────────────────
//  List columns
// ──────────────────────────────────────────────

type ListColumnsRequest struct {
	PageNo      int    `form:"page_no"`
	PageSize    int    `form:"page_size"`
	FilterTitle string `form:"filter_title"`
	FilterTag   string `form:"filter_tag"`
	OrderBy     string `form:"order_by" binding:"omitempty,oneof=updated_at title created_at"`
	Order       string `form:"order"    binding:"omitempty,oneof=asc desc"`
}

type ListColumnsResponse struct {
	Total    int64                   `json:"total"`
	ItemList []repository.ColumnItem `json:"item_list"`
}

func (s *ColumnService) ListColumns(
	ctx context.Context,
	req *ListColumnsRequest,
) (*ListColumnsResponse, error) {
	if req.PageNo <= 0 {
		req.PageNo = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	var filter *repository.ColumnFilter
	if req.FilterTitle != "" || req.FilterTag != "" {
		filter = &repository.ColumnFilter{Title: req.FilterTitle, Tag: req.FilterTag}
	}

	var order *repository.Order
	if req.OrderBy != "" {
		order = &repository.Order{Field: req.OrderBy, Desc: req.Order == "desc"}
	}

	items, total, err := s.repo.ListColumns(
		ctx,
		req.PageSize,
		(req.PageNo-1)*req.PageSize,
		repository.SearchParam{Filter: filter, Order: order},
	)
	if err != nil {
		return nil, err
	}
	return &ListColumnsResponse{Total: total, ItemList: items}, nil
}

// ──────────────────────────────────────────────
//  Get column detail (+ chapter list)
// ──────────────────────────────────────────────

type GetColumnRequest struct {
	Slug string `uri:"slug" binding:"required"`
}

type GetColumnResponse struct {
	Item *repository.ColumnDetail `json:"item"`
}

func (s *ColumnService) GetColumn(
	ctx context.Context,
	req *GetColumnRequest,
) (*GetColumnResponse, error) {
	detail, err := s.repo.GetColumnBySlug(ctx, req.Slug)
	if err != nil {
		return nil, err
	}
	return &GetColumnResponse{Item: detail}, nil
}

// ──────────────────────────────────────────────
//  Get a single chapter (reading view)
// ──────────────────────────────────────────────

type GetChapterRequest struct {
	Slug    string `uri:"slug"    binding:"required"`
	Chapter string `uri:"chapter" binding:"required"`
}

type GetChapterResponse struct {
	Item *repository.ChapterView `json:"item"`
}

func (s *ColumnService) GetChapter(
	ctx context.Context,
	req *GetChapterRequest,
) (*GetChapterResponse, error) {
	view, err := s.repo.GetChapter(ctx, req.Slug, req.Chapter)
	if err != nil {
		return nil, err
	}
	return &GetChapterResponse{Item: view}, nil
}

// ──────────────────────────────────────────────
//  Create column (CLI)
// ──────────────────────────────────────────────

type CreateColumnRequest struct {
	Slug     string         `json:"slug"     binding:"required"`
	Title    string         `json:"title"    binding:"required"`
	Summary  string         `json:"summary"`
	Author   string         `json:"author"`
	Path     string         `json:"path"     binding:"required"` // relative dir within repo, e.g. "columns/zk-snark"
	Tags     []string       `json:"tags"`
	Chapters []ChapterInput `json:"chapters" binding:"required,min=1"`
}

type CreateColumnResponse struct {
	ID int64 `json:"id"`
}

func (s *ColumnService) CreateColumn(
	ctx context.Context,
	req *CreateColumnRequest,
) (*CreateColumnResponse, error) {
	col := &schema.Column{
		Slug:    strings.TrimSpace(req.Slug),
		Title:   strings.TrimSpace(req.Title),
		Summary: strings.TrimSpace(req.Summary),
		Author:  strings.TrimSpace(req.Author),
		Path:    strings.TrimSpace(req.Path),
	}
	if err := s.repo.CreateColumn(ctx, col, req.Tags, toSchemaChapters(req.Chapters)); err != nil {
		return nil, err
	}
	return &CreateColumnResponse{ID: col.ID}, nil
}

// ──────────────────────────────────────────────
//  Update column (CLI)
// ──────────────────────────────────────────────

type UpdateColumnRequest struct {
	Slug     string          `uri:"slug" binding:"required"`
	Title    string          `json:"title"`
	Summary  string          `json:"summary"`
	Author   string          `json:"author"`
	Path     string          `json:"path"`
	Tags     *[]string       `json:"tags"`     // nil = leave unchanged
	Chapters *[]ChapterInput `json:"chapters"` // nil = leave unchanged
}

type UpdateColumnResponse struct{}

func (s *ColumnService) UpdateColumn(
	ctx context.Context,
	req *UpdateColumnRequest,
) (*UpdateColumnResponse, error) {
	updates := make(map[string]interface{})
	if req.Title != "" {
		updates["title"] = strings.TrimSpace(req.Title)
	}
	if req.Summary != "" {
		updates["summary"] = strings.TrimSpace(req.Summary)
	}
	if req.Author != "" {
		updates["author"] = strings.TrimSpace(req.Author)
	}
	if req.Path != "" {
		updates["path"] = strings.TrimSpace(req.Path)
	}

	var chapters *[]schema.Chapter
	if req.Chapters != nil {
		c := toSchemaChapters(*req.Chapters)
		chapters = &c
	}

	if len(updates) == 0 && req.Tags == nil && chapters == nil {
		return &UpdateColumnResponse{}, nil
	}

	if err := s.repo.UpdateColumn(ctx, req.Slug, updates, req.Tags, chapters); err != nil {
		return nil, err
	}
	return &UpdateColumnResponse{}, nil
}

// ──────────────────────────────────────────────
//  Delete column (CLI)
// ──────────────────────────────────────────────

type DeleteColumnRequest struct {
	Slug string `uri:"slug" binding:"required"`
}

type DeleteColumnResponse struct{}

func (s *ColumnService) DeleteColumn(
	ctx context.Context,
	req *DeleteColumnRequest,
) (*DeleteColumnResponse, error) {
	if err := s.repo.DeleteColumn(ctx, req.Slug); err != nil {
		return nil, err
	}
	return &DeleteColumnResponse{}, nil
}

// ──────────────────────────────────────────────
//  List column tags
// ──────────────────────────────────────────────

type ListTagsRequest struct{}

type ListTagsResponse struct {
	Items []repository.TagItem `json:"items"`
}

func (s *ColumnService) ListTags(
	ctx context.Context,
	_ *ListTagsRequest,
) (*ListTagsResponse, error) {
	items, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	return &ListTagsResponse{Items: items}, nil
}
