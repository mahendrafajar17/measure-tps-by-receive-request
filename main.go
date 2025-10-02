package main

import (
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type WebhookConfig struct {
	StatusCode     int               `json:"status_code"`
	ContentType    string            `json:"content_type"`
	ResponseBody   string            `json:"response_body"`
	Timeout        int               `json:"timeout"` // in milliseconds
	Headers        map[string]string `json:"headers"`
	EnableLogging  bool              `json:"enable_logging"`
}

type Webhook struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Config      WebhookConfig  `json:"config"`
	Calculator  *TPSCalculator `json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
	LastRequest *time.Time     `json:"last_request,omitempty"`
}


type TPSCalculator struct {
	mu           sync.RWMutex
	requestCount int64
	startTime    time.Time
	lastTime     time.Time
	isActive     bool
}

type WebhookServer struct {
	webhooks map[string]*Webhook
	mu       sync.RWMutex
	router   *gin.Engine
}

func NewTPSCalculator() *TPSCalculator {
	return &TPSCalculator{}
}

func NewWebhookServer(router *gin.Engine) *WebhookServer {
	server := &WebhookServer{
		webhooks: make(map[string]*Webhook),
		router:   router,
	}

	// Create default webhooks
	defaultWebhook := &Webhook{
		ID:   "default",
		Name: "Default Webhook",
		Path: "/webhook",
		Config: WebhookConfig{
			StatusCode:    200,
			ContentType:   "application/json",
			ResponseBody:  `{"message": "Request received"}`,
			Timeout:       0,
			Headers:       make(map[string]string),
			EnableLogging: true,
		},
		Calculator: NewTPSCalculator(),
		CreatedAt:  time.Now(),
	}

	fastWebhook := &Webhook{
		ID:   "fast",
		Name: "Fast Webhook",
		Path: "/webhook/fast",
		Config: WebhookConfig{
			StatusCode:    200,
			ContentType:   "application/json",
			ResponseBody:  `{"message": "Fast response", "delay": "0ms"}`,
			Timeout:       0,
			Headers:       make(map[string]string),
			EnableLogging: false, // Disable for max performance
		},
		Calculator: NewTPSCalculator(),
		CreatedAt:  time.Now(),
	}

	slowWebhook := &Webhook{
		ID:   "slow",
		Name: "Slow Webhook",
		Path: "/webhook/slow",
		Config: WebhookConfig{
			StatusCode:    200,
			ContentType:   "application/json",
			ResponseBody:  `{"message": "Slow response", "delay": "1000ms"}`,
			Timeout:       1000, // 1 second
			Headers:       make(map[string]string),
			EnableLogging: true,
		},
		Calculator: NewTPSCalculator(),
		CreatedAt:  time.Now(),
	}

	server.webhooks["default"] = defaultWebhook
	server.webhooks["fast"] = fastWebhook
	server.webhooks["slow"] = slowWebhook

	return server
}

func (ws *WebhookServer) createWebhook(name, path string, config WebhookConfig) *Webhook {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Generate unique ID
	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]

	// Use custom path if provided, otherwise use /w/{id}
	finalPath := path
	if finalPath == "" {
		finalPath = "/w/" + id
	} else {
		// Ensure path starts with /
		if !strings.HasPrefix(finalPath, "/") {
			finalPath = "/" + finalPath
		}
	}

	webhook := &Webhook{
		ID:         id,
		Name:       name,
		Path:       finalPath,
		Config:     config,
		Calculator: NewTPSCalculator(),
		CreatedAt:  time.Now(),
	}

	ws.webhooks[id] = webhook

	// Register the custom path route
	ws.registerWebhookRoute(webhook)

	return webhook
}

func (ws *WebhookServer) registerWebhookRoute(webhook *Webhook) {
	ws.router.Any(webhook.Path, func(c *gin.Context) {
		ws.handleWebhookRequest(webhook.ID, c)
	})
}

func (ws *WebhookServer) handleWebhookRequest(webhookID string, c *gin.Context) {
	webhook, exists := ws.getWebhook(webhookID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
		return
	}

	// Record request for metrics
	webhook.Calculator.RecordRequest()

	// Update last request time
	now := time.Now()
	webhook.LastRequest = &now

	// Conditional structured logging with logrus
	if webhook.Config.EnableLogging {
		logrus.WithFields(logrus.Fields{
			"webhook_id": webhookID,
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"ip":         c.ClientIP(),
			"user_agent": c.GetHeader("User-Agent"),
			"webhook":    webhook.Name,
		}).Info("Request received")
	}

	// Apply timeout if configured
	if webhook.Config.Timeout > 0 {
		time.Sleep(time.Duration(webhook.Config.Timeout) * time.Millisecond)
	}

	// Set custom headers
	for key, value := range webhook.Config.Headers {
		c.Header(key, value)
	}

	// Set content type and return response
	c.Header("Content-Type", webhook.Config.ContentType)
	c.String(webhook.Config.StatusCode, webhook.Config.ResponseBody)
}

func (ws *WebhookServer) getWebhook(id string) (*Webhook, bool) {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	webhook, exists := ws.webhooks[id]
	return webhook, exists
}

func (ws *WebhookServer) getAllWebhooks() []*Webhook {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	webhooks := make([]*Webhook, 0, len(ws.webhooks))
	for _, webhook := range ws.webhooks {
		webhooks = append(webhooks, webhook)
	}
	return webhooks
}

func (ws *WebhookServer) deleteWebhook(id string) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	// Don't allow deleting default webhooks
	if id == "default" || id == "fast" || id == "slow" {
		return false
	}

	if _, exists := ws.webhooks[id]; exists {
		delete(ws.webhooks, id)
		return true
	}
	return false
}


func (t *TPSCalculator) RecordRequest() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()

	if !t.isActive {
		t.startTime = now
		t.isActive = true
	}

	t.requestCount++
	t.lastTime = now
}

func (t *TPSCalculator) GetMetrics() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.isActive {
		return map[string]interface{}{
			"total_requests":   0,
			"duration_seconds": 0,
			"tps":              0,
			"start_time":       nil,
			"end_time":         nil,
		}
	}

	duration := t.lastTime.Sub(t.startTime).Seconds()
	var tps float64
	if duration > 0 {
		tps = float64(t.requestCount) / duration
	}

	return map[string]interface{}{
		"total_requests":   t.requestCount,
		"duration_seconds": duration,
		"tps":              tps,
		"start_time":       t.startTime.Format(time.RFC3339),
		"end_time":         t.lastTime.Format(time.RFC3339),
	}
}

func (t *TPSCalculator) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.requestCount = 0
	t.startTime = time.Time{}
	t.lastTime = time.Time{}
	t.isActive = false
}

func main() {
	// Setup logrus for dual output (console + file)
	logFile, err := os.OpenFile("webhook.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logrus.Fatalln("Failed to open log file:", err)
	}
	
	// Dual output: console AND file
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	logrus.SetOutput(multiWriter)
	
	// Set log format
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	
	logrus.Info("🎯 Multi-Webhook Server initializing...")

	r := gin.Default()
	webhookServer := NewWebhookServer(r)

	// Default webhook endpoints
	r.Any("/webhook", func(c *gin.Context) {
		webhookServer.handleWebhookRequest("default", c)
	})
	
	r.Any("/webhook/fast", func(c *gin.Context) {
		webhookServer.handleWebhookRequest("fast", c)
	})
	
	r.Any("/webhook/slow", func(c *gin.Context) {
		webhookServer.handleWebhookRequest("slow", c)
	})

	// Dynamic webhook handler for /w/{id} pattern (fallback for webhooks without custom path)
	r.Any("/w/:id", func(c *gin.Context) {
		webhookID := c.Param("id")
		webhookServer.handleWebhookRequest(webhookID, c)
	})

	// Webhook management endpoints
	r.GET("/api/webhooks", func(c *gin.Context) {
		webhooks := webhookServer.getAllWebhooks()
		c.JSON(http.StatusOK, webhooks)
	})

	r.POST("/api/webhooks", func(c *gin.Context) {
		var req struct {
			Name   string        `json:"name" binding:"required"`
			Path   string        `json:"path"`
			Config WebhookConfig `json:"config"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Set defaults for config
		if req.Config.StatusCode == 0 {
			req.Config.StatusCode = 200
		}
		if req.Config.ContentType == "" {
			req.Config.ContentType = "application/json"
		}
		if req.Config.ResponseBody == "" {
			req.Config.ResponseBody = `{"message": "Request received"}`
		}
		if req.Config.Headers == nil {
			req.Config.Headers = make(map[string]string)
		}
		// EnableLogging defaults to true if not specified

		webhook := webhookServer.createWebhook(req.Name, req.Path, req.Config)
		c.JSON(http.StatusCreated, webhook)
	})

	r.GET("/api/webhooks/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhook, exists := webhookServer.getWebhook(id)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
			return
		}
		c.JSON(http.StatusOK, webhook)
	})

	r.PUT("/api/webhooks/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhook, exists := webhookServer.getWebhook(id)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
			return
		}

		var updateReq struct {
			Name   string        `json:"name"`
			Config WebhookConfig `json:"config"`
		}

		if err := c.ShouldBindJSON(&updateReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		webhookServer.mu.Lock()
		if updateReq.Name != "" {
			webhook.Name = updateReq.Name
		}
		if updateReq.Config.StatusCode != 0 {
			webhook.Config = updateReq.Config
		}
		webhookServer.mu.Unlock()

		c.JSON(http.StatusOK, webhook)
	})

	r.DELETE("/api/webhooks/:id", func(c *gin.Context) {
		id := c.Param("id")
		if webhookServer.deleteWebhook(id) {
			c.JSON(http.StatusOK, gin.H{"message": "Webhook deleted"})
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found or cannot delete default webhook"})
		}
	})

	// Metrics endpoints
	r.GET("/api/webhooks/:id/metrics", func(c *gin.Context) {
		id := c.Param("id")
		webhook, exists := webhookServer.getWebhook(id)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
			return
		}
		metrics := webhook.Calculator.GetMetrics()
		c.JSON(http.StatusOK, metrics)
	})

	r.POST("/api/webhooks/:id/reset", func(c *gin.Context) {
		id := c.Param("id")
		webhook, exists := webhookServer.getWebhook(id)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
			return
		}
		webhook.Calculator.Reset()
		c.JSON(http.StatusOK, gin.H{"message": "Metrics reset"})
	})

	// Request logs endpoints (disabled - using console logging only)
	r.GET("/api/requests", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Request logging disabled - check console for logs",
			"logs":    []interface{}{},
		})
	})

	r.DELETE("/api/requests", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "Request logging disabled - no logs to clear",
		})
	})

	// Legacy endpoints for backward compatibility
	r.GET("/api/config", func(c *gin.Context) {
		webhook, _ := webhookServer.getWebhook("default")
		c.JSON(http.StatusOK, webhook.Config)
	})

	r.POST("/api/config", func(c *gin.Context) {
		var newConfig WebhookConfig
		if err := c.ShouldBindJSON(&newConfig); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		webhook, _ := webhookServer.getWebhook("default")
		webhookServer.mu.Lock()
		webhook.Config = newConfig
		webhookServer.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{"message": "Configuration updated"})
	})

	r.POST("/api/request", func(c *gin.Context) {
		webhook, _ := webhookServer.getWebhook("default")
		webhook.Calculator.RecordRequest()
		c.JSON(http.StatusOK, gin.H{
			"message":   "Request recorded",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/api/metrics", func(c *gin.Context) {
		webhook, _ := webhookServer.getWebhook("default")
		metrics := webhook.Calculator.GetMetrics()
		c.JSON(http.StatusOK, metrics)
	})

	r.POST("/api/reset", func(c *gin.Context) {
		webhook, _ := webhookServer.getWebhook("default")
		webhook.Calculator.Reset()
		c.JSON(http.StatusOK, gin.H{
			"message": "Metrics reset",
		})
	})

	// Serve static files for web interface
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/index.html")
	})

	logrus.Info("🎯 Multi-Webhook Server starting on :8080")
	logrus.Info("📱 Web interface: http://localhost:8080")
	logrus.Info("📋 Log file: webhook.log")
	logrus.Info("")
	logrus.Info("🔗 Default Webhooks:")
	logrus.Info("   • Standard: http://localhost:8080/webhook")
	logrus.Info("   • Fast (0ms): http://localhost:8080/webhook/fast")
	logrus.Info("   • Slow (1s): http://localhost:8080/webhook/slow")
	logrus.Info("")
	logrus.Info("🔗 Custom webhooks: http://localhost:8080/w/{webhook-id}")
	logrus.Info("📊 API docs: http://localhost:8080/api/webhooks")
	r.Run(":8080")
}
