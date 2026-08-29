// SPDX-License-Identifier: MPL-2.0

package main

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func registerRoutes(app core.App, cfg Config) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/ping", func(e *core.RequestEvent) error {
			return e.JSON(http.StatusOK, map[string]string{"message": "pong"})
		})

		stateRoute := requireGroupBasicAuth(stateRouteNotFound)
		se.Router.GET("/state/{group}/{name}", stateRoute)
		se.Router.POST("/state/{group}/{name}", stateRoute)
		se.Router.DELETE("/state/{group}/{name}", stateRoute)
		se.Router.Any("LOCK /state/{group}/{name}", stateRoute)
		se.Router.Any("UNLOCK /state/{group}/{name}", stateRoute)

		return se.Next()
	})

	_ = cfg
}

func stateRouteNotFound(e *core.RequestEvent, _ *core.Record) error {
	return e.NotFoundError("State not found.", nil)
}
