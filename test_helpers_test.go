// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func newTestApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()

	app := newApp(Config{StateMasterKey: make([]byte, 32)}, t.TempDir())
	require.NoError(t, app.Bootstrap())
	require.NoError(t, app.RunAllMigrations())
	t.Cleanup(func() {
		require.NoError(t, app.ResetBootstrapState())
	})

	return app
}

func createUser(t *testing.T, app core.App) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)
	user := core.NewRecord(collection)
	user.Set("email", "owner@example.test")
	user.SetPassword("correct horse")
	require.NoError(t, app.Save(user))
	return user
}

func createGroup(t *testing.T, app core.App, owner *core.Record, slug, username, password string) *core.Record {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("groups")
	require.NoError(t, err)
	group := core.NewRecord(collection)
	group.Set("email", username+"@terraform.invalid")
	group.Set("username", username)
	group.Set("slug", slug)
	group.Set("displayName", slug)
	group.Set("owner", owner.Id)
	group.SetPassword(password)
	require.NoError(t, app.Save(group))
	return group
}
