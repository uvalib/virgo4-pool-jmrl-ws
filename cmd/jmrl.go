package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// Search accepts a search POST, transforms the query into JMRL format and perfoms the search
func (svc *ServiceContext) search(c *gin.Context) {
	log.Printf("JMRL search requested")
	var req SearchRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("ERROR: unable to parse search request: %s", err.Error())
		c.String(http.StatusBadRequest, "invalid request")
		return
	}

	// dates are not suported and will cause no results to be returned
	// Fail this query with a bad request and info about the reason
	log.Printf("Raw query: %s", req.Query)
	if strings.Contains(req.Query, "date:") {
		log.Printf("ERROR: date queries are not supported")
		c.String(http.StatusBadRequest, "Date queries are not supported by JMRL")
		return
	}
	// EX: keyword: {(calico OR "tortoise shell") AND cats}
	// Approach, replace all {} with (),
	// Remove keyword:, replace subject, author and title with JMRL codes
	// Identifier is special; it maps to two query terms: barcode and callnumber
	// replace it with: (b:(val) or c:(val))
	parsedQ := req.Query
	for strings.Contains(parsedQ, "identifier:") {
		iIdx := strings.Index(parsedQ, "identifier:")
		idx0 := indexAt(parsedQ, "{", iIdx)
		idx1 := indexAt(parsedQ, "}", idx0)
		idStr := parsedQ[idx0+1 : idx1]
		idQ := fmt.Sprintf("(b:(%s) OR c:(%s))", idStr, idStr)
		parsedQ = fmt.Sprintf("%s%s%s", parsedQ[0:iIdx], idQ, parsedQ[idx1+1:])
	}
	parsedQ = strings.ReplaceAll(parsedQ, "{", "(")
	parsedQ = strings.ReplaceAll(parsedQ, "}", ")")
	parsedQ = strings.ReplaceAll(parsedQ, "keyword: ", "")
	parsedQ = strings.ReplaceAll(parsedQ, "title: ", "t:")
	parsedQ = strings.ReplaceAll(parsedQ, "author: ", "a:")
	parsedQ = strings.ReplaceAll(parsedQ, "subject: ", "d:")

	parsedQ = strings.TrimSpace(parsedQ)
	log.Printf("Parsed query: [%s]", parsedQ)
	parsedQ = url.QueryEscape(parsedQ)
	tgtURL := fmt.Sprintf("%s/bibs/search?text=%s", svc.API, parsedQ)
	resp, err := svc.apiGet(tgtURL)
	if err != nil {
		c.String(err.StatusCode, err.Message)
		return
	}

	log.Printf("RESPONSE: %s", resp)

	c.String(http.StatusNotImplemented, "Not yet implemented")
}

// helper to find index of a substring starting at a specific offset
func indexAt(s string, tgt string, startIdx int) int {
	idx := strings.Index(s[startIdx:], tgt)
	if idx > -1 {
		idx += startIdx
	}
	return idx
}

// Facets placeholder implementaion for a V4 facet POST.
func (svc *ServiceContext) facets(c *gin.Context) {
	log.Printf("JMRL facets requested, but JMRL does not support this")
	c.JSON(http.StatusOK, "")
}

// GetResource will get a JMRL resource by ID
func (svc *ServiceContext) getResource(c *gin.Context) {
	id := c.Param("id")
	log.Printf("Resource %s details requested", id)
	c.String(http.StatusNotImplemented, "Not yet implemented")
}
