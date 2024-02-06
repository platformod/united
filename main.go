// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"log"

	"github.com/sethvargo/go-envconfig"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := envconfig.Process(context.Background(), &cfg); err != nil {
		log.Fatal(err)
	}

	//gin.SetMode(gin.ReleaseMode)

	r := gin.Default()
	_ = r.SetTrustedProxies(nil)

	r.Use(UnitedSetup())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
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
