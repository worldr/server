// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"testing"

	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/stretchr/testify/require"
)

func TestStoreUpgrade(t *testing.T) {
	StoreTest(t, func(t *testing.T, ss store.Store) {
		sqlStore := ss.(SqlStore)

		t.Run("invalid currentModelVersion", func(t *testing.T) {
			err := upgradeDatabase(sqlStore, "notaversion")
			require.EqualError(t, err, "failed to parse current model version notaversion: No Major.Minor.Patch elements found")
		})

		t.Run("upgrade from invalid version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, "invalid")
			err := upgradeDatabase(sqlStore, "6.0.0")
			require.EqualError(t, err, "failed to parse database schema version invalid: No Major.Minor.Patch elements found")
			require.Equal(t, "invalid", sqlStore.GetCurrentSchemaVersion())
		})

		t.Run("upgrade from unsupported version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, "5.0.0")
			err := upgradeDatabase(sqlStore, "6.1.0")
			require.EqualError(t, err, "Database schema version 5.0.0 is no longer supported. This server supports automatic upgrades from schema version 6.0.0 through schema version 6.1.0. Please manually upgrade to at least version 6.0.0 before continuing.")
			require.Equal(t, "5.0.0", sqlStore.GetCurrentSchemaVersion())
		})

		t.Run("upgrade from earliest supported version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, OLDEST_SUPPORTED_VERSION)
			err := upgradeDatabase(sqlStore, CURRENT_SCHEMA_VERSION)
			require.NoError(t, err)
			require.Equal(t, CURRENT_SCHEMA_VERSION, sqlStore.GetCurrentSchemaVersion())
		})

		t.Run("upgrade from no existing version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, "")
			err := upgradeDatabase(sqlStore, CURRENT_SCHEMA_VERSION)
			require.NoError(t, err)
			require.Equal(t, CURRENT_SCHEMA_VERSION, sqlStore.GetCurrentSchemaVersion())
		})

		// TODO: This test is irrelevant to Worldr. Add it back when we have a new minor version.
		// The test is going to be relevant when we have at least two migrations.
		// Right now we have only one and the test doesn't do anything.
		// t.Run("upgrade schema running earlier minor version", func(t *testing.T) {
		// 	saveSchemaVersion(sqlStore, "6.0.0")
		// 	err := upgradeDatabase(sqlStore, "6.1.0")
		// 	require.NoError(t, err)
		// 	// The migrations will move past 6.1.0 regardless of the input parameter.
		// 	require.Equal(t, CURRENT_SCHEMA_VERSION, sqlStore.GetCurrentSchemaVersion())
		// })

		t.Run("upgrade schema running later minor version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, "6.0.0")
			err := upgradeDatabase(sqlStore, "6.1.0")
			require.NoError(t, err)
			require.Equal(t, "6.1.0", sqlStore.GetCurrentSchemaVersion())
		})

		// TODO: This test is irrelevant to Worldr. Add it back when we have a new major version.
		// t.Run("upgrade schema running earlier major version", func(t *testing.T) {
		// 	saveSchemaVersion(sqlStore, "4.1.0")
		// 	err := upgradeDatabase(sqlStore, CURRENT_SCHEMA_VERSION)
		// 	require.NoError(t, err)
		// 	require.Equal(t, CURRENT_SCHEMA_VERSION, sqlStore.GetCurrentSchemaVersion())
		// })

		t.Run("upgrade schema running later major version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, "7.1.0")
			err := upgradeDatabase(sqlStore, "6.1.0")
			require.EqualError(t, err, "Database schema version 7.1.0 is not supported. This server supports versions >=6.1.0, <7.0.0. Please upgrade to at least version 7.0.0 before continuing.")
			require.Equal(t, "7.1.0", sqlStore.GetCurrentSchemaVersion())
		})
	})
}

func TestSaveSchemaVersion(t *testing.T) {
	StoreTest(t, func(t *testing.T, ss store.Store) {
		sqlStore := ss.(SqlStore)

		t.Run("set earliest version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, OLDEST_SUPPORTED_VERSION)
			props, err := ss.System().Get()
			require.Nil(t, err)

			require.Equal(t, OLDEST_SUPPORTED_VERSION, props["Version"])
			require.Equal(t, OLDEST_SUPPORTED_VERSION, sqlStore.GetCurrentSchemaVersion())
		})

		t.Run("set current version", func(t *testing.T) {
			saveSchemaVersion(sqlStore, CURRENT_SCHEMA_VERSION)
			props, err := ss.System().Get()
			require.Nil(t, err)

			require.Equal(t, CURRENT_SCHEMA_VERSION, props["Version"])
			require.Equal(t, CURRENT_SCHEMA_VERSION, sqlStore.GetCurrentSchemaVersion())
		})
	})
}
