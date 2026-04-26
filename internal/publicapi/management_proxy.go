package publicapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ProxyStaffList(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/staff/"
	c.Request.URL.RawPath = "/api/v1/staff/"
	h.authProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStaffGet(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/staff/" + id
	c.Request.URL.RawPath = "/api/v1/staff/" + id
	h.authProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStaffCreate(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/staff/"
	c.Request.URL.RawPath = "/api/v1/staff/"
	h.authProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStaffUpdate(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/staff/" + id
	c.Request.URL.RawPath = "/api/v1/staff/" + id
	h.authProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStaffDelete(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/staff/" + id
	c.Request.URL.RawPath = "/api/v1/staff/" + id
	h.authProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehiclesList(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/vehicles"
	c.Request.URL.RawPath = "/api/v1/vehicles"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehiclesSearch(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/vehicles/search"
	c.Request.URL.RawPath = "/api/v1/vehicles/search"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehiclesCreate(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/vehicles"
	c.Request.URL.RawPath = "/api/v1/vehicles"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehiclesUpdate(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/vehicles/" + id
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyRoutesList(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/routes"
	c.Request.URL.RawPath = "/api/v1/routes"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehicleAuthorizedRoutesList(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/vehicles/" + id + "/authorized-routes"
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id + "/authorized-routes"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehicleAuthorizedRoutesAdd(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/vehicles/" + id + "/authorized-routes"
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id + "/authorized-routes"
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehicleAuthorizedRoutesUpdate(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	authID := c.Param("authId")
	c.Request.URL.Path = "/api/v1/vehicles/" + id + "/authorized-routes/" + authID
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id + "/authorized-routes/" + authID
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehicleAuthorizedRoutesDelete(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	authID := c.Param("authId")
	c.Request.URL.Path = "/api/v1/vehicles/" + id + "/authorized-routes/" + authID
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id + "/authorized-routes/" + authID
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyVehiclesDelete(c *gin.Context) {
	if h.queueProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "QUEUE_PROXY_DISABLED"})
		return
	}

	id := c.Param("id")
	c.Request.URL.Path = "/api/v1/vehicles/" + id
	c.Request.URL.RawPath = "/api/v1/vehicles/" + id
	h.queueProxy.ServeHTTP(c.Writer, c.Request)
}
