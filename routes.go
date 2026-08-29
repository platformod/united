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
		se.Router.DELETE("/state/{group}/{name}", requireGroupBasicAuth(deleteState))
		se.Router.Any("LOCK /state/{group}/{name}", requireGroupBasicAuth(lockState))
		se.Router.Any("UNLOCK /state/{group}/{name}", requireGroupBasicAuth(unlockState))

		return se.Next()
	})
}
