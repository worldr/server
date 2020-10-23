// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package sqlstore

import (
	"fmt"
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
		table.ColMap("Id").SetMaxSize(100).SetNotNull(true)
		table.SetUniqueTogether("UserId", "ChannelId")
	}

	return s
}

func (s SqlChannelCategoryStore) createIndexesIfNotExists() {
	s.CreateCompositeIndexIfNotExists("idx_channel_cat_id", tableName, []string{"UserId", "ChannelId"})
}

func (s SqlChannelCategoryStore) SaveOrUpdate(cat *model.ChannelCategory) (*model.ChannelCategory, *model.AppError) {
	cat.Id = cat.GetId()
	if !cat.IsValidCategory() {
		return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", SAVE_ERROR, nil, fmt.Sprintf("Category is invalid %+v", cat), http.StatusBadRequest)
	}

	// Does the channel have category assigned?
	channelLabeledErr := s.GetReplica().SelectOne(
		&model.ChannelCategory{},
		"SELECT * FROM ChannelCategories WHERE ChannelId = :ChannelId AND UserId = :UserId",
		map[string]interface{}{"UserId": cat.UserId, "ChannelId": cat.ChannelId},
	)

	// Does the category with this Id exist for user?
	existing := &model.ChannelCategory{}
	categoryExistsErr := s.GetReplica().SelectOne(
		existing,
		"SELECT * FROM ChannelCategories WHERE UserId = :UserId AND Id = :Id",
		map[string]interface{}{"UserId": cat.UserId, "Id": cat.Id},
	)

	if categoryExistsErr == nil {
		// Don't change sorting if the category exists
		cat.Sort = existing.Sort
	}

	// Insert or update the record for the channel
	if channelLabeledErr == nil {
		if _, err := s.GetMaster().Update(cat); err != nil {
			return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", UPDATE_ERROR, nil, err.Error(), http.StatusInternalServerError)
		}
	} else {
		if err := s.GetMaster().Insert(cat); err != nil {
			return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", SAVE_ERROR, nil, err.Error(), http.StatusInternalServerError)
		}
	}

	// Update the name of the category for all records of the user
	_, err := s.GetReplica().Exec(
		`UPDATE ChannelCategories SET Name = :Name WHERE UserId = :UserId AND Id = :Id`,
		map[string]interface{}{"Name": cat.Name, "Id": cat.Id, "UserId": cat.UserId},
	)
	if err != nil {
		return nil, model.NewAppError("SqlChannelCategoryStore.SaveOrUpdate", UPDATE_ERROR, nil, err.Error(), http.StatusInternalServerError)
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
			UserId = :UserId
		ORDER BY Sort, Id`, map[string]interface{}{"UserId": userId}); err != nil {
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

func (s SqlChannelCategoryStore) SetOrder(userId string, name string, order int32) *model.AppError {
	id := model.GetChannelCategoryId(name)
	existing := &model.ChannelCategory{}
	categoryExistsErr := s.GetReplica().SelectOne(
		existing,
		"SELECT * FROM ChannelCategories WHERE UserId = :UserId AND Id = :Id",
		map[string]interface{}{"UserId": userId, "Id": id},
	)
	if categoryExistsErr != nil {
		return model.NewAppError("SqlChannelCategoryStore.SetOrder", UPDATE_ERROR, nil, categoryExistsErr.Error(), http.StatusNotFound)
	}
	_, err := s.GetMaster().Exec(
		`UPDATE ChannelCategories SET Sort = :Sort WHERE UserId = :UserId AND Id = :Id`,
		map[string]interface{}{"Id": id, "UserId": userId, "Sort": order},
	)
	if err != nil {
		return model.NewAppError("SqlChannelCategoryStore.SetOrder", UPDATE_ERROR, nil, err.Error(), http.StatusInternalServerError)
	}
	return nil
}
