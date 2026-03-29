package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/yuin/goldmark"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed content/blog/*.md
var blogFiles embed.FS

type BlogMeta struct {
	Title   string   `yaml:"title" json:"title"`
	Date    string   `yaml:"date" json:"date"`
	Excerpt string   `yaml:"excerpt" json:"excerpt"`
	Tags    []string `yaml:"tags" json:"tags"`
	Author  string   `yaml:"author" json:"author"`
}

type BlogPost struct {
	BlogMeta
	Slug    string `json:"slug"`
	Content string `json:"content,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/blog", handleBlogList)
	mux.HandleFunc("GET /api/blog/{slug}", handleBlogPost)

	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("failed to create static sub filesystem: %v", err)
	}

	fileServer := http.FileServer(http.FS(staticSub))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if path != "/" {
			cleanPath := strings.TrimPrefix(path, "/")
			if _, err := fs.Stat(staticSub, cleanPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexData, err := fs.ReadFile(staticSub, "index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexData)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Portfolio server starting on :%s", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func loadBlogPosts(includeContent bool) ([]BlogPost, error) {
	entries, err := fs.ReadDir(blogFiles, "content/blog")
	if err != nil {
		return nil, fmt.Errorf("reading blog directory: %w", err)
	}

	var posts []BlogPost
	md := goldmark.New()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}

		data, err := fs.ReadFile(blogFiles, "content/blog/"+entry.Name())
		if err != nil {
			continue
		}

		var meta BlogMeta
		body, err := frontmatter.Parse(bytes.NewReader(data), &meta)
		if err != nil {
			continue
		}

		slug := strings.TrimSuffix(entry.Name(), ".md")

		post := BlogPost{
			BlogMeta: meta,
			Slug:     slug,
		}

		if includeContent {
			var buf bytes.Buffer
			if err := md.Convert(body, &buf); err == nil {
				post.Content = buf.String()
			}
		}

		posts = append(posts, post)
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date > posts[j].Date
	})

	return posts, nil
}

func handleBlogList(w http.ResponseWriter, r *http.Request) {
	posts, err := loadBlogPosts(false)
	if err != nil {
		http.Error(w, "failed to load posts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posts)
}

func handleBlogPost(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "slug required", http.StatusBadRequest)
		return
	}

	posts, err := loadBlogPosts(true)
	if err != nil {
		http.Error(w, "failed to load posts", http.StatusInternalServerError)
		return
	}

	for _, post := range posts {
		if post.Slug == slug {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(post)
			return
		}
	}

	http.Error(w, "post not found", http.StatusNotFound)
}
