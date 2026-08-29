// SPDX-License-Identifier: MPL-2.0

package main

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func registerRoutes(app core.App, cfg Config) {
	app.Store().Set(stateMasterKeyStoreKey, cfg.StateMasterKey)

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/ping", func(e *core.RequestEvent) error {
			return e.JSON(http.StatusOK, map[string]string{"message": "pong"})
		})

		se.Router.GET("/state/{group}/{name}", requireGroupBasicAuth(getState))
		se.Router.POST("/state/{group}/{name}", requireGroupBasicAuth(func(e *core.RequestEvent, group *core.Record) error {
			return postState(e, group, cfg.StateMasterKey)
		}))
		se.Router.DELETE("/state/{group}/{name}", requireGroupBasicAuth(deleteStateRouteNotFound))
		stateRoute := requireGroupBasicAuth(stateRouteNotFound)
		se.Router.Any("LOCK /state/{group}/{name}", stateRoute)
		se.Router.Any("UNLOCK /state/{group}/{name}", stateRoute)

		return se.Next()
	})

	_ = cfg
}

func stateRouteNotFound(e *core.RequestEvent, _ *core.Record) error {
	return e.NotFoundError("State not found.", nil)
}

func postStateRouteNotFound(e *core.RequestEvent, group *core.Record) error {
	return stateRouteNotFoundWithLockID(e, group, e.Request.URL.Query().Get("ID"))
}

func deleteStateRouteNotFound(e *core.RequestEvent, group *core.Record) error {
	return stateRouteNotFoundWithLockID(e, group, e.Request.URL.Query().Get("ID"))
}

func stateRouteNotFoundWithLockID(e *core.RequestEvent, _ *core.Record, _ string) error {
	return e.NotFoundError("State not found.", nil)
}
