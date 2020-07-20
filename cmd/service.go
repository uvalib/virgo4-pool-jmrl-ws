package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvalib/virgo4-api/v4api"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/uvalib/virgo4-jwt/v4jwt"
	"golang.org/x/text/language"
)

// ServiceContext contains common data used by all handlers
type ServiceContext struct {
	Version         string
	API             string
	AuthToken       string
	AccessToken     string
	AccessExpiresAt time.Time
	JWTKey          string
	I18NBundle      *i18n.Bundle
	HTTPClient      *http.Client
}

// RequestError contains http status code and message for and API request
type RequestError struct {
	StatusCode int
	Message    string
}

// InitializeService will initialize the service context based on the config parameters.
// Any pools found in the DB will be added to the context and polled for status.
// Any errors are FATAL.
func InitializeService(version string, cfg *ServiceConfig) *ServiceContext {
	log.Printf("Initializing Service")
	svc := ServiceContext{Version: version, API: cfg.API, JWTKey: cfg.JWTKey}

	log.Printf("Create HTTP Client")
	defaultTransport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 600 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 2 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
	svc.HTTPClient = &http.Client{
		Transport: defaultTransport,
		Timeout:   5 * time.Second,
	}

	// Create the auth token from base64 encoding of key:secret. Per JRML docs
	// https://techdocs.iii.com/sierraapi/Content/zTutorials/tutAuthenticate.htm
	log.Printf("Create base64 encoded JRML auth token...")
	token := fmt.Sprintf("%s:%s", cfg.APIKey, cfg.APISecret)
	svc.AuthToken = base64.StdEncoding.EncodeToString([]byte(token))

	log.Printf("Authenticate with JMRL API")
	svc.getAccessToken()

	log.Printf("Init localization")
	svc.I18NBundle = i18n.NewBundle(language.English)
	svc.I18NBundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	svc.I18NBundle.MustLoadMessageFile("./i18n/active.en.toml")
	svc.I18NBundle.MustLoadMessageFile("./i18n/active.es.toml")

	return &svc
}

// IgnoreFavicon is a dummy to handle browser favicon requests without warnings
func (svc *ServiceContext) ignoreFavicon(c *gin.Context) {
	// no-op; just here to prevent errors when request made from browser
}

// GetVersion reports the version of the serivce
func (svc *ServiceContext) getVersion(c *gin.Context) {
	build := "unknown"
	// working directory is the bin directory, and build tag is in the root
	files, _ := filepath.Glob("../buildtag.*")
	if len(files) == 1 {
		build = strings.Replace(files[0], "../buildtag.", "", 1)
	}

	vMap := make(map[string]string)
	vMap["version"] = svc.Version
	vMap["build"] = build
	c.JSON(http.StatusOK, vMap)
}

// HealthCheck reports the health of the serivce
func (svc *ServiceContext) healthCheck(c *gin.Context) {
	type hcResp struct {
		Healthy bool   `json:"healthy"`
		Message string `json:"message,omitempty"`
	}
	hcMap := make(map[string]hcResp)

	idx := strings.LastIndex(svc.API, "/")
	baseURL := svc.API[0:idx]
	authURL := fmt.Sprintf("%s/about", baseURL)
	postReq, _ := http.NewRequest("GET", authURL, nil)
	postReq.Header.Set("Accept", "application/json")
	resp, postErr := svc.HTTPClient.Do(postReq)
	if resp != nil {
		defer resp.Body.Close()
	}
	if postErr != nil {
		hcMap["jmrl"] = hcResp{Healthy: false, Message: postErr.Error()}
	} else if resp.StatusCode != 200 {
		hcMap["jmrl"] = hcResp{Healthy: false, Message: resp.Status}
	} else {
		hcMap["jmrl"] = hcResp{Healthy: true}
	}

	c.JSON(http.StatusOK, hcMap)
}

// IdentifyHandler returns localized identity information for this pool
func (svc *ServiceContext) identifyHandler(c *gin.Context) {
	acceptLang := strings.Split(c.GetHeader("Accept-Language"), ",")[0]
	if acceptLang == "" {
		acceptLang = "en-US"
	}
	log.Printf("Identify request Accept-Language %s", acceptLang)
	localizer := i18n.NewLocalizer(svc.I18NBundle, acceptLang)

	resp := v4api.PoolIdentity{Attributes: make([]v4api.PoolAttribute, 0)}
	resp.Name = localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "PoolName"})
	resp.Description = localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "PoolDescription"})
	resp.Mode = "record"
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "logo_url", Supported: true, Value: "/assets/jmrl_logo.svg"})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "external_url", Supported: true, Value: "https://jmrl.org"})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "external_hold", Supported: true, Value: "https://jmrl.org"})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "uva_ils", Supported: false})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "facets", Supported: false})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "cover_images", Supported: false})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "course_reserves", Supported: false})
	resp.Attributes = append(resp.Attributes, v4api.PoolAttribute{Name: "sorting", Supported: false})

	c.JSON(http.StatusOK, resp)
}

// getBearerToken is a helper to extract the user auth token from the Auth header
func getBearerToken(authorization string) (string, error) {
	components := strings.Split(strings.Join(strings.Fields(authorization), " "), " ")

	// must have two components, the first of which is "Bearer", and the second a non-empty token
	if len(components) != 2 || components[0] != "Bearer" || components[1] == "" {
		return "", fmt.Errorf("Invalid Authorization header: [%s]", authorization)
	}

	return components[1], nil
}

// AuthMiddleware is a middleware handler that verifies presence of a
// user Bearer token in the Authorization header.
func (svc *ServiceContext) authMiddleware(c *gin.Context) {
	tokenStr, err := getBearerToken(c.Request.Header.Get("Authorization"))
	if err != nil {
		log.Printf("Authentication failed: [%s]", err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	if tokenStr == "undefined" {
		log.Printf("Authentication failed; bearer token is undefined")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	log.Printf("Validating JWT auth token...")
	v4Claims, jwtErr := v4jwt.Validate(tokenStr, svc.JWTKey)
	if jwtErr != nil {
		log.Printf("JWT signature for %s is invalid: %s", tokenStr, jwtErr.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// add the parsed claims and signed JWT string to the request context so other handlers can access it.
	c.Set("jwt", tokenStr)
	c.Set("claims", v4Claims)
	log.Printf("got bearer token: [%s]: %+v", tokenStr, v4Claims)
}

// GetAccess token will POST to the JMRL API /v5/token API to get an access token with an expiration time
// Results will be stored in the ServiceContext
func (svc *ServiceContext) getAccessToken() error {
	log.Printf("Get JMRL access token")
	startTime := time.Now()
	authURL := fmt.Sprintf("%s/token", svc.API)
	postReq, _ := http.NewRequest("POST", authURL, nil)
	postReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", svc.AuthToken))
	postResp, postErr := svc.HTTPClient.Do(postReq)
	respBytes, respErr := handleAPIResponse(authURL, postResp, postErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if respErr != nil {
		svc.AccessExpiresAt = time.Now()
		svc.AccessToken = ""
		log.Printf("ERROR: Failed response from POST %s %d. Elapsed Time: %d (ms). %s",
			authURL, respErr.StatusCode, elapsedMS, respErr.Message)
		return errors.New(respErr.Message)
	}
	log.Printf("Successful response from POST %s. Elapsed Time: %d (ms)", authURL, elapsedMS)

	var authResp struct {
		AccessToken   string `json:"access_token"`
		TokenType     string `json:"token_type"`
		ExpireSeconds int    `json:"expires_in"`
	}

	parseErr := json.Unmarshal(respBytes, &authResp)
	if parseErr != nil {
		log.Printf("ERROR: Unable to parse auth response: %v", parseErr)
		svc.AccessExpiresAt = time.Now()
		svc.AccessToken = ""
		return parseErr
	}

	log.Printf("Authentication successful, expires in %d seconds", authResp.ExpireSeconds)
	svc.AccessToken = authResp.AccessToken
	svc.AccessExpiresAt = time.Now().Add(time.Second * time.Duration(authResp.ExpireSeconds))
	return nil
}

// APIGet sends a GET to the JMRL API and returns results a byte array
func (svc *ServiceContext) apiGet(tgtURL string) ([]byte, *RequestError) {
	log.Printf("JMRL API GET request: %s", tgtURL)
	startTime := time.Now()
	if startTime.After(svc.AccessExpiresAt) {
		log.Printf("Access token has expired; requesting a new one")
		authErr := svc.getAccessToken()
		if authErr != nil {
			return nil, &RequestError{StatusCode: 401, Message: authErr.Error()}
		}
	}

	getReq, _ := http.NewRequest("GET", tgtURL, nil)
	getReq.Header.Set("deleted", "false")
	getReq.Header.Set("suppressed", "false")
	getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", svc.AccessToken))
	rawResp, rawErr := svc.HTTPClient.Do(getReq)
	resp, err := handleAPIResponse(tgtURL, rawResp, rawErr)
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)

	if err != nil {
		log.Printf("ERROR: Failed response from GET %s %d. Elapsed Time: %d (ms). %s",
			tgtURL, err.StatusCode, elapsedMS, err.Message)
	} else {
		log.Printf("Successful response from GET %s. Elapsed Time: %d (ms)", tgtURL, elapsedMS)
	}
	return resp, err
}

func handleAPIResponse(URL string, resp *http.Response, err error) ([]byte, *RequestError) {
	if err != nil {
		status := http.StatusBadRequest
		errMsg := err.Error()
		if strings.Contains(err.Error(), "Timeout") {
			status = http.StatusRequestTimeout
			errMsg = fmt.Sprintf("%s timed out", URL)
		} else if strings.Contains(err.Error(), "connection refused") {
			status = http.StatusServiceUnavailable
			errMsg = fmt.Sprintf("%s refused connection", URL)
		}
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	} else if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		status := resp.StatusCode
		errMsg := string(bodyBytes)
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	return bodyBytes, nil
}
