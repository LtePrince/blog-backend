package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	blogs "blog-backend/internal/blog/repository"
	post "blog-backend/internal/blog/schema"
	blogservice "blog-backend/internal/blog/service"
	columns "blog-backend/internal/column/repository"
	series "blog-backend/internal/column/schema"
	columnservice "blog-backend/internal/column/service"
	"blog-backend/internal/config"
	"blog-backend/internal/resource"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type publishedSite struct {
	root    string
	db      *gorm.DB
	blogs   blogs.IBlogRepository
	columns columns.IColumnRepository
}

func newPublishedSite(t *testing.T) *publishedSite {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "test.db")
	db, err := resource.NewDB(cfg)
	requireOK(t, err)
	pool, err := db.DB()
	requireOK(t, err)
	t.Cleanup(func() { pool.Close() })
	site := &publishedSite{root: root, db: db, blogs: blogs.NewBlogRepository(db, root), columns: columns.NewColumnRepository(db, root)}
	requireOK(t, site.blogs.AutoMigrate())
	requireOK(t, site.columns.AutoMigrate())
	return site
}

func requireOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (s *publishedSite) put(t *testing.T, name, body string) {
	t.Helper()
	dest := filepath.Join(s.root, name)
	requireOK(t, os.MkdirAll(filepath.Dir(dest), 0755))
	requireOK(t, os.WriteFile(dest, []byte(body), 0644))
}

func (s *publishedSite) chapters(t *testing.T) []series.Chapter {
	t.Helper()
	var rows []series.Chapter
	requireOK(t, s.db.Order("sort").Find(&rows).Error)
	return rows
}

func TestChapterPublicationAndFileSync(t *testing.T) {
	s := newPublishedSite(t)
	ctx := context.Background()
	s.put(t, "columns/book/a.md", "A")
	s.put(t, "columns/book/b.md", "B")
	col := series.Column{Slug: "book", Path: "columns/book", Title: "Book"}
	list := []series.Chapter{{Slug: "a", File: "a.md", Title: "A"}, {Slug: "b", File: "b.md", Title: "B"}}
	requireOK(t, s.columns.CreateColumn(ctx, &col, nil, list))
	// Existing installations have timestamps but no body hashes.
	requireOK(t, s.db.Exec("UPDATE column_chapters SET content_hash=NULL, published_markdown=NULL, created_at=80,updated_at=100+sort").Error)
	requireOK(t, s.columns.SeedPublishedChapters(ctx))
	before := s.chapters(t)
	if before[0].UpdatedAt != 100 || before[1].UpdatedAt != 101 || before[0].ContentHash != fmt.Sprintf("%x", sha256.Sum256([]byte("A"))) {
		t.Fatal(before)
	}
	s.put(t, "columns/book/a.md", "edited A")
	s.put(t, "columns/book/b.md", "edited B")
	requireOK(t, s.columns.SeedPublishedChapters(ctx))
	for _, original := range []string{"a", "b"} {
		view, err := s.columns.GetChapter(ctx, "book", original)
		requireOK(t, err)
		if view.Current.Content != strings.ToUpper(original) {
			t.Fatal(view.Current)
		}
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("edited B")))
	if err := s.columns.PublishChapter(ctx, "book", list[1], strings.Repeat("0", 64), nil); err == nil {
		t.Fatal("published wrong server version")
	}
	requireOK(t, s.columns.PublishChapter(ctx, "book", list[1], hash, nil))
	requireOK(t, s.columns.PublishChapter(ctx, "book", list[1], hash, nil))
	view, err := s.columns.GetChapter(ctx, "book", "b")
	requireOK(t, err)
	if view.Current.Content != "edited B" {
		t.Fatal(view.Current)
	}
	view, err = s.columns.GetChapter(ctx, "book", "a")
	requireOK(t, err)
	if view.Current.Content != "A" {
		t.Fatal(view.Current)
	}
	after := s.chapters(t)
	if before[0] != after[0] || before[1].ID != after[1].ID || before[1].CreatedAt != after[1].CreatedAt || after[1].UpdatedAt <= 101 || after[1].ContentHash != fmt.Sprintf("%x", sha256.Sum256([]byte("edited B"))) {
		t.Fatal(after)
	}
	s.put(t, "columns/book/a.md", "A")
	// Repeated POST, metadata edits and reordering must not create a new revision.
	list[0], list[1] = list[1], list[0]
	list[0].Title = "Renamed"
	requireOK(t, s.columns.CreateColumn(ctx, &col, nil, list))
	requireOK(t, s.columns.UpdateColumn(ctx, col.Slug, map[string]any{"title": "Book 2"}, nil, &list))
	detail, err := s.columns.GetColumnBySlug(ctx, col.Slug)
	requireOK(t, err)
	if detail.UpdatedAt != after[1].UpdatedAt || detail.Chapters[0].Slug != "b" || detail.Chapters[1].UpdatedAt != 100 || detail.Chapters[0].UpdatedAt != detail.UpdatedAt {
		t.Fatal(detail)
	}
	s.put(t, "columns/book/b.md", "another edit")
	requireOK(t, s.columns.UpdateColumn(ctx, col.Slug, nil, nil, &list))
	revisions := s.chapters(t)
	if revisions[0].UpdatedAt <= after[1].UpdatedAt || revisions[1].UpdatedAt != 100 {
		t.Fatal(revisions)
	}
	bad := []series.Chapter{list[1], {Slug: "missing", File: "absent.md", Title: "missing"}}
	if err := s.columns.UpdateColumn(ctx, col.Slug, nil, nil, &bad); err == nil {
		t.Fatal("publish accepted missing chapter")
	}
	if rows := s.chapters(t); len(rows) != 2 || rows[0] != revisions[0] || rows[1] != revisions[1] {
		t.Fatal(rows)
	}
	requireOK(t, s.columns.WithdrawChapter(ctx, col.Slug, "b"))
	detail, err = s.columns.GetColumnBySlug(ctx, col.Slug)
	requireOK(t, err)
	if detail.UpdatedAt != 100 || len(detail.Chapters) != 1 {
		t.Fatal(detail)
	}
}

