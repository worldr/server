// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package storetest

import (
	"testing"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCategoryStore(t *testing.T, ss store.Store) {
	t.Run("testSaveOrUpdateCategory", func(t *testing.T) { testSaveOrUpdateCategory(t, ss) })
	t.Run("testGetCategoriesForUser", func(t *testing.T) { testGetCategoriesForUser(t, ss) })
	t.Run("testDeleteCategory", func(t *testing.T) { testDeleteCategory(t, ss) })
}

func testSaveOrUpdateCategory(t *testing.T, ss store.Store) {
	cat, err := createCategory(t, ss)
	require.Nil(t, err)

	cat, err = ss.ChannelCategory().Get(cat.UserId, cat.ChannelId)
	assert.Nil(t, err)

	newName := cat.Name + "-updated"
	var newSort int32 = 2
	updated := model.ChannelCategory{
		UserId:    cat.UserId,
		ChannelId: cat.ChannelId,
		Name:      newName,
		Sort:      newSort,
	}
	_, err = ss.ChannelCategory().SaveOrUpdate(&updated)
	require.Nil(t, err)
	cat, err = ss.ChannelCategory().Get(cat.UserId, cat.ChannelId)
	require.Nil(t, err)
	assert.EqualValues(t, newSort, cat.Sort)
	assert.EqualValues(t, newName, cat.Name)
}

func testGetCategoriesForUser(t *testing.T, ss store.Store) {
	cat1, err := createCategory(t, ss)
	require.Nil(t, err)

	cat2 := model.ChannelCategory{
		UserId:    cat1.UserId,
		ChannelId: model.NewId(),
		Name:      "Category " + model.NewId(),
		Sort:      2,
	}
	_, err = ss.ChannelCategory().SaveOrUpdate(&cat2)
	require.Nil(t, err)

	cats, err := ss.ChannelCategory().GetForUser(cat1.UserId)
	require.Nil(t, err)

	assert.EqualValues(t, 2, len(*cats))
}

func testDeleteCategory(t *testing.T, ss store.Store) {
	cat, err := createCategory(t, ss)
	require.Nil(t, err)

	err = ss.ChannelCategory().Delete(cat.UserId, cat.ChannelId)
	require.Nil(t, err)

	cat, err = ss.ChannelCategory().Get(cat.UserId, cat.ChannelId)
	require.NotNil(t, err)
	assert.EqualValues(t, "sql: no rows in result set", err.DetailedError)
}

func createCategory(t *testing.T, ss store.Store) (*model.ChannelCategory, *model.AppError) {
	// and a test user
	user := model.User{
		Email:    MakeEmail(),
		Nickname: model.NewId(),
		Username: model.NewId(),
	}
	userPtr, err := ss.User().Save(&user)
	require.Nil(t, err)
	user = *userPtr

	// create a test category
	name := model.NewId()
	cat := model.ChannelCategory{
		UserId:    user.Id,
		ChannelId: model.NewId(),
		Name:      name,
		Sort:      1,
	}
	return ss.ChannelCategory().SaveOrUpdate(&cat)
}
