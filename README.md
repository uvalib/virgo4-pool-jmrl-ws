# Virgo4 Jefferson Madision Regional Libray search pool

This is the Virgo4 pool for the Jefferson Madison Regional Library (JRML).
It implements the API detailed here: https://github.com/uvalib/v4-api

### System Requirements
* GO version 1.13 or greater (mod required)

### Current API

* GET /version : returns build version
* GET /identify : returns pool information
* GET /healthcheck : returns health check information
* GET /metrics : returns Prometheus metrics
* POST /api/search : returns search results for a Solr pool
* GET /api/resource/{id} : returns detailed information for a single Solr record
