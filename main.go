// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log"
	"net/http"

	healthcheck "github.com/RaMin0/gin-health-check"
	"github.com/gin-contrib/logger"
	"github.com/gin-contrib/requestid"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sethvargo/go-envconfig"
)

func main() {
	if err := envconfig.Process(context.Background(), &cfg); err != nil {
		log.Fatal(err)
	}

	if cfg.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	_ = r.SetTrustedProxies(nil)

	r.Use(gin.Recovery())
	r.Use(healthcheck.Default())
	r.Use(requestid.New())
	r.Use(logger.SetLogger(
		logger.WithLogger(func(_ *gin.Context, l zerolog.Logger) zerolog.Logger {
			return l.Output(gin.DefaultWriter).With().Logger()
		}),
	))
	r.Use(UnitedSetup())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	state := r.Group("/state", UnitedBasicAuth())
	{
		state.GET("/:group/:name", getHandler)
		state.POST("/:group/:name", postHandler)

		// Tested in the http backend tf code, but I'm iffy if this is ever actually called
		state.DELETE("/:group/:name", deleteHandler)

		// LOCK and UNLOCK, don't have helper handlers in gin
		state.Handle("LOCK", "/:group/:name", lockHandler)
		state.Handle("UNLOCK", "/:group/:name", unlockHandler)
	}

	_ = r.Run()
}
