// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAppRegistersTestCommands(t *testing.T) {
	app := newApp(Config{StateMasterKey: make([]byte, 32)}, t.TempDir())

	for _, name := range []string{"test-provision", "test-inspect"} {
		command, _, err := app.RootCmd.Find([]string{name})
		require.NoError(t, err)
		require.Equal(t, name, command.Name())
	}
}

func TestProvisionTestGroupCreatesOwnerAndCredential(t *testing.T) {
	app := newTestApp(t)

	group, err := provisionTestGroup(app, testProvisionInput{
		OwnerEmail: "owner@example.test",
		GroupSlug:  "integration",
		Username:   "terraform",
		Password:   "test-password",
	})
	require.NoError(t, err)
	require.Equal(t, "integration", group.GetString("slug"))
	require.Equal(t, "terraform", group.GetString("username"))
	require.True(t, group.ValidatePassword("test-password"))

	owner, err := app.FindRecordById("users", group.GetString("owner"))
	require.NoError(t, err)
	require.Equal(t, "owner@example.test", owner.GetString("email"))
}

func TestInspectTestStateReportsVersionsAndDeletion(t *testing.T) {
	app := newTestApp(t)
	owner := createUser(t, app)
	group := createGroup(t, app, owner, "integration", "terraform", "test-password")
	state := createState(t, app, group, "network")

	inspection, err := inspectTestState(app, "integration", "network")
	require.NoError(t, err)
	require.Equal(t, testStateInspection{States: 1, Versions: 0, Deleted: false}, inspection)

	state.Set("deletedAt", "2026-08-29 00:00:00.000Z")
	require.NoError(t, app.Save(state))

	inspection, err = inspectTestState(app, "integration", "network")
	require.NoError(t, err)
	require.Equal(t, testStateInspection{States: 1, Versions: 0, Deleted: true}, inspection)
}
