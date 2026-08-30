// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/require"
)

func TestGroupSlugMigrationRejectsExistingUnsafeSlug(t *testing.T) {
	app := newApp(Config{StateMasterKey: make([]byte, 32)}, t.TempDir())
	require.NoError(t, app.Bootstrap())
	t.Cleanup(func() {
		require.NoError(t, app.ResetBootstrapState())
	})

	require.NoError(t, app.RunSystemMigrations())

	var slugMigration *core.Migration
	var priorMigrations core.MigrationsList
	for _, migration := range core.AppMigrations.Items() {
		if migration.File == "1788134400_group_slug_constraint.go" {
			slugMigration = migration
			continue
		}
		priorMigrations.Register(migration.Up, migration.Down, migration.File)
	}
	require.NotNil(t, slugMigration)
	_, err := core.NewMigrationsRunner(app, priorMigrations).Up()
	require.NoError(t, err)

	unsafeSlug := "platform/production"
	group := createGroup(t, app, createUser(t, app), unsafeSlug, "platform-tf", testPassword(t))

	err = app.RunInTransaction(func(txApp core.App) error {
		return slugMigration.Up(txApp)
	})
	require.Error(t, err)
	require.ErrorContains(t, err, group.Id)
	require.ErrorContains(t, err, unsafeSlug)
}

func TestInitialSchemaCreatesPrivateStateCollections(t *testing.T) {
	app := newTestApp(t)

	for _, name := range []string{"users", "groups", "states", "statefiles"} {
		_, err := app.FindCollectionByNameOrId(name)
		require.NoError(t, err, name)
	}

	groups, err := app.FindCollectionByNameOrId("groups")
	require.NoError(t, err)
	require.False(t, groups.PasswordAuth.Enabled)

	states, err := app.FindCollectionByNameOrId("states")
	require.NoError(t, err)
	require.Nil(t, states.ListRule)
	require.Nil(t, states.ViewRule)
}
