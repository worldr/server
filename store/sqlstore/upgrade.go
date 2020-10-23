// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"fmt"
	"os"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
)

const (
	CURRENT_SCHEMA_VERSION   = VERSION_6_2_0
	VERSION_6_2_0            = "6.2.0"
	VERSION_6_1_0            = "6.1.0"
	VERSION_6_0_0            = "6.0.0"
	OLDEST_SUPPORTED_VERSION = VERSION_6_0_0
)

const (
	EXIT_VERSION_SAVE                   = 1003
	EXIT_THEME_MIGRATION                = 1004
	EXIT_TEAM_INVITEID_MIGRATION_FAILED = 1006
)

// upgradeDatabase attempts to migrate the schema to the latest supported version.
// The value of model.CurrentVersion is accepted as a parameter for unit testing, but it is not
// used to stop migrations at that version.
func upgradeDatabase(sqlStore SqlStore, currentModelVersionString string) error {
	currentModelVersion, err := semver.Parse(currentModelVersionString)
	if err != nil {
		return errors.Wrapf(err, "failed to parse current model version %s", currentModelVersionString)
	}

	nextUnsupportedMajorVersion := semver.Version{
		Major: currentModelVersion.Major + 1,
	}

	oldestSupportedVersion, err := semver.Parse(OLDEST_SUPPORTED_VERSION)
	if err != nil {
		return errors.Wrapf(err, "failed to parse oldest supported version %s", OLDEST_SUPPORTED_VERSION)
	}

	var currentSchemaVersion *semver.Version
	currentSchemaVersionString := sqlStore.GetCurrentSchemaVersion()
	if currentSchemaVersionString != "" {
		currentSchemaVersion, err = semver.New(currentSchemaVersionString)
		if err != nil {
			return errors.Wrapf(err, "failed to parse database schema version %s", currentSchemaVersionString)
		}
	}

	// Assume a fresh database if no schema version has been recorded.
	if currentSchemaVersion == nil {
		if err := sqlStore.System().SaveOrUpdate(&model.System{Name: "Version", Value: currentModelVersion.String()}); err != nil {
			return errors.Wrap(err, "failed to initialize schema version for fresh database")
		}

		currentSchemaVersion = &currentModelVersion
		mlog.Info("The database schema version has been set", mlog.String("version", currentSchemaVersion.String()))
		return nil
	}

	// Upgrades prior to the oldest supported version are not supported.
	if currentSchemaVersion.LT(oldestSupportedVersion) {
		return errors.Errorf("Database schema version %s is no longer supported. This server supports automatic upgrades from schema version %s through schema version %s. Please manually upgrade to at least version %s before continuing.", *currentSchemaVersion, oldestSupportedVersion, currentModelVersion, oldestSupportedVersion)
	}

	// Allow forwards compatibility only within the same major version.
	if currentSchemaVersion.GTE(nextUnsupportedMajorVersion) {
		return errors.Errorf("Database schema version %s is not supported. This server supports versions >=%s, <%s. Please upgrade to at least version %s before continuing.", *currentSchemaVersion, currentModelVersion, nextUnsupportedMajorVersion, nextUnsupportedMajorVersion)
	} else if currentSchemaVersion.GT(currentModelVersion) {
		mlog.Warn("The database schema version and model versions do not match", mlog.String("schema_version", currentSchemaVersion.String()), mlog.String("model_version", currentModelVersion.String()))
	}

	upgradeDatabaseToVersion610(sqlStore)
	upgradeDatabaseToVersion620(sqlStore)

	return nil
}

func saveSchemaVersion(sqlStore SqlStore, version string) {
	if err := sqlStore.System().SaveOrUpdate(&model.System{Name: "Version", Value: version}); err != nil {
		mlog.Critical(err.Error())
		time.Sleep(time.Second)
		os.Exit(EXIT_VERSION_SAVE)
	}

	mlog.Warn("The database schema version has been upgraded", mlog.String("version", version))
}

func shouldPerformUpgrade(sqlStore SqlStore, currentSchemaVersion string, expectedSchemaVersion string) bool {
	if sqlStore.GetCurrentSchemaVersion() == currentSchemaVersion {
		mlog.Warn("Attempting to upgrade the database schema version", mlog.String("current_version", currentSchemaVersion), mlog.String("new_version", expectedSchemaVersion))
		return true
	} else {
		mlog.Info("Skipping schema upgrade", mlog.String("current_version", currentSchemaVersion), mlog.String("new_version", expectedSchemaVersion))
	}
	return false
}

func upgradeDatabaseToVersion610(sqlStore SqlStore) {
	if shouldPerformUpgrade(sqlStore, VERSION_6_0_0, VERSION_6_1_0) {
		sqlStore.CreateColumnIfNotExists("Posts", "ReplyToId", "varchar(26)", "varchar(26)", "")
		saveSchemaVersion(sqlStore, VERSION_6_1_0)
	}
}

func upgradeDatabaseToVersion620(sqlStore SqlStore) {
	if shouldPerformUpgrade(sqlStore, VERSION_6_1_0, VERSION_6_2_0) {
		sqlStore.CreateColumnIfNotExists("ChannelCategories", "Id", "varchar(100)", "varchar(100)", "")

		// Update categories previously stored

		// 1. Get all users that have categories
		var uids []string
		_, err := sqlStore.GetReplica().Select(&uids, "SELECT UserId FROM ChannelCategories GROUP BY UserId")
		if err != nil {
			mlog.Error("Error fetching user ids from ChannelCategories", mlog.Err(err))
			panic(fmt.Sprintf("Migration to %v failed", VERSION_6_2_0))
		}

		// 2. For each user update categories, setting Id and removing Id collisions
		for _, v := range uids {
			cats := make([]*model.ChannelCategory, 0)
			names := make(map[string]string)
			sorts := make(map[string]int32)
			_, err := sqlStore.GetReplica().Select(
				&cats,
				"SELECT * FROM ChannelCategories WHERE UserId = :UserId ORDER BY ChannelId",
				map[string]interface{}{"UserId": v},
			)
			if err != nil {
				mlog.Error("Error categories for user", mlog.Err(err))
				panic(fmt.Sprintf("Migration to %v failed", VERSION_6_2_0))
			}
			for _, c := range cats {
				c.Id = c.GetId()
				if _, exists := names[c.Id]; !exists {
					names[c.Id] = c.Name
					sorts[c.Id] = c.Sort
				}
				c.Name = names[c.Id]
				c.Sort = sorts[c.Id]
				sqlStore.ChannelCategory().SaveOrUpdate(c)
			}
		}

		saveSchemaVersion(sqlStore, VERSION_6_2_0)
	}
}
