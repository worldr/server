// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

type SqlChannelCategoryStore struct {
	SqlStore
}

const (
	tableName    = "ChannelCategories"
	UPDATE_ERROR = "store.sql_channel_category.update.app_error"
	SAVE_ERROR   = "store.sql_channel_category.save.app_error"
	GET_ERROR    = "store.sql_channel_category.get.app_error"
	DELETE_ERROR = "store.sql_channel_category.delete.app_error"
)

func newSqlChannelCategoryStore(sqlStore SqlStore) store.ChannelCategoryStore {
	s := &SqlChannelCategoryStore{sqlStore}

	for _, db := range sqlStore.GetAllConns() {
		table := db.AddTableWithName(model.ChannelCategory{}, tableName).SetKeys(false, "UserId", "ChannelId")
		table.ColMap("UserId").SetMaxSize(26).SetNotNull(true)
		table.ColMap("ChannelId").SetMaxSize(26).SetNotNull(true)
		table.ColMap("Name").SetMaxSize(100).SetNotNull(true)
		table.SetUniqueTogether("UserId", "ChannelId")
	}

	return s
}

func (s SqlChannelCategoryStore) createIndexesIfNotExists() {
	s.CreateCompositeIndexIfNotExists("idx_channel_cat_id", tableName, []string{"UserId", "ChannelId"})
}

func (s SqlChannelCategoryStore) SaveOrUpdate(cat *model.ChannelCategory) (*model.ChannelCategory, *model.AppError) {
	err := s.GetReplica().SelectOne(
		&model.ChannelCategory{},
		"SELECT * FROM ChannelCategories WHERE ChannelId = :ChannelId AND UserId = :UserId",
		map[string]interface{}{"UserId": cat.UserId, "ChannelId": cat.ChannelId},
	)
	if err == nil {
		if _, err := s.GetMaster().Update(cat); err != nil {
			return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", UPDATE_ERROR, nil, err.Error(), http.StatusInternalServerError)
		}
	} else {
		if err := s.GetMaster().Insert(cat); err != nil {
			return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", SAVE_ERROR, nil, err.Error(), http.StatusInternalServerError)
		}
	}
	return s.Get(cat.UserId, cat.ChannelId)
}

func (s SqlChannelCategoryStore) GetForUser(userId string) (*model.ChannelCategoriesList, *model.AppError) {
	var cats = &model.ChannelCategoriesList{}

	if _, err := s.GetReplica().Select(cats,
		`SELECT
			*
		FROM
			ChannelCategories
		WHERE
			UserId = :UserId`, map[string]interface{}{"UserId": userId}); err != nil {
		return nil, model.NewAppError("SqlChannelCategoryStore.GetForUser", GET_ERROR, nil, err.Error(), http.StatusInternalServerError)
	}
	return cats, nil
}

func (s SqlChannelCategoryStore) Get(userId string, channelId string) (*model.ChannelCategory, *model.AppError) {
	var cat *model.ChannelCategory

	if err := s.GetReplica().SelectOne(&cat,
		`SELECT
			*
		FROM
			ChannelCategories
		WHERE
			ChannelId = :ChannelId
			AND
			UserId = :UserId`, map[string]interface{}{"UserId": userId, "ChannelId": channelId}); err != nil {
		return nil, model.NewAppError("SqlChannelCategoryStore.Get", GET_ERROR, nil, err.Error(), http.StatusInternalServerError)
	}

	return cat, nil
}

func (s SqlChannelCategoryStore) Delete(userId string, channelId string) *model.AppError {
	_, err := s.GetMaster().Exec(
		"DELETE FROM ChannelCategories WHERE UserId = :UserId AND ChannelId = :ChannelId",
		map[string]interface{}{"UserId": userId, "ChannelId": channelId},
	)
	if err != nil {
		return model.NewAppError("SqlChannelCategoryStore.Delete", DELETE_ERROR, nil, "", http.StatusInternalServerError)
	}
	return nil
}
