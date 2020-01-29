package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Search accepts a search POST, transforms the query into JMRL format and perfoms the search
func (svc *ServiceContext) search(c *gin.Context) {
	log.Printf("JMRL search requested")
	c.String(http.StatusNotImplemented, "Not yet implemented")
}

// Facets placeholder implementaion for a V4 facet POST.
func (svc *ServiceContext) facets(c *gin.Context) {
	log.Printf("JMRL facets requested")
	c.String(http.StatusNotImplemented, "Not yet implemented")
}

// GetResource will get a JMRL resource by ID
func (svc *ServiceContext) getResource(c *gin.Context) {
	id := c.Param("id")
	log.Printf("Resource %s details requested", id)
	c.String(http.StatusNotImplemented, "Not yet implemented")
}
