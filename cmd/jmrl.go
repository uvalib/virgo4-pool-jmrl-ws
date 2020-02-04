package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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

	acceptLang := strings.Split(c.GetHeader("Accept-Language"), ",")[0]
	if acceptLang == "" {
		acceptLang = "en-US"
	}

	// dates are not suported and will cause no results to be returned
	// Fail this query with a bad request and info about the reason
	log.Printf("Raw query: %s, %+v", req.Query, req.Pagination)
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
	fields := "fields=default,varFields"
	paging := fmt.Sprintf("offset=%d&limit=%d", req.Pagination.Start, 20)
	tgtURL := fmt.Sprintf("%s/bibs/search?text=%s&%s&%s", svc.API, parsedQ, paging, fields)

	startTime := time.Now()
	resp, err := svc.apiGet(tgtURL)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)
	v4Resp := &PoolResult{ElapsedMS: elapsedMS, ContentLanguage: "medium"}
	v4Resp.Groups = make([]Group, 0)

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
		v4Resp.StatusCode = http.StatusInsufficientStorage
		v4Resp.StatusMessage = respErr.Error()
		c.JSON(err.StatusCode, v4Resp)
		return
	}

	v4Resp.Pagination = Pagination{Start: jmrlResp.Start, Total: jmrlResp.Total,
		Rows: jmrlResp.Count}
	for _, entry := range jmrlResp.Entries {
		bib := entry.Bib
		groupRec := Group{Value: bib.ID, Count: 1}
		groupRec.Records = make([]Record, 0)
		record := Record{}
		record.Fields = make([]RecordField, 0)
		addResultFields(&record, &bib)
		groupRec.Records = append(groupRec.Records, record)
		v4Resp.Groups = append(v4Resp.Groups, groupRec)
	}

	v4Resp.StatusCode = http.StatusOK
	v4Resp.StatusMessage = "OK"
	v4Resp.ContentLanguage = acceptLang
	c.JSON(http.StatusOK, v4Resp)
}

// TODO localization of labels
func addResultFields(record *Record, bib *JMRLBib) {
	f := RecordField{Name: "id", Type: "identifier", Label: "Identifier",
		Value: bib.ID, Display: "optional"}
	record.Fields = append(record.Fields, f)

	f = RecordField{Name: "publication_date", Type: "publication_date", Label: "Publication Date",
		Value: fmt.Sprintf("%d", bib.PublishYear)}
	record.Fields = append(record.Fields, f)

	f = RecordField{Name: "format", Type: "format", Label: "Format",
		Value: bib.Type.Value}
	record.Fields = append(record.Fields, f)

	f = RecordField{Name: "language", Type: "language", Label: "Language",
		Value: bib.Language.Value, Visibility: "detailed"}
	record.Fields = append(record.Fields, f)

	vals := getVarField(record, &bib.VarFields, "245", "a")
	f = RecordField{Name: "title", Type: "title", Label: "Title", Value: vals[0]}
	record.Fields = append(record.Fields, f)

	vals = getVarField(record, &bib.VarFields, "245", "b")
	if len(vals) > 0 {
		f = RecordField{Name: "subtitle", Type: "subtitle", Label: "Subtitle", Value: vals[0]}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "020", "a")
	for _, val := range vals {
		f = RecordField{Name: "isbn", Type: "isbn", Label: "ISBN", Value: val}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "100", "a")
	for _, val := range vals {
		f = RecordField{Name: "author", Type: "author", Label: "Author", Value: val}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "650", "a")
	for _, val := range vals {
		f = RecordField{Name: "subject", Type: "subject", Label: "Subject", Value: val, Visibility: "detailed"}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "520", "a")
	if len(vals) > 0 {
		f = RecordField{Name: "summary", Type: "summary", Label: "Summary", Value: vals[0]}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "776", "d")
	if len(vals) > 0 {
		f = RecordField{Name: "published", Type: "published", Label: "Published", Value: vals[0], Visibility: "detailed"}
		record.Fields = append(record.Fields, f)
	}

	vals = getVarField(record, &bib.VarFields, "856", "u")
	if len(vals) > 0 {
		f = RecordField{Name: "freading_url", Type: "url", Label: "Access Online", Value: vals[0]}
		record.Fields = append(record.Fields, f)
	}
}

func stripTrailingData(value string) string {
	lastChar := string(value[len(value)-1])
	if lastChar == ":" || lastChar == "/" {
		value = value[0 : len(value)-1]
		value = strings.TrimSpace(value)
	}
	return value
}

/// helper to get an array of MARC values for the target element
func getVarField(record *Record, varFields *[]JMRLVarFields, marc string, subfield string) []string {
	out := make([]string, 0)
	for _, field := range *varFields {
		if field.MarcTag == marc {
			for _, sub := range field.Subfields {
				if sub.Tag == subfield {
					out = append(out, stripTrailingData(sub.Content))
				}
			}
		}
	}
	return out
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
