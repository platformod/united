// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
