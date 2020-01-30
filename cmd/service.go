package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// ServiceContext contains common data used by all handlers
type ServiceContext struct {
	Version         string
	API             string
	AuthToken       string
	AccessToken     string
	AccessExpiresAt time.Time
	I18NBundle      *i18n.Bundle
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
	svc := ServiceContext{Version: version, API: cfg.API}

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

	// timeout := time.Duration(5 * time.Second)
	// client := http.Client{
	// 	Timeout: timeout,
	// }
	// authURL := fmt.Sprintf("%s/token", svc.API)
	// postReq, _ := http.NewRequest("POST", authURL, nil)
	// postReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", svc.AuthToken))
	// resp, postErr := client.Do(postReq)
	// log.Printf("Response %+v", resp)
	// if postErr != nil {
	// 	hcMap["jmrl"] = hcResp{Healthy: false, Message: postErr.Error()}
	// } else if resp.StatusCode != 200 {
	// 	hcMap["jmrl"] = hcResp{Healthy: false, Message: resp.Status}
	// } else {
	// 	hcMap["jmrl"] = hcResp{Healthy: true}
	// }

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

	var resp struct {
		Name         string `json:"name"`
		Descrription string `json:"description"`
	}

	resp.Name = localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "PoolName"})
	resp.Descrription = localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "PoolDescription"})

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
	token, err := getBearerToken(c.Request.Header.Get("Authorization"))

	if err != nil {
		log.Printf("Authentication failed: [%s]", err.Error())
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	log.Printf("got bearer token: [%s]", token)
}

// GetAccess token will POST to the JMRL API /v5/token API to get an access token with an expiration time
// Results will be stored in the ServiceContext
func (svc *ServiceContext) getAccessToken() error {
	log.Printf("Get JMRL access token")
	startTime := time.Now()
	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	authURL := fmt.Sprintf("%s/token", svc.API)
	postReq, _ := http.NewRequest("POST", authURL, nil)
	postReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", svc.AuthToken))
	postResp, postErr := client.Do(postReq)

	status := "Successful"
	if postErr != nil {
		status = "Failed"
	}
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)
	log.Printf("%s response from POST %s. Elapsed Time: %d (ms)", status, authURL, elapsedMS)

	respBytes, respErr := handleAPIResponse(authURL, postResp, postErr)
	if respErr != nil {
		svc.AccessExpiresAt = time.Now()
		svc.AccessToken = ""
		return errors.New(respErr.Message)
	}

	log.Printf("Authentication successful; parsing response")
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

// APIPost sends a POST to the JMRL API and returns results a byte array
func (svc *ServiceContext) apiPost(tgtURL string, values url.Values) ([]byte, *RequestError) {
	log.Printf("JMRL API POST request: %s", tgtURL)
	startTime := time.Now()
	if startTime.After(svc.AccessExpiresAt) {
		log.Printf("Access token has expired; requesting a new one")
		authErr := svc.getAccessToken()
		if authErr != nil {
			return nil, &RequestError{StatusCode: 401, Message: authErr.Error()}
		}
	}

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	postReq, _ := http.NewRequest("POST", tgtURL, nil)
	postReq.Header.Set("deleted", "false")
	postReq.Header.Set("suppressed", "false")
	postReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", svc.AccessToken))
	resp, err := client.Do(postReq)
	status := "Successful"
	if err != nil {
		status = "Failed"
	}
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)
	log.Printf("%s response from POST %s. Elapsed Time: %d (ms)", status, tgtURL, elapsedMS)
	return handleAPIResponse(tgtURL, resp, err)
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

	timeout := time.Duration(5 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	getReq, _ := http.NewRequest("GET", tgtURL, nil)
	getReq.Header.Set("deleted", "false")
	getReq.Header.Set("suppressed", "false")
	getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", svc.AccessToken))
	resp, err := client.Do(getReq)
	status := "Successful"
	if err != nil {
		status = "Failed"
	}
	elapsedNanoSec := time.Since(startTime)
	elapsedMS := int64(elapsedNanoSec / time.Millisecond)
	log.Printf("%s response from GET %s. Elapsed Time: %d (ms)", status, tgtURL, elapsedMS)
	return handleAPIResponse(tgtURL, resp, err)
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
		log.Printf("ERROR: %s request failed: %s", URL, errMsg)
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	} else if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		status := resp.StatusCode
		errMsg := string(bodyBytes)
		log.Printf("ERROR: %s request failed: %s", URL, errMsg)
		return nil, &RequestError{StatusCode: status, Message: errMsg}
	}

	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	return bodyBytes, nil
}
