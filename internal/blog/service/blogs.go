package service

import (
	"context"
	"strings"

	"blog-backend/internal/blog/repository"
	"blog-backend/internal/blog/schema"
)

// BlogService contains the business logic for blog operations.
type BlogService struct {
	repo repository.IBlogRepository
}

// NewBlogService creates a new BlogService.
func NewBlogService(repo repository.IBlogRepository) *BlogService {
	return &BlogService{repo: repo}
}

// ──────────────────────────────────────────────
//  List blogs (paginated, with optional title/tag filter)
// ──────────────────────────────────────────────

// ListBlogsRequest carries query parameters for listing blogs.
type ListBlogsRequest struct {
	PageNo      int    `form:"page_no"`
	PageSize    int    `form:"page_size"`
	FilterTitle string `form:"filter_title"`
	FilterTag   string `form:"filter_tag"`
	OrderBy     string `form:"order_by" binding:"omitempty,oneof=date title created_at"`
	Order       string `form:"order"    binding:"omitempty,oneof=asc desc"`
}

// ListBlogsResponse is the envelope returned to the transport layer.
type ListBlogsResponse struct {
	Total    int64                 `json:"total"`
	ItemList []repository.BlogItem `json:"item_list"`
}

// ListBlogs returns a paginated list of blog summaries.
func (s *BlogService) ListBlogs(
	ctx context.Context,
	req *ListBlogsRequest,
) (*ListBlogsResponse, error) {
	if req.PageNo <= 0 {
		req.PageNo = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	var filter *repository.BlogFilter
	if req.FilterTitle != "" || req.FilterTag != "" {
		filter = &repository.BlogFilter{
			Title: req.FilterTitle,
			Tag:   req.FilterTag,
		}
	}

	var order *repository.Order
	if req.OrderBy != "" {
		order = &repository.Order{
			Field: req.OrderBy,
			Desc:  req.Order == "desc",
		}
	}

	param := repository.SearchParam{
		Filter: filter,
		Order:  order,
	}

	items, total, err := s.repo.ListBlogs(
		ctx,
		req.PageSize,
		(req.PageNo-1)*req.PageSize,
		param,
	)
	if err != nil {
		return nil, err
	}

	return &ListBlogsResponse{
		Total:    total,
		ItemList: items,
	}, nil
}

// ──────────────────────────────────────────────
//  Recent blogs (shortcut – latest N posts)
// ──────────────────────────────────────────────

// RecentBlogsRequest carries query parameters for fetching recent posts.
type RecentBlogsRequest struct {
	Limit int `form:"limit"`
}

// RecentBlogsResponse wraps the result.
type RecentBlogsResponse struct {
	ItemList []repository.BlogItem `json:"item_list"`
}

// RecentBlogs returns the N most recent blog posts (default 5).
func (s *BlogService) RecentBlogs(
	ctx context.Context,
	req *RecentBlogsRequest,
) (*RecentBlogsResponse, error) {
	if req.Limit <= 0 || req.Limit > 20 {
		req.Limit = 5
	}

	items, _, err := s.repo.ListBlogs(ctx, req.Limit, 0)
	if err != nil {
		return nil, err
	}

	return &RecentBlogsResponse{
		ItemList: items,
	}, nil
}

// ──────────────────────────────────────────────
//  Get single blog detail
// ──────────────────────────────────────────────

// GetBlogRequest carries the blog ID.
type GetBlogRequest struct {
	ID int64 `uri:"id" binding:"required"`
}

// GetBlogResponse wraps the full blog detail.
type GetBlogResponse struct {
	Item *repository.BlogDetail `json:"item"`
}

// GetBlog returns the full detail for a single blog post.
func (s *BlogService) GetBlog(
	ctx context.Context,
	req *GetBlogRequest,
) (*GetBlogResponse, error) {
	detail, err := s.repo.GetBlogByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	return &GetBlogResponse{Item: detail}, nil
}

// ──────────────────────────────────────────────
//  Create blog (CLI uploads meta.yaml content)
// ──────────────────────────────────────────────

// CreateBlogRequest is the JSON body sent by the CLI tool.
type CreateBlogRequest struct {
	Path    string   `json:"path"    binding:"required"` // relative dir within repo, e.g. "my-first-post"
	Title   string   `json:"title"   binding:"required"`
	Summary string   `json:"summary"`
	Date    string   `json:"date"    binding:"required"`
	Tags    []string `json:"tags"` // tag names
	Cover   string   `json:"cover"`
	Author  string   `json:"author"`
}

// CreateBlogResponse returns the created blog's ID.
type CreateBlogResponse struct {
	ID int64 `json:"id"`
}

// CreateBlog creates a new blog record from CLI-submitted metadata.
func (s *BlogService) CreateBlog(
	ctx context.Context,
	req *CreateBlogRequest,
) (*CreateBlogResponse, error) {
	blog := &schema.Blog{
		Title:   strings.TrimSpace(req.Title),
		Summary: strings.TrimSpace(req.Summary),
		Path:    strings.TrimSpace(req.Path),
		Date:    strings.TrimSpace(req.Date),
		Cover:   strings.TrimSpace(req.Cover),
		Author:  strings.TrimSpace(req.Author),
	}

	if err := s.repo.CreateBlog(ctx, blog, req.Tags); err != nil {
		return nil, err
	}

	return &CreateBlogResponse{ID: blog.ID}, nil
}

// ──────────────────────────────────────────────
//  Update blog metadata
// ──────────────────────────────────────────────

// UpdateBlogRequest carries the ID in URI and updated fields in JSON body.
type UpdateBlogRequest struct {
	ID      int64     `uri:"id"      binding:"required"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
	Date    string    `json:"date"`
	Tags    *[]string `json:"tags"` // nil = don't change, empty = clear all, non-empty = set
	Cover   string    `json:"cover"`
	Author  string    `json:"author"`
	Path    string    `json:"path"`
}

// UpdateBlogResponse is an empty success response.
type UpdateBlogResponse struct{}

// UpdateBlog updates the metadata of an existing blog.
func (s *BlogService) UpdateBlog(
	ctx context.Context,
	req *UpdateBlogRequest,
) (*UpdateBlogResponse, error) {
	updates := make(map[string]interface{})

	if req.Title != "" {
		updates["title"] = strings.TrimSpace(req.Title)
	}
	if req.Summary != "" {
		updates["summary"] = strings.TrimSpace(req.Summary)
	}
	if req.Date != "" {
		updates["date"] = strings.TrimSpace(req.Date)
	}
	if req.Cover != "" {
		updates["cover"] = strings.TrimSpace(req.Cover)
	}
	if req.Author != "" {
		updates["author"] = strings.TrimSpace(req.Author)
	}
	if req.Path != "" {
		updates["path"] = strings.TrimSpace(req.Path)
	}

	if len(updates) == 0 && req.Tags == nil {
		return &UpdateBlogResponse{}, nil
	}

	if err := s.repo.UpdateBlog(ctx, req.ID, updates, req.Tags); err != nil {
		return nil, err
	}

	return &UpdateBlogResponse{}, nil
}

// ──────────────────────────────────────────────
//  Delete blog
// ──────────────────────────────────────────────

// DeleteBlogRequest carries the blog ID.
type DeleteBlogRequest struct {
	ID int64 `uri:"id" binding:"required"`
}

// DeleteBlogResponse is an empty success response.
type DeleteBlogResponse struct{}

// DeleteBlog removes a blog record.
func (s *BlogService) DeleteBlog(
	ctx context.Context,
	req *DeleteBlogRequest,
) (*DeleteBlogResponse, error) {
	if err := s.repo.DeleteBlog(ctx, req.ID); err != nil {
		return nil, err
	}
	return &DeleteBlogResponse{}, nil
}

// ──────────────────────────────────────────────
//  Site statistics
// ──────────────────────────────────────────────

// StatsRequest is empty – no input needed.
type StatsRequest struct{}

// StatsResponse wraps the site statistics.
type StatsResponse struct {
	PostCount int64 `json:"post_count"`
	TagCount  int64 `json:"tag_count"`
}

// Stats returns aggregate site statistics.
func (s *BlogService) Stats(
	ctx context.Context,
	_ *StatsRequest,
) (*StatsResponse, error) {
	stats, err := s.repo.GetStats(ctx)
	if err != nil {
		return nil, err
	}
	return &StatsResponse{
		PostCount: stats.PostCount,
		TagCount:  stats.TagCount,
	}, nil
}

// ──────────────────────────────────────────────
//  List tags
// ──────────────────────────────────────────────

// ListTagsRequest is empty — no input needed.
type ListTagsRequest struct{}

// ListTagsResponse wraps the list of tags.
type ListTagsResponse struct {
	Items []repository.TagItem `json:"items"`
}

// ListTags returns all tags with their associated post counts.
func (s *BlogService) ListTags(
	ctx context.Context,
	_ *ListTagsRequest,
) (*ListTagsResponse, error) {
	items, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	return &ListTagsResponse{Items: items}, nil
}
