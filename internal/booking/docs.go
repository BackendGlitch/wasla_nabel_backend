package booking

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterDocsRoutes mounts a minimal OpenAPI and Swagger UI page
func RegisterDocsRoutes(r *gin.Engine) {
	r.GET("/openapi.json", func(c *gin.Context) {
		c.JSON(http.StatusOK, bookingOpenAPISpec())
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, swaggerHTML("/openapi.json"))
	})
}

func bookingOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":   "Station Booking Service API",
			"version": "1.0.0",
		},
		"paths": map[string]interface{}{
			"/api/v1/bookings": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":  "Create booking",
					"security": []map[string]interface{}{{"bearerAuth": []interface{}{}}},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"destinationId": map[string]interface{}{"type": "string"},
										"seats":         map[string]interface{}{"type": "integer"},
										"subRoute":      map[string]interface{}{"type": "string"},
									},
									"required": []string{"destinationId", "seats"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{"description": "Created"},
					},
				},
			},
			"/api/v1/bookings/{id}/cancel": map[string]interface{}{
				"put": map[string]interface{}{
					"summary":  "Cancel booking",
					"security": []map[string]interface{}{{"bearerAuth": []interface{}{}}},
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"reason": map[string]interface{}{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Cancelled"},
					},
				},
			},
			"/api/v1/trips": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":  "List trips",
					"security": []map[string]interface{}{{"bearerAuth": []interface{}{}}},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "OK"},
					},
				},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		},
	}
}

func swaggerHTML(specURL string) string {
	return `<!doctype html><html><head><meta charset="utf-8"/><title>API Docs</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head><body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>window.ui=SwaggerUIBundle({url: '` + specURL + `', dom_id:'#swagger-ui'});</script>
</body></html>`
}
