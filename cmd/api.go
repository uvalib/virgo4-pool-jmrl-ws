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
