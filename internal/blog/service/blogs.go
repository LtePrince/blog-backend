package service

import (
	"context"

	"blog-backend/internal/blog/repository"
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
//  List blogs (paginated, with optional title filter)
// ──────────────────────────────────────────────

// ListBlogsRequest carries query parameters for listing blogs.
type ListBlogsRequest struct {
	PageNo      int    `form:"page_no"`
	PageSize    int    `form:"page_size"`
	FilterTitle string `form:"filter_title"`
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
	if req.FilterTitle != "" {
		filter = &repository.BlogFilter{Title: req.FilterTitle}
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
