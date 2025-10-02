package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type TPSCalculator struct {
	mu           sync.RWMutex
	requestCount int64
	startTime    time.Time
	lastTime     time.Time
	isActive     bool
}

func NewTPSCalculator() *TPSCalculator {
	return &TPSCalculator{}
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
			"total_requests": 0,
			"duration_seconds": 0,
			"tps": 0,
			"start_time": nil,
			"end_time": nil,
		}
	}

	duration := t.lastTime.Sub(t.startTime).Seconds()
	var tps float64
	if duration > 0 {
		tps = float64(t.requestCount) / duration
	}

	return map[string]interface{}{
		"total_requests": t.requestCount,
		"duration_seconds": duration,
		"tps": tps,
		"start_time": t.startTime.Format(time.RFC3339),
		"end_time": t.lastTime.Format(time.RFC3339),
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
	calculator := NewTPSCalculator()
	
	r := gin.Default()

	r.POST("/api/request", func(c *gin.Context) {
		calculator.RecordRequest()
		c.JSON(http.StatusOK, gin.H{
			"message": "Request recorded",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/api/metrics", func(c *gin.Context) {
		metrics := calculator.GetMetrics()
		c.JSON(http.StatusOK, metrics)
	})

	r.POST("/api/reset", func(c *gin.Context) {
		calculator.Reset()
		c.JSON(http.StatusOK, gin.H{
			"message": "Metrics reset",
		})
	})

	r.Run(":8080")
}