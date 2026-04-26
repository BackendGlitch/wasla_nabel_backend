package middleware

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var (
	redisClient *redis.Client
	redisOnce   sync.Once
)

func getRedisClient() *redis.Client {
	redisOnce.Do(func() {
		redisURL := os.Getenv("REDIS_URL")
		if redisURL == "" {
			redisURL = "redis://localhost:6379"
		}

		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			fmt.Printf("middleware: failed to parse REDIS_URL: %v\n", err)
			return
		}

		redisClient = redis.NewClient(opt)
	})
	return redisClient
}

func isSessionValid(staffID, token string) bool {
	client := getRedisClient()
	if client == nil {
		// Fail closed for safety: if we cannot validate the session, treat it as invalid
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	storedToken, err := client.Get(ctx, fmt.Sprintf("session:%s", staffID)).Result()
	if err != nil {
		// Session missing or Redis error -> treat as invalid
		return false
	}

	return storedToken == token
}

// CORS middleware
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, "+MachineTypeHeader+", "+MachineIDHeader)
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// Logger middleware
func Logger() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Skip logging User-Agent for health check endpoints to prevent random data in tickets
		var userAgent string
		if param.Path == "/health" {
			userAgent = "-" // Use dash instead of actual User-Agent for health checks
		} else {
			userAgent = param.Request.UserAgent()
		}

		return fmt.Sprintf("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"\n",
			param.ClientIP,
			param.TimeStamp.Format("02/Jan/2006:15:04:05 -0700"),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			userAgent,
			param.ErrorMessage,
		)
	})
}

// AuthRequired middleware
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		var token string
		if authHeader != "" {
			// Extract token from "Bearer <token>"
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				token = tokenParts[1]
			}
		}

		// Fallback: allow token via query param for WebSocket/browser cases
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			utils.UnauthorizedResponse(c, "Authorization header or token query required")
			c.Abort()
			return
		}

		secretKey := os.Getenv("JWT_SECRET_KEY")
		if secretKey == "" {
			secretKey = "your-secret-key-change-this-in-production"
		}

		claims, err := utils.ValidateJWT(token, secretKey)
		if err != nil {
			utils.UnauthorizedResponse(c, "Invalid token")
			c.Abort()
			return
		}

		// Extract staff ID from claims
		staffID, ok := claims["staff_id"].(string)
		if !ok || staffID == "" {
			utils.UnauthorizedResponse(c, "Invalid token claims")
			c.Abort()
			return
		}

		// Validate session in Redis so that logout actually invalidates the token.
		// If the session is missing or the token doesn't match, force re-login.
		if !isSessionValid(staffID, token) {
			utils.UnauthorizedResponse(c, "Session expired or invalid, please login again")
			c.Abort()
			return
		}

		// Set staff ID in context for use in handlers
		c.Set("staff_id", staffID)

		c.Next()
	}
}
