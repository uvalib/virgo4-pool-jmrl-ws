package main

// SearchRequest contains all of the data necessary for a client seatch request
// Note that the JMRL pool does not support facets/filters
type SearchRequest struct {
	Query      string     `json:"query"`
	Pagination Pagination `json:"pagination"`
}

// Pagination cantains pagination info
type Pagination struct {
	Start int `json:"start"`
	Rows  int `json:"rows"`
	Total int `json:"total"`
}

// PoolResult is the response V4API response format
type PoolResult struct {
	ElapsedMS       int64                  `json:"elapsed_ms,omitempty"`
	Pagination      Pagination             `json:"pagination"`
	Groups          []Group                `json:"group_list,omitempty"`
	Confidence      string                 `json:"confidence,omitempty"`
	Debug           map[string]interface{} `json:"debug"`
	Warnings        []string               `json:"warnings"`
	StatusCode      int                    `json:"status_code"`
	StatusMessage   string                 `json:"status_msg,omitempty"`
	ContentLanguage string                 `json:"-"`
}

// Group contains the records for a single group in a search result set.
type Group struct {
	Value   string        `json:"value"`
	Count   int           `json:"count"`
	Fields  []RecordField `json:"fields,omitempty"`
	Records []Record      `json:"record_list,omitempty"`
}

// Record is a summary of one search hit
type Record struct {
	Fields []RecordField          `json:"fields"`
	Debug  map[string]interface{} `json:"debug"`
}

// RecordField contains metadata for a single field in a record.
type RecordField struct {
	Name       string `json:"name"`
	Type       string `json:"type,omitempty"` // empty implies "text"
	Label      string `json:"label"`
	Value      string `json:"value"`
	Visibility string `json:"visibility,omitempty"` // e.g. "basic" or "detailed".  empty implies "basic"
	Display    string `json:"display,omitempty"`    // e.g. "optional".  empty implies not optional
}

// JMRLResult contains the response data from a JMRL search
type JMRLResult struct {
	Count   int `json:"count"`
	Total   int `json:"total"`
	Start   int `json:"start"`
	Entries []struct {
		Relevance float32 `json:"relevance"`
		Bib       struct {
			ID          string          `json:"id"`
			Title       string          `json:"title"`
			Author      string          `json:"author"`
			PublishYear string          `json:"publish_year"`
			Language    JMRLCodeValue   `json:"lang"`
			Type        JMRLCodeValue   `json:"materialType"`
			VarFields   []JMRLVarFields `json:"varFields"`
		} `json:"bib"`
	} `json:"entries"`
}

// JMRLCodeValue is a pair of code / value or code/name data
type JMRLCodeValue struct {
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
	Name  string `json:"name,omitempty"`
}

// JMRLVarFields contains MARC data from the JRML fields=varFields request param
type JMRLVarFields struct {
	FieldTag  string `json:"fieldTag"`
	MarcTag   string `json:"marcTag"`
	Subfields []struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
	} `json:"subfields"`
}
