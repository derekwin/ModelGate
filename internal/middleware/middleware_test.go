package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimit_NilLimiterSkips(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	m := NewAuthMiddleware(nil)

	r.Use(m.RateLimit())
	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestBodySizeLimit_RejectsLargeContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodySizeLimit(8))
	r.POST("/upload", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("123456789"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestBodySizeLimit_RejectsLargeChunkedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(BodySizeLimit(8))
	r.POST("/upload", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("123456789"))
	req.ContentLength = -1
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
