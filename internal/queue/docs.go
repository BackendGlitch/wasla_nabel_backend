package queue

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func RegisterDocsRoutes(r *gin.Engine) {
	r.GET("/openapi-queue.json", func(c *gin.Context) {
		c.JSON(http.StatusOK, queueOpenAPISpec())
	})
	r.GET("/docs-queue", docsPage("/openapi-queue.json"))

	// Unified docs (queue + booking minimal endpoints in one spec)
	r.GET("/openapi-all.json", func(c *gin.Context) {
		c.JSON(http.StatusOK, mergedOpenAPISpec())
	})
	r.GET("/docs-all", docsPage("/openapi-all.json"))
}

func queueOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Station Queue Service API", "version": "1.0.0"},
		"paths": map[string]interface{}{
			"/api/v1/queue/{destinationId}": map[string]interface{}{
				"get":  map[string]interface{}{"summary": "List queue"},
				"post": map[string]interface{}{"summary": "Add queue entry"},
			},
			"/api/v1/day-passes": map[string]interface{}{
				"get": map[string]interface{}{"summary": "List day passes"},
			},
		},
	}
}

// mergedOpenAPISpec provides a single spec with core Queue and Booking endpoints
func mergedOpenAPISpec() map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info":    map[string]interface{}{"title": "Station Unified API", "version": "1.0.0"},
		"paths": map[string]interface{}{
			// Queue Service
			"/api/v1/queue/{destinationId}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List queue (ordered) for destination",
					"parameters": []map[string]interface{}{
						{"name": "destinationId", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
				},
				"post": map[string]interface{}{
					"summary": "Add queue entry",
					"parameters": []map[string]interface{}{
						{"name": "destinationId", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
				},
			},
			"/api/v1/day-passes": map[string]interface{}{
				"get": map[string]interface{}{"summary": "List day passes"},
			},
			"/api/v1/vehicles": map[string]interface{}{
				"get": map[string]interface{}{"summary": "List vehicles"},
			},
			"/api/v1/vehicles/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "Get vehicle details",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
				},
			},
			"/api/v1/vehicles/{id}/authorized-routes": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List vehicle authorized routes",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
				},
			},
			"/api/v1/routes": map[string]interface{}{
				"get": map[string]interface{}{
					"summary": "List routes/destinations",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "OK",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"data": map[string]interface{}{
												"type":  "array",
												"items": map[string]interface{}{"$ref": "#/components/schemas/Route"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			// Booking Service (minimal)
			"/api/v1/bookings": map[string]interface{}{
				"post": map[string]interface{}{"summary": "Create booking"},
			},
			"/api/v1/bookings/{id}/cancel": map[string]interface{}{
				"put": map[string]interface{}{
					"summary": "Cancel booking",
					"parameters": []map[string]interface{}{
						{"name": "id", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
				},
			},
			"/api/v1/trips": map[string]interface{}{
				"get": map[string]interface{}{"summary": "List trips"},
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type": "http", "scheme": "bearer", "bearerFormat": "JWT",
				},
			},
			"schemas": map[string]interface{}{
				"Route": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id":       map[string]interface{}{"type": "string"},
						"name":     map[string]interface{}{"type": "string"},
						"isActive": map[string]interface{}{"type": "boolean"},
					},
				},
			},
		},
		"security": []map[string]interface{}{{"bearerAuth": []interface{}{}}},
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

// docsPage serves a simple CIN form that fetches a token and loads Swagger UI with Authorization
func docsPage(specURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `<!doctype html><html><head><meta charset="utf-8"/><title>Docs Auth</title>
<style>body{font-family:sans-serif;margin:24px}#form{margin-bottom:16px}input{padding:8px;margin-right:8px}</style>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head><body>
<div id="form">
  <input id="cin" placeholder="Enter CIN" maxlength="20" />
  <button onclick="login()">Login</button>
  <span id="msg"></span>
  <div id="hint" style="margin-top:8px;color:#666">After login, the docs will load with your token.</div>
  <hr/>
</div>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
async function login(){
  const cin = document.getElementById('cin').value.trim();
  if(!cin){document.getElementById('msg').textContent='Enter CIN';return}
  try{
    const authUrl = window.location.protocol + '//' + window.location.hostname + ':8001/api/v1/auth/login';
    const res = await fetch(authUrl,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({cin})});
    const data = await res.json();
    if(!res.ok||!data?.data?.token){ document.getElementById('msg').textContent='Login failed'; return }
    const token = data.data.token;
    document.getElementById('msg').textContent='Logged in';
    window.ui = SwaggerUIBundle({ url: '`+specURL+`', dom_id:'#swagger-ui', requestInterceptor: (req)=>{ req.headers = req.headers||{}; req.headers['Authorization']='Bearer '+token; return req; } });
  }catch(e){ document.getElementById('msg').textContent='Error'; }
}
</script>
</body></html>`)
	}
}
