package main

// JMRLResult contains the response data from a JMRL search
type JMRLResult struct {
	Count   int `json:"count"`
	Total   int `json:"total"`
	Start   int `json:"start"`
	Entries []struct {
		Relevance float32 `json:"relevance"`
		Bib       JMRLBib `json:"bib"`
	} `json:"entries"`
}

// JMRLBib contans the MARC and JRML data for a single query hit
type JMRLBib struct {
	ID          string          `json:"id"`
	PublishYear int             `json:"publishYear"`
	Language    JMRLCodeValue   `json:"lang"`
	Type        JMRLCodeValue   `json:"materialType"`
	Locations   []JMRLCodeValue `json:"locations"`
	Available   bool            `json:"available"`
	VarFields   []JMRLVarFields `json:"varFields"`
}

// JMRLCodeValue is a pair of code / value or code/name data
type JMRLCodeValue struct {
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
	Name  string `json:"name,omitempty"`
}

// JMRLVarFields contains MARC data from the JRML fields=varFields request param
type JMRLVarFields struct {
	MarcTag   string `json:"marcTag"`
	Subfields []struct {
		Tag     string `json:"tag"`
		Content string `json:"content"`
	} `json:"subfields"`
}
