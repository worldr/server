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
		table := db.AddTableWithName(model.ChannelCategory{}, tableName).SetKeys(true, "Id")
		table.ColMap("UserId").SetMaxSize(26)
		table.ColMap("Name").SetMaxSize(100).SetNotNull(true)
	}

	return s
}

func (s SqlChannelCategoryStore) createIndexesIfNotExists() {
	s.CreateIndexIfNotExists("idx_channel_cat_user_id", tableName, "UserId")
}

func (s SqlChannelCategoryStore) SaveOrUpdate(cat *model.ChannelCategory) (*model.ChannelCategory, *model.AppError) {
	err := s.GetReplica().SelectOne(
		&model.ChannelCategory{},
		"SELECT * FROM ChannelCategories WHERE Id = :Id",
		map[string]interface{}{"Id": cat.Id},
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
	return s.Get(cat.Id)
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

func (s SqlChannelCategoryStore) Get(catId int32) (*model.ChannelCategory, *model.AppError) {
	var cat *model.ChannelCategory

	if err := s.GetReplica().SelectOne(&cat,
		`SELECT
			*
		FROM
			ChannelCategories
		WHERE
			Id = :Id`, map[string]interface{}{"Id": catId}); err != nil {
		return nil, model.NewAppError("SqlChannelCategoryStore.Get", GET_ERROR, nil, err.Error(), http.StatusInternalServerError)
	}

	return cat, nil
}

func (s SqlChannelCategoryStore) Delete(userId string, catId int32) *model.AppError {
	_, err := s.GetMaster().Exec(
		"DELETE FROM ChannelCategories WHERE UserId = :UserId AND Id = :Id",
		map[string]interface{}{"UserId": userId, "Id": catId},
	)
	if err != nil {
		return model.NewAppError("SqlChannelCategoryStore.Delete", DELETE_ERROR, nil, "", http.StatusInternalServerError)
	}
	return nil
}
