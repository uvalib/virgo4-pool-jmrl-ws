package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/uvalib/virgo4-api/v4api"
	"github.com/uvalib/virgo4-parser/v4parser"
)

type providerDetails struct {
	Provider    string `json:"provider"`
	Label       string `json:"label,omitempty"`
	HomepageURL string `json:"homepage_url,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
}

type poolProviders struct {
	Providers []providerDetails `json:"providers"`
}

// ProvidersHandler returns a list of access_url providers for JMRL
func (svc *ServiceContext) providersHandler(c *gin.Context) {
	p := poolProviders{Providers: make([]providerDetails, 0)}
	p.Providers = append(p.Providers, providerDetails{
		Provider:    "freading",
		Label:       "Freading",
		LogoURL:     "/assets/freading.png",
		HomepageURL: "https://freading.com/index",
	})
	p.Providers = append(p.Providers, providerDetails{
		Provider:    "overdrive",
		Label:       "Overdrive",
		LogoURL:     "/assets/overdrive.png",
		HomepageURL: "https://www.overdrive.com",
	})
	c.JSON(http.StatusOK, p)
}

// Search accepts a search POST, transforms the query into JMRL format and perfoms the search
func (svc *ServiceContext) search(c *gin.Context) {
	log.Printf("JMRL search requested")
	var req v4api.SearchRequest
	if err := c.BindJSON(&req); err != nil {
		log.Printf("ERROR: unable to parse search request: %s", err.Error())
		c.String(http.StatusBadRequest, "invalid request")
		return
	}

	// make sure the query is well formed
	log.Printf("Raw query: %s, %+v", req.Query, req.Pagination)
	valid, errors := v4parser.Validate(req.Query)
	if valid == false {
		log.Printf("INFO: Query [%s] is not valid: %s", req.Query, errors)
		c.String(http.StatusBadRequest, "Malformed search")
		return
	}

	// JMRL does not support filtering. If a filter is specified in the search, return 0 hits
	// Note: when doing a next page request, the request contains:
	//       Filters:[{PoolID:worldcat Facets:[]}]
	//       accept this configuration
	filtersSpecified := false
	if len(req.Filters) > 1 {
		filtersSpecified = true
	} else if len(req.Filters) == 1 {
		filtersSpecified = len(req.Filters[0].Facets) > 0
	}
	if filtersSpecified || strings.Contains(req.Query, "filter:") {
		log.Printf("Filters specified in search, return no matches")
		v4Resp := &v4api.PoolResult{ElapsedMS: 0, Confidence: "low"}
		v4Resp.Groups = make([]v4api.Group, 0)
		v4Resp.Pagination = v4api.Pagination{Start: 0, Total: 0, Rows: 0}
		v4Resp.StatusCode = http.StatusOK
		c.JSON(http.StatusOK, v4Resp)
		return
	}

	// date, identifier, journal_title, fulltext, series, and filter are not supported.
	// Fail these with a bad request and info about the reason
	// We mark these messages as WARNING's because they are expected
	if strings.Contains(req.Query, "date:") {
		log.Printf("WARNING: date queries are not supported")
		c.String(http.StatusNotImplemented, "Date queries are not supported")
		return
	}
	if strings.Contains(req.Query, "identifier:") {
		log.Printf("WARNING: identifier queries are not supported")
		c.String(http.StatusNotImplemented, "Identifier queries are not supported")
		return
	}
	if strings.Contains(req.Query, "journal_title:") {
		log.Printf("WARNING: journal title queries are not supported")
		c.String(http.StatusNotImplemented, "Journal Title queries are not supported")
		return
	}
	if strings.Contains(req.Query, "fulltext:") {
		log.Printf("WARNING: full text queries are not supported")
		c.String(http.StatusNotImplemented, "Full Text queries are not supported")
		return
	}
	if strings.Contains(req.Query, "series:") {
		log.Printf("WARNING: series queries are not supported")
		c.String(http.StatusNotImplemented, "Series queries are not supported")
		return
	}

	// EX: keyword: {(calico OR "tortoise shell") AND cats}
	// Approach, replace all {} with (),
	// Remove keyword:, replace subject, author and title with JMRL codes
	parsedQ := req.Query
	parsedQ = strings.ReplaceAll(parsedQ, "{", "(")
	parsedQ = strings.ReplaceAll(parsedQ, "}", ")")
	parsedQ = strings.ReplaceAll(parsedQ, "keyword:", "")
	parsedQ = strings.ReplaceAll(parsedQ, "title:", "t:")
	parsedQ = strings.ReplaceAll(parsedQ, "author:", "a:")
	parsedQ = strings.ReplaceAll(parsedQ, "subject:", "d:")

	// map unsupported fields to fine inventory number, which they won't match
	// this preserves the AND/OR/NOT behavior
	parsedQ = strings.ReplaceAll(parsedQ, "published:", "v:")

	parsedQ = strings.TrimSpace(parsedQ)
	log.Printf("Parsed query: %s", parsedQ)
	if parsedQ == "()" {
		parsedQ = "(*)"
	}

	parsedQ = url.QueryEscape(parsedQ)
	fields := "fields=default,varFields,locations,available"
	paging := fmt.Sprintf("offset=%d&limit=%d", req.Pagination.Start, 20)
	tgtURL := fmt.Sprintf("%s/bibs/search?text=%s&%s&%s", svc.API, parsedQ, paging, fields)

	startTime := time.Now()
	resp, err := svc.apiGet(tgtURL)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)
	v4Resp := &v4api.PoolResult{ElapsedMS: elapsedMS, Confidence: "low"}
	v4Resp.Groups = make([]v4api.Group, 0)

	if err != nil {
		v4Resp.StatusCode = err.StatusCode
		v4Resp.StatusMessage = err.Message
		c.JSON(err.StatusCode, v4Resp)
		return
	}

	jmrlResp := &JMRLResult{}
	respErr := json.Unmarshal(resp, jmrlResp)
	if respErr != nil {
		log.Printf("ERROR: Invalid response from JMRL API: %s", respErr.Error())
		v4Resp.StatusCode = http.StatusInternalServerError
		v4Resp.StatusMessage = respErr.Error()
		c.JSON(http.StatusInternalServerError, v4Resp)
		return
	}

	v4Resp.Pagination = v4api.Pagination{Start: jmrlResp.Start, Total: jmrlResp.Total,
		Rows: jmrlResp.Count}
	for _, entry := range jmrlResp.Entries {
		bib := entry.Bib
		groupRec := v4api.Group{Value: bib.ID, Count: 1}
		groupRec.Records = make([]v4api.Record, 0)
		record := v4api.Record{}
		record.Fields = getResultFields(&bib)
		groupRec.Records = append(groupRec.Records, record)
		v4Resp.Groups = append(v4Resp.Groups, groupRec)
	}

	if jmrlResp.Total > 0 {
		v4Resp.Confidence = "medium"
	}

	v4Resp.StatusCode = http.StatusOK
	c.JSON(http.StatusOK, v4Resp)
}

// TODO localization of labels
func getResultFields(bib *JMRLBib) []v4api.RecordField {
	fields := make([]v4api.RecordField, 0)
	f := v4api.RecordField{Name: "id", Type: "identifier", Label: "Identifier",
		Value: bib.ID, Display: "optional", CitationPart: "id"}
	fields = append(fields, f)

	for _, loc := range bib.Locations {
		val := fmt.Sprintf("Jefferson-Madison Regional Library - %s", loc.Name)
		if loc.Name == "none" {
			val = "Jefferson-Madison Regional Library"
		}
		f = v4api.RecordField{Name: "location", Type: "location", Label: "Location",
			Value: val}
		fields = append(fields, f)
	}

	f = v4api.RecordField{Name: "published_date", Type: "published_date", Label: "Publication Date",
		Value: fmt.Sprintf("%d", bib.PublishYear), CitationPart: "published_date"}
	fields = append(fields, f)

	f = v4api.RecordField{Name: "format", Type: "format", Label: "Format",
		Value: bib.Type.Value, CitationPart: "format"}
	fields = append(fields, f)

	f = v4api.RecordField{Name: "language", Type: "language", Label: "Language",
		Value: bib.Language.Value, Visibility: "detailed", CitationPart: "language"}
	fields = append(fields, f)

	vals := getVarField(&bib.VarFields, "245", "a")
	f = v4api.RecordField{Name: "title", Type: "title", Label: "Title", Value: html.UnescapeString(vals[0]), CitationPart: "title"}
	fields = append(fields, f)

	vals = getVarField(&bib.VarFields, "245", "b")
	if len(vals) > 0 {
		f = v4api.RecordField{Name: "subtitle", Type: "subtitle", Label: "Subtitle", Value: html.UnescapeString(vals[0]), CitationPart: "subtitle"}
		fields = append(fields, f)
	}

	vals = getVarField(&bib.VarFields, "020", "a")
	for _, val := range vals {
		f = v4api.RecordField{Name: "isbn", Type: "isbn", Label: "ISBN", Value: val, Visibility: "detailed", CitationPart: "serial_number"}
		fields = append(fields, f)
	}

	vals = getVarField(&bib.VarFields, "092", "")
	for _, val := range vals {
		f = v4api.RecordField{Name: "call_number", Type: "call_number", Label: "Call Number",
			Value: val, Visibility: "detailed", CitationPart: "call_number"}
		fields = append(fields, f)
	}

	vals = getVarField(&bib.VarFields, "100", "a")
	for _, val := range vals {
		f = v4api.RecordField{Name: "author", Type: "author", Label: "Author", Value: html.UnescapeString(val), CitationPart: "author"}
		fields = append(fields, f)
	}

	// Get subjects....
	marcIDs := []string{"600", "650", "651", "647"}
	for _, id := range marcIDs {
		vals = getVarField(&bib.VarFields, id, "a")
		for _, val := range vals {
			f = v4api.RecordField{Name: "subject", Type: "subject", Label: "Subject", Value: val, Visibility: "detailed", CitationPart: "subject"}
			fields = append(fields, f)
		}
	}

	vals = getVarField(&bib.VarFields, "505", "a")
	if len(vals) > 0 {
		f = v4api.RecordField{Name: "contents", Type: "contents", Label: "Contents",
			Value: html.UnescapeString(vals[0]), Visibility: "detailed"}
		fields = append(fields, f)
	}

	vals = getVarField(&bib.VarFields, "520", "a")
	if len(vals) > 0 {
		f = v4api.RecordField{Name: "summary", Type: "summary", Label: "Summary",
			Value: html.UnescapeString(vals[0]), CitationPart: "abstract"}
		fields = append(fields, f)
	}

	vals = getVarField(&bib.VarFields, "776", "d")
	if len(vals) > 0 {
		f = v4api.RecordField{Name: "published", Type: "published", Label: "Published", Value: vals[0],
			Visibility: "detailed", CitationPart: "publisher"}
		fields = append(fields, f)
	}

	/*
		availF := v4api.RecordField{Name: "availability", Type: "availability", Label: "Availability", Value: "Checked Out"}
		vals = getVarField(&bib.VarFields, "856", "u")
		if len(vals) > 0 {
			provider := "freading"
			if strings.Contains(vals[0], "overdrive") {
				provider = "overdrive"
			}
			f = v4api.RecordField{Name: "access_url", Type: "url", Label: "Online Access", Value: vals[0], Provider: provider}
			fields = append(fields, f)
			if bib.Available {
				availF.Value = "Online"
			}
		} else {
			if bib.Available {
				availF.Value = "On Shelf Now"
			}
		}
		fields = append(fields, availF)
	*/
	return fields
}

func stripTrailingData(value string) string {
	if value == "" {
		return ""
	}
	lastChar := string(value[len(value)-1])
	if lastChar == ":" || lastChar == "/" || lastChar == "." {
		value = value[0 : len(value)-1]
		value = strings.TrimSpace(value)
	}
	return value
}

// helper to get an array of MARC values for the target element
func getVarField(varFields *[]JMRLVarFields, marc string, subfield string) []string {
	out := make([]string, 0)
	for _, field := range *varFields {
		val := ""
		if field.MarcTag == marc {
			for _, sub := range field.Subfields {
				if subfield == "" {
					if val != "" {
						val += " "
					}
					val += stripTrailingData(sub.Content)
				} else if sub.Tag == subfield {
					val = stripTrailingData(sub.Content)
				}
			}
		}
		if val != "" {
			out = append(out, val)
		}
	}
	return out
}

// Facets placeholder implementaion for a V4 facet POST.
func (svc *ServiceContext) facets(c *gin.Context) {
	log.Printf("JMRL facets requested, but JMRL does not support this. Returning empty list")
	empty := make(map[string]any)
	empty["facets"] = make([]v4api.Facet, 0)
	c.JSON(http.StatusOK, empty)
}

// GetResource will get a JMRL resource by ID
func (svc *ServiceContext) getResource(c *gin.Context) {
	id := c.Param("id")
	log.Printf("Resource %s details requested", id)
	tgtURL := fmt.Sprintf("%s/bibs/%s?fields=default,varFields,locations,available", svc.API, id)
	resp, err := svc.apiGet(tgtURL)
	if err != nil {
		c.JSON(err.StatusCode, err.Message)
		return
	}

	jmrlBib := &JMRLBib{}
	respErr := json.Unmarshal(resp, jmrlBib)
	if respErr != nil {
		log.Printf("ERROR: Invalid response from JMRL API: %s", respErr.Error())
		c.JSON(http.StatusInternalServerError, respErr.Error())
		return
	}

	var jsonResp struct {
		Fields []v4api.RecordField `json:"fields"`
	}
	jsonResp.Fields = getResultFields(jmrlBib)
	c.JSON(http.StatusOK, jsonResp)
}
