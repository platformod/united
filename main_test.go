// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoTestProvisionCommand(t *testing.T) {
	for _, command := range []string{"test-provision", "test-inspect"} {
		t.Run(command, func(t *testing.T) {
			app := newApp(Config{StateMasterKey: make([]byte, 32)}, t.TempDir())
			app.RootCmd.SetArgs([]string{command})

			err := app.RootCmd.Execute()
			require.Error(t, err)
			require.Contains(t, err.Error(), `unknown command "`+command+`"`)
		})
	}
}
