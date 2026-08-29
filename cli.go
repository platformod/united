// SPDX-License-Identifier: MPL-2.0

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

type testProvisionInput struct {
	OwnerEmail string
	GroupSlug  string
	Username   string
	Password   string
}

type testStateInspection struct {
	States   int  `json:"states"`
	Versions int  `json:"versions"`
	Deleted  bool `json:"deleted"`
}

func registerTestCommands(app *pocketbase.PocketBase) {
	var provision testProvisionInput

	provisionCommand := &cobra.Command{
		Use:    "test-provision",
		Short:  "create an integration-test group credential",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := app.RunAllMigrations(); err != nil {
				return err
			}
			if _, err := provisionTestGroup(app, provision); err != nil {
				return err
			}

			return nil
		},
	}
	provisionCommand.Flags().StringVar(&provision.OwnerEmail, "owner-email", "", "owner email")
	provisionCommand.Flags().StringVar(&provision.GroupSlug, "group-slug", "", "group route slug")
	provisionCommand.Flags().StringVar(&provision.Username, "username", "", "Terraform Basic Auth username")
	provisionCommand.Flags().StringVar(&provision.Password, "password", "", "Terraform Basic Auth password")

	for _, name := range []string{"owner-email", "group-slug", "username", "password"} {
		if err := provisionCommand.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	var groupSlug, stateName string

	inspectCommand := &cobra.Command{
		Use:    "test-inspect",
		Short:  "inspect integration-test state records",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := app.RunAllMigrations(); err != nil {
				return err
			}
			inspection, err := inspectTestState(app, groupSlug, stateName)
			if err != nil {
				return err
			}

			return json.NewEncoder(cmd.OutOrStdout()).Encode(inspection)
		},
	}
	inspectCommand.Flags().StringVar(&groupSlug, "group-slug", "", "group route slug")
	inspectCommand.Flags().StringVar(&stateName, "state-name", "", "state name")

	for _, name := range []string{"group-slug", "state-name"} {
		if err := inspectCommand.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	app.RootCmd.AddCommand(provisionCommand, inspectCommand)
}

func provisionTestGroup(app core.App, input testProvisionInput) (*core.Record, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}

	owner := core.NewRecord(users)
	owner.Set("email", input.OwnerEmail)
	owner.SetPassword(input.Password)

	if err := app.Save(owner); err != nil {
		return nil, fmt.Errorf("create test owner: %w", err)
	}

	groups, err := app.FindCollectionByNameOrId("groups")
	if err != nil {
		return nil, err
	}

	group := core.NewRecord(groups)
	group.Set("email", input.Username+"@terraform.invalid")
	group.Set("username", input.Username)
	group.Set("slug", input.GroupSlug)
	group.Set("displayName", input.GroupSlug)
	group.Set("owner", owner.Id)
	group.SetPassword(input.Password)

	if err := app.Save(group); err != nil {
		return nil, fmt.Errorf("create test group: %w", err)
	}

	return group, nil
}

func inspectTestState(app core.App, groupSlug, stateName string) (testStateInspection, error) {
	group, err := app.FindFirstRecordByData("groups", "slug", groupSlug)
	if err != nil {
		return testStateInspection{}, err
	}

	state, err := findState(app, group.Id, stateName)
	if errors.Is(err, sql.ErrNoRows) {
		return testStateInspection{}, nil
	}

	if err != nil {
		return testStateInspection{}, err
	}

	versions, err := app.FindRecordsByFilter("statefiles", "state = {:stateID}", "", 0, 0, dbx.Params{"stateID": state.Id})
	if err != nil {
		return testStateInspection{}, err
	}

	return testStateInspection{
		States:   1,
		Versions: len(versions),
		Deleted:  !state.GetDateTime("deletedAt").IsZero(),
	}, nil
}
