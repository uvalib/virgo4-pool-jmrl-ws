package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// ServiceContext contains common data used by all handlers
type ServiceContext struct {
	Version    string
	API        string
	AuthToken  string
	I18NBundle *i18n.Bundle
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

	// do something with token

	log.Printf("got bearer token: [%s]", token)
}
