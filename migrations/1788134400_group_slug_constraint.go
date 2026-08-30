// SPDX-License-Identifier: MPL-2.0

package migrations

import (
	"errors"

	"github.com/pocketbase/pocketbase/core"
)

const groupSlugPattern = "^[a-z0-9]+(?:-[a-z0-9]+)*$"

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		groups, err := txApp.FindCollectionByNameOrId(groupsCollectionID)
		if err != nil {
			return err
		}

		slug, ok := groups.Fields.GetByName("slug").(*core.TextField)
		if !ok {
			return errors.New("groups slug field is missing or not text")
		}
		slug.Pattern = groupSlugPattern

		return txApp.Save(groups)
	}, func(txApp core.App) error {
		groups, err := txApp.FindCollectionByNameOrId(groupsCollectionID)
		if err != nil {
			return err
		}

		slug, ok := groups.Fields.GetByName("slug").(*core.TextField)
		if !ok {
			return errors.New("groups slug field is missing or not text")
		}
		slug.Pattern = ""

		return txApp.Save(groups)
	})
}
