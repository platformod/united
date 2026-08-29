// SPDX-License-Identifier: MPL-2.0

package main

import "github.com/pocketbase/pocketbase/core"

const basicAuthChallenge = `Basic realm="Authorization Required", charset="UTF-8"`

func requireGroupBasicAuth(next func(*core.RequestEvent, *core.Record) error) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		username, password, ok := e.Request.BasicAuth()
		if !ok || username == "" || password == "" {
			return groupBasicAuthUnauthorized(e)
		}

		group, err := e.App.FindFirstRecordByData("groups", "slug", e.Request.PathValue("group"))
		if err != nil || username != group.GetString("username") || !group.ValidatePassword(password) {
			return groupBasicAuthUnauthorized(e)
		}

		e.Set("group", group)

		return next(e, group)
	}
}

func groupBasicAuthUnauthorized(e *core.RequestEvent) error {
	e.Response.Header().Set("WWW-Authenticate", basicAuthChallenge)

	return e.UnauthorizedError("", nil)
}
