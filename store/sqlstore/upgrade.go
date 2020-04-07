// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"os"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
)

const (
	CURRENT_SCHEMA_VERSION   = VERSION_6_0_0
	VERSION_6_0_1            = "6.0.1"
	VERSION_6_0_0            = "6.0.0"
	VERSION_3_0_0            = "3.0.0"
	OLDEST_SUPPORTED_VERSION = VERSION_3_0_0
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
		return errors.Errorf("Database schema version %s is no longer supported. This Mattermost server supports automatic upgrades from schema version %s through schema version %s. Please manually upgrade to at least version %s before continuing.", *currentSchemaVersion, oldestSupportedVersion, currentModelVersion, oldestSupportedVersion)
	}

	// Allow forwards compatibility only within the same major version.
	if currentSchemaVersion.GTE(nextUnsupportedMajorVersion) {
		return errors.Errorf("Database schema version %s is not supported. This Mattermost server supports only >=%s, <%s. Please upgrade to at least version %s before continuing.", *currentSchemaVersion, currentModelVersion, nextUnsupportedMajorVersion, nextUnsupportedMajorVersion)
	} else if currentSchemaVersion.GT(currentModelVersion) {
		mlog.Warn("The database schema version and model versions do not match", mlog.String("schema_version", currentSchemaVersion.String()), mlog.String("model_version", currentModelVersion.String()))
	}

	upgradeDatabaseToVersion600(sqlStore)
	upgradeDatabaseToVersion601(sqlStore)

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
	}

	return false
}

func upgradeDatabaseToVersion600(sqlStore SqlStore) {
	saveSchemaVersion(sqlStore, VERSION_6_0_0)
}

// Uncomment when need be
func upgradeDatabaseToVersion601(sqlStore SqlStore) {
	if shouldPerformUpgrade(sqlStore, VERSION_6_0_0, VERSION_6_0_1) {
		//
		// TODO
		//
		//saveSchemaVersion(sqlStore, VERSION_6_0_0)
	}
}
