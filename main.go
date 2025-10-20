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

// Custom panic recovery middleware
func panicRecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logrus.WithFields(logrus.Fields{
					"method": c.Request.Method,
					"path":   c.Request.URL.Path,
					"error":  err,
				}).Error("Panic recovered in HTTP handler")
				
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
					"message": "An unexpected error occurred",
				})
				c.Abort()
			}
		}()
		c.Next()
	}
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
	
	// Add custom panic recovery middleware
	r.Use(panicRecoveryMiddleware())
	
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
			Path   string        `json:"path"`
			Config WebhookConfig `json:"config"`
		}

		if err := c.ShouldBindJSON(&updateReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate webhook is not nil
		if webhook == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Webhook data is corrupted"})
			return
		}

		webhookServer.mu.Lock()
		defer webhookServer.mu.Unlock()
		
		// Double-check webhook still exists after acquiring lock
		webhook, exists = webhookServer.webhooks[id]
		if !exists || webhook == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found or has been deleted"})
			return
		}
		
		// Update name if provided
		if updateReq.Name != "" {
			webhook.Name = updateReq.Name
		}
		
		// Update path if provided (but don't allow changing default webhook paths)
		if updateReq.Path != "" && id != "default" && id != "fast" && id != "slow" {
			// Ensure path starts with /
			if !strings.HasPrefix(updateReq.Path, "/") {
				updateReq.Path = "/" + updateReq.Path
			}
			webhook.Path = updateReq.Path
			// Note: Route re-registration is not supported in Gin after server starts
			// Path changes will take effect on next server restart
		}
		
		// Update config - merge with existing config
		if updateReq.Config.StatusCode != 0 {
			webhook.Config.StatusCode = updateReq.Config.StatusCode
		}
		if updateReq.Config.ContentType != "" {
			webhook.Config.ContentType = updateReq.Config.ContentType
		}
		if updateReq.Config.ResponseBody != "" {
			webhook.Config.ResponseBody = updateReq.Config.ResponseBody
		}
		if updateReq.Config.Timeout >= 0 {
			webhook.Config.Timeout = updateReq.Config.Timeout
		}
		if updateReq.Config.Headers != nil {
			if webhook.Config.Headers == nil {
				webhook.Config.Headers = make(map[string]string)
			}
			for key, value := range updateReq.Config.Headers {
				webhook.Config.Headers[key] = value
			}
		}
		// Update logging setting
		webhook.Config.EnableLogging = updateReq.Config.EnableLogging

		c.JSON(http.StatusOK, webhook)
	})

	r.PATCH("/api/webhooks/:id", func(c *gin.Context) {
		id := c.Param("id")
		webhook, exists := webhookServer.getWebhook(id)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found"})
			return
		}

		var patchReq struct {
			Name   *string `json:"name"`
			Path   *string `json:"path"`
			Config *struct {
				StatusCode    *int               `json:"status_code"`
				ContentType   *string            `json:"content_type"`
				ResponseBody  *string            `json:"response_body"`
				Timeout       *int               `json:"timeout"`
				Headers       map[string]string  `json:"headers"`
				EnableLogging *bool              `json:"enable_logging"`
			} `json:"config"`
		}

		if err := c.ShouldBindJSON(&patchReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate webhook is not nil
		if webhook == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Webhook data is corrupted"})
			return
		}

		webhookServer.mu.Lock()
		defer webhookServer.mu.Unlock()
		
		// Double-check webhook still exists after acquiring lock
		webhook, exists = webhookServer.webhooks[id]
		if !exists || webhook == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Webhook not found or has been deleted"})
			return
		}
		
		// Update name if provided
		if patchReq.Name != nil {
			webhook.Name = *patchReq.Name
		}
		
		// Update path if provided (but don't allow changing default webhook paths)
		if patchReq.Path != nil && id != "default" && id != "fast" && id != "slow" {
			newPath := *patchReq.Path
			// Ensure path starts with /
			if !strings.HasPrefix(newPath, "/") {
				newPath = "/" + newPath
			}
			webhook.Path = newPath
			// Note: Route re-registration is not supported in Gin after server starts
			// Path changes will take effect on next server restart
		}
		
		// Update config fields individually if provided
		if patchReq.Config != nil {
			if patchReq.Config.StatusCode != nil {
				webhook.Config.StatusCode = *patchReq.Config.StatusCode
			}
			if patchReq.Config.ContentType != nil {
				webhook.Config.ContentType = *patchReq.Config.ContentType
			}
			if patchReq.Config.ResponseBody != nil {
				webhook.Config.ResponseBody = *patchReq.Config.ResponseBody
			}
			if patchReq.Config.Timeout != nil {
				webhook.Config.Timeout = *patchReq.Config.Timeout
			}
			if patchReq.Config.Headers != nil {
				if webhook.Config.Headers == nil {
					webhook.Config.Headers = make(map[string]string)
				}
				for key, value := range patchReq.Config.Headers {
					webhook.Config.Headers[key] = value
				}
			}
			if patchReq.Config.EnableLogging != nil {
				webhook.Config.EnableLogging = *patchReq.Config.EnableLogging
			}
		}

		c.JSON(http.StatusOK, webhook)
	})

	r.PUT("/api/webhooks/bulk", func(c *gin.Context) {
		var bulkUpdateReq struct {
			Updates map[string]struct {
				Name   string        `json:"name"`
				Config WebhookConfig `json:"config"`
			} `json:"updates"`
		}

		if err := c.ShouldBindJSON(&bulkUpdateReq); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if bulkUpdateReq.Updates == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No updates provided"})
			return
		}

		updatedWebhooks := make(map[string]*Webhook)
		failedUpdates := make(map[string]string)
		
		webhookServer.mu.Lock()
		defer webhookServer.mu.Unlock()
		
		for webhookID, updateData := range bulkUpdateReq.Updates {
			webhook, exists := webhookServer.webhooks[webhookID]
			if !exists || webhook == nil {
				failedUpdates[webhookID] = "Webhook not found"
				continue
			}
			
			if updateData.Name != "" {
				webhook.Name = updateData.Name
			}
			if updateData.Config.StatusCode != 0 {
				webhook.Config = updateData.Config
			}
			updatedWebhooks[webhookID] = webhook
		}

		response := gin.H{
			"message": "Bulk update completed",
			"updated": updatedWebhooks,
		}
		
		if len(failedUpdates) > 0 {
			response["failed"] = failedUpdates
		}

		c.JSON(http.StatusOK, response)
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
