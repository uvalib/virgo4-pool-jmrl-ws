package main

import (
	"fmt"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
)

// Version of the service
const version = "1.1.1"

/**
 * MAIN
 */
func main() {
	log.Printf("===> V4 JMRL pool starting up <===")

	// Get config params and use them to init service context. Any issues are fatal
	cfg := LoadConfiguration()
	svc := InitializeService(version, cfg)

	log.Printf("Setup routes...")
	gin.SetMode(gin.ReleaseMode)
	gin.DisableConsoleColor()
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression))
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	corsCfg.AllowCredentials = true
	corsCfg.AddAllowHeaders("Authorization")
	router.Use(cors.New(corsCfg))

	//
	// we are removing Prometheus support for now
	//
	//p := ginprometheus.NewPrometheus("gin")

	// roundabout setup of /metrics endpoint to avoid double-gzip of response
	//router.Use(p.HandlerFunc())
	//h := promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{DisableCompression: true}))

	//router.GET(p.MetricsPath, func(c *gin.Context) {
	//	h.ServeHTTP(c.Writer, c.Request)
	//})

	router.GET("/", svc.getVersion)
	router.GET("/favicon.ico", svc.ignoreFavicon)
	router.GET("/version", svc.getVersion)
	router.GET("/healthcheck", svc.healthCheck)
	router.GET("/identify", svc.identifyHandler)
	api := router.Group("/api")
	{
		api.GET("/providers", svc.providersHandler)
		api.POST("/search", svc.authMiddleware, svc.search)
		api.POST("/search/facets", svc.authMiddleware, svc.facets)
		api.GET("/resource/:id", svc.authMiddleware, svc.getResource)
	}

	router.Use(static.Serve("/assets", static.LocalFile("./assets", true)))

	portStr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Start service v%s on port %s", version, portStr)
	log.Fatal(router.Run(portStr))
}