func TestRecentPostsUseUpdatedBodyBeforeLimit(t *testing.T) {
	s := newPublishedSite(t)
	ctx := context.Background()
	var firstID int64
	for i, date := range []string{"2020-01-01", "2026-01-01"} {
		dir := fmt.Sprintf("posts/p%d", i)
		s.put(t, dir+"/index.md", "original")
		row := post.Blog{Title: dir, Path: dir, Date: date}
		requireOK(t, s.blogs.CreateBlog(ctx, &row, nil))
		if i == 0 {
			firstID = row.ID
		}
		requireOK(t, s.db.Model(&row).UpdateColumn("updated_at", 100+i).Error)
	}
	s.put(t, "posts/p0/index.md", "updated")
	requireOK(t, s.blogs.SeedContentHashes(ctx))
	unchanged, _, err := s.blogs.ListBlogs(ctx, 1, 0)
	requireOK(t, err)
	if unchanged[0].ID == firstID {
		t.Fatal("disk edit changed post ranking")
	}
	requireOK(t, s.blogs.UpdateBlog(ctx, firstID, nil, nil))
	recent, _, err := s.blogs.ListBlogs(ctx, 1, 0)
	requireOK(t, err)
	if len(recent) != 1 || recent[0].Title != "posts/p0" || recent[0].UpdatedAt <= 101 {
		t.Fatal(recent)
	}
	requireOK(t, s.blogs.SeedContentHashes(ctx))
	tags := []string{}
	requireOK(t, s.blogs.UpdateBlog(ctx, recent[0].ID, map[string]any{"title": recent[0].Title}, &tags))
	next, _, err := s.blogs.ListBlogs(ctx, 1, 0)
	requireOK(t, err)
	if next[0].UpdatedAt != recent[0].UpdatedAt {
		t.Fatalf("republish changed timestamp: %v", next)
	}
}

func TestStaticFilesFollowPublication(t *testing.T) {
	s := newPublishedSite(t)
	ctx := context.Background()
	s.put(t, "posts/p/index.md", "![](./image.png)")
	s.put(t, "posts/draft/index.md", "Draft")
	s.put(t, "columns/book/a.md", "![](./a/plot.png)")
	s.put(t, "columns/book/draft.md", "![](./draft/plot.png)")
	for _, name := range []string{"posts/p/image.png", "posts/p/cover.png", "posts/draft/image.png", "columns/book/a/plot.png", "columns/book/draft/plot.png"} {
		s.put(t, name, "image bytes")
	}
	row := post.Blog{Title: "post", Path: "./posts/p/", Cover: "./cover.png", Date: "2026-01-01"}
	requireOK(t, s.blogs.CreateBlog(ctx, &row, nil))
	col := series.Column{Slug: "book", Path: "columns/book", Title: "Book"}
	requireOK(t, s.columns.CreateColumn(ctx, &col, nil, []series.Chapter{{Slug: "a", Title: "A", File: "a.md"}}))
	h := HttpServer{engine: gin.New(), blogService: blogservice.NewBlogService(s.blogs), columnService: columnservice.NewColumnService(s.columns)}
	cfg := &config.Config{}
	cfg.Content.RepoDir = s.root
	h.registerRoutes(cfg)
	check := func(file string, want int) {
		t.Helper()
		for _, method := range []string{"GET", "HEAD"} {
			w := httptest.NewRecorder()
			h.engine.ServeHTTP(w, httptest.NewRequest(method, "/static/"+file, nil))
			if w.Code != want || (want == http.StatusOK && method == "GET" && !bytes.Equal(w.Body.Bytes(), []byte("image bytes"))) {
				t.Fatalf("%s %s: %d %q", method, file, w.Code, w.Body.String())
			}
		}
	}
	for _, file := range []string{"posts/p/image.png", "posts/p/cover.png", "columns/book/a/plot.png"} {
		check(file, 200)
	}
	for _, file := range []string{"posts/p/index.md", "posts/draft/index.md", "posts/draft/image.png", "columns/book/draft.md", "columns/book/draft/plot.png", "columns/book/"} {
		check(file, 404)
	}
	requireOK(t, s.blogs.DeleteBlog(ctx, row.ID))
	check("posts/p/image.png", 404)
	requireOK(t, s.columns.DeleteColumn(ctx, col.Slug))
	check("columns/book/a/plot.png", 404)
}
