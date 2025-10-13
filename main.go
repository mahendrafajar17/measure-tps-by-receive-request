package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

type WebhookConfig struct {
	StatusCode    int               `json:"status_code" yaml:"status_code"`
	ContentType   string            `json:"content_type" yaml:"content_type"`
	ResponseBody  string            `json:"response_body" yaml:"response_body"`
	Timeout       int               `json:"timeout" yaml:"timeout"` // in milliseconds
	Headers       map[string]string `json:"headers" yaml:"headers"`
	EnableLogging bool              `json:"enable_logging" yaml:"enable_logging"`
}

type Webhook struct {
	ID          string         `json:"id" yaml:"id"`
	Name        string         `json:"name" yaml:"name"`
	Path        string         `json:"path" yaml:"path"`
	Config      WebhookConfig  `json:"config" yaml:"config"`
	Calculator  *TPSCalculator `json:"-" yaml:"-"`
	CreatedAt   time.Time      `json:"created_at" yaml:"created_at"`
	LastRequest *time.Time     `json:"last_request,omitempty" yaml:"last_request,omitempty"`
}

type WebhookConfigFile struct {
	Server struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`
	Logging struct {
		LogFile   string `yaml:"log_file"`
		LogLevel  string `yaml:"log_level"`
		LogFormat string `yaml:"log_format"`
	} `yaml:"logging"`
	DefaultWebhooks []struct {
		ID     string        `yaml:"id"`
		Name   string        `yaml:"name"`
		Path   string        `yaml:"path"`
		Config WebhookConfig `yaml:"config"`
	} `yaml:"default_webhooks"`
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

func loadConfigFromYAML(filename string) (*WebhookConfigFile, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config WebhookConfigFile
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func NewWebhookServer(router *gin.Engine) (*WebhookServer, *WebhookConfigFile) {
	server := &WebhookServer{
		webhooks: make(map[string]*Webhook),
		router:   router,
	}

	// Try to load from config.yaml first
	config, err := loadConfigFromYAML("config.yaml")
	if err != nil {
		logrus.Warnf("Could not load config.yaml: %v, using default configuration", err)
		// Use default configuration if YAML file not found
		server.loadDefaultWebhooks()
		// Return default config
		defaultConfig := &WebhookConfigFile{
			Server: struct {
				Port int    `yaml:"port"`
				Host string `yaml:"host"`
			}{
				Port: 8080,
				Host: "localhost",
			},
		}
		return server, defaultConfig
	} else {
		logrus.Info("Loading webhooks from config.yaml")
		server.loadWebhooksFromConfig(config)
		// Set defaults if not specified
		if config.Server.Port == 0 {
			config.Server.Port = 8080
		}
		if config.Server.Host == "" {
			config.Server.Host = "localhost"
		}
		return server, config
	}
}

func (ws *WebhookServer) loadDefaultWebhooks() {
	// Create default webhooks (fallback)
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
			EnableLogging: false,
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
			ResponseBody:  `{"message": "Slow response", "delay": "2000ms"}`,
			Timeout:       2000,
			Headers:       make(map[string]string),
			EnableLogging: true,
		},
		Calculator: NewTPSCalculator(),
		CreatedAt:  time.Now(),
	}

	ws.webhooks["default"] = defaultWebhook
	ws.webhooks["fast"] = fastWebhook
	ws.webhooks["slow"] = slowWebhook
}

func (ws *WebhookServer) loadWebhooksFromConfig(config *WebhookConfigFile) {
	for _, webhookConfig := range config.DefaultWebhooks {
		if webhookConfig.Config.Headers == nil {
			webhookConfig.Config.Headers = make(map[string]string)
		}
		
		webhook := &Webhook{
			ID:         webhookConfig.ID,
			Name:       webhookConfig.Name,
			Path:       webhookConfig.Path,
			Config:     webhookConfig.Config,
			Calculator: NewTPSCalculator(),
			CreatedAt:  time.Now(),
		}
		
		ws.webhooks[webhookConfig.ID] = webhook
		
		// Register route for this webhook
		ws.registerWebhookRoute(webhook)
	}
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

	// Read and log request body if logging is enabled
	var requestBody string
	var requestHeaders map[string][]string
	if webhook.Config.EnableLogging {
		// Read request body
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err == nil {
			requestBody = string(bodyBytes)
			// Restore the request body for further processing
			c.Request.Body = io.NopCloser(strings.NewReader(requestBody))
		}
		
		// Copy request headers
		requestHeaders = make(map[string][]string)
		for key, values := range c.Request.Header {
			requestHeaders[key] = values
		}

		// Log request details
		logrus.WithFields(logrus.Fields{
			"webhook_id":      webhookID,
			"method":          c.Request.Method,
			"path":            c.Request.URL.Path,
			"query_params":    c.Request.URL.RawQuery,
			"ip":              c.ClientIP(),
			"user_agent":      c.GetHeader("User-Agent"),
			"webhook":         webhook.Name,
			"request_headers": requestHeaders,
			"request_body":    requestBody,
			"content_length":  c.Request.ContentLength,
		}).Info("Request received")
	}

	// Apply timeout if configured
	if webhook.Config.Timeout > 0 {
		time.Sleep(time.Duration(webhook.Config.Timeout) * time.Millisecond)
	}

	// Set custom headers
	responseHeaders := make(map[string]string)
	for key, value := range webhook.Config.Headers {
		c.Header(key, value)
		responseHeaders[key] = value
	}

	// Set content type and prepare response
	c.Header("Content-Type", webhook.Config.ContentType)
	responseHeaders["Content-Type"] = webhook.Config.ContentType

	// Send response
	c.String(webhook.Config.StatusCode, webhook.Config.ResponseBody)

	// Log response details if logging is enabled
	if webhook.Config.EnableLogging {
		logrus.WithFields(logrus.Fields{
			"webhook_id":       webhookID,
			"webhook":          webhook.Name,
			"response_status":  webhook.Config.StatusCode,
			"response_headers": responseHeaders,
			"response_body":    webhook.Config.ResponseBody,
			"processing_time":  time.Since(now).String(),
		}).Info("Response sent")
	}
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
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	logrus.Info("🎯 Multi-Webhook Server initializing...")

	r := gin.Default()
	webhookServer, config := NewWebhookServer(r)

	// Note: Webhook routes are now registered automatically from YAML config

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

	// Summary endpoint for all webhooks
	r.GET("/api/summary", func(c *gin.Context) {
		webhooks := webhookServer.getAllWebhooks()
		summary := make(map[string]interface{})
		
		for _, webhook := range webhooks {
			metrics := webhook.Calculator.GetMetrics()
			summary[webhook.ID] = map[string]interface{}{
				"name":            webhook.Name,
				"path":            webhook.Path,
				"delay_ms":        webhook.Config.Timeout,
				"total_requests":  metrics["total_requests"],
				"tps":             metrics["tps"],
				"duration_seconds": metrics["duration_seconds"],
			}
		}
		
		c.JSON(http.StatusOK, gin.H{
			"summary": summary,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Serve static files for web interface
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/static/index.html")
	})

	// Use port from config
	serverAddr := fmt.Sprintf(":%d", config.Server.Port)
	baseURL := fmt.Sprintf("http://%s:%d", config.Server.Host, config.Server.Port)
	
	logrus.Infof("🎯 Multi-Webhook Server starting on %s", serverAddr)
	logrus.Infof("📱 Web interface: %s", baseURL)
	logrus.Info("📋 Log file: webhook.log")
	logrus.Info("")
	logrus.Info("🔗 Default Webhooks:")
	logrus.Infof("   • Standard: %s/webhook", baseURL)
	logrus.Infof("   • Fast (0ms): %s/webhook/fast", baseURL)
	logrus.Infof("   • Slow (2s): %s/webhook/slow", baseURL)
	logrus.Info("")
	logrus.Infof("🔗 Custom webhooks: %s/w/{webhook-id}", baseURL)
	logrus.Infof("📊 API docs: %s/api/webhooks", baseURL)
	r.Run(serverAddr)
}
