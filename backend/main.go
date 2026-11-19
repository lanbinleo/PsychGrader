package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" {
		// Local-first default. Docker sets HOST=0.0.0.0 behind its own
		// loopback port binding; never expose an unauthenticated instance by
		// accident on a LAN.
		host = "127.0.0.1"
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
	r.MaxMultipartMemory = 8 << 20

	// CORS is limited to the local Vite dev server and configured production
	// origins. Wildcard CORS plus an empty passcode would let any website read
	// the teacher's local data.
	allowedOrigins := map[string]bool{
		"http://localhost:5173": true,
		"http://127.0.0.1:5173": true,
	}
	for _, o := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if o = strings.TrimSpace(o); o != "" {
			allowedOrigins[o] = true
		}
	}
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Vary", "Origin")
			if allowedOrigins[origin] {
				c.Header("Access-Control-Allow-Origin", origin)
			}
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
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
			if reqPath == ".." || strings.HasPrefix(reqPath, ".."+string(os.PathSeparator)) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			fullPath := filepath.Join(staticDir, reqPath)
			if fi, err := os.Stat(fullPath); err == nil && !fi.IsDir() {
				c.File(fullPath)
				return
			}
			c.File(filepath.Join(staticDir, "index.html"))
		})
	}

	addr := net.JoinHostPort(host, port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("PsychGrader listening on http://%s (db=%s, static=%s, auth=%v)", addr, dbPath, staticDir, srv.Passcode != "")
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited: %v", err)
	}
}
