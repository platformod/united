// SPDX-License-Identifier: MPL-2.0

package migrations

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/pocketbase/pocketbase/core"
)

const groupSlugPattern = "^[a-z0-9]+(?:-[a-z0-9]+)*$"

var validGroupSlug = regexp.MustCompile(groupSlugPattern)

func init() {
	core.AppMigrations.Register(func(txApp core.App) error {
		groups, err := txApp.FindCollectionByNameOrId(groupsCollectionID)
		if err != nil {
			return err
		}

		if err := rejectUnsafeGroupSlugs(txApp); err != nil {
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

func rejectUnsafeGroupSlugs(app core.App) error {
	groups, err := app.FindRecordsByFilter("groups", "", "id", 0, 0, nil)
	if err != nil {
		return err
	}

	for _, group := range groups {
		slug := group.GetString("slug")
		if !validGroupSlug.MatchString(slug) {
			return fmt.Errorf("group %s has invalid immutable route slug %q; migrate its Terraform state to a group with a safe slug before applying this migration", group.Id, slug)
		}
	}

	return nil
}
