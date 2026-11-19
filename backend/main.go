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
	if strings.TrimSpace(os.Getenv("ADMIN_PASSCODE")) != "" && len(strings.TrimSpace(os.Getenv("ADMIN_PASSCODE"))) < 12 {
		log.Printf("WARNING: ADMIN_PASSCODE is shorter than 12 characters")
	}
	if sec := strings.TrimSpace(os.Getenv("APP_SECRET")); sec != "" &&
		(len(sec) < 32 || sec == "replace-with-a-random-secret" || sec == "psychgrader-secret-change-me") {
		log.Fatal("APP_SECRET must be a random value of at least 32 characters (refusing to start with a placeholder/weak secret)")
	}
	if strings.TrimSpace(os.Getenv("ADMIN_PASSCODE")) == "" {
		switch host {
		case "127.0.0.1", "localhost", "::1":
			log.Printf("WARNING: ADMIN_PASSCODE is empty; auth is disabled on loopback only")
		default:
			log.Fatal("ADMIN_PASSCODE must be set when HOST is not loopback (refusing to expose an unauthenticated instance)")
		}
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
	// Gin trusts all proxies by default, which lets clients spoof
	// X-Forwarded-For and bypass the login limiter. The app is meant to sit
	// behind one reverse proxy; configure TRUSTED_PROXIES (CIDR list) only when
	// that proxy is the sole ingress.
	if tp := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); tp != "" {
		var cidrs []string
		for _, item := range strings.Split(tp, ",") {
			if item = strings.TrimSpace(item); item != "" {
				cidrs = append(cidrs, item)
			}
		}
		_ = r.SetTrustedProxies(cidrs)
	} else {
		_ = r.SetTrustedProxies(nil)
	}

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
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		if xfp := c.GetHeader("X-Forwarded-Proto"); xfp != "" {
			scheme = strings.ToLower(strings.TrimSpace(strings.Split(xfp, ",")[0]))
		}
		sameOrigin := origin == scheme+"://"+c.Request.Host
		if origin != "" {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Vary", "Origin")
			if allowedOrigins[origin] || sameOrigin {
				if !sameOrigin {
					c.Header("Access-Control-Allow-Origin", origin)
				}
			} else {
				// CORS controls reads, not whether a cross-site POST is sent.
				// Reject non-GET cross-origin writes explicitly.
				if c.Request.Method != http.MethodOptions && c.Request.Method != http.MethodGet {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "跨站请求被拒绝"})
					return
				}
			}
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}

		// CSRF hardening for write endpoints: require JSON, except multipart
		// PDF upload. This blocks simple form / text-plain cross-site POSTs.
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead &&
			c.Request.Method != http.MethodOptions {
			ct := c.ContentType()
			if strings.Contains(c.Request.URL.Path, "/parse-pdf") {
				if !strings.HasPrefix(ct, "multipart/form-data") {
					c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "PDF 上传必须使用 multipart/form-data"})
					return
				}
			} else if !strings.HasPrefix(ct, "application/json") {
				c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "写接口必须使用 application/json"})
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
