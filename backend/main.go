package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"psychgrader/internal/db"
	"psychgrader/internal/handlers"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./data/psychgrader.db"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		if _, err := os.Stat("./static/index.html"); err == nil {
			staticDir = "./static"
		} else {
			staticDir = "../frontend/dist"
		}
	}

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open sqlite db: %v", err)
	}
	defer store.DB.Close()

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	// Permissive CORS for local Vite dev server
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	srv := handlers.NewServer(store)
	srv.RegisterRoutes(r)

	// Serve built SPA frontend if present
	if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "API route not found"})
				return
			}
			reqPath := filepath.Clean(strings.TrimPrefix(c.Request.URL.Path, "/"))
			fullPath := filepath.Join(staticDir, reqPath)
			if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
				c.File(fullPath)
				return
			}
			c.File(filepath.Join(staticDir, "index.html"))
		})
	}

	log.Printf("PsychGrader listening on http://0.0.0.0:%s (db=%s, static=%s)", port, dbPath, staticDir)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
