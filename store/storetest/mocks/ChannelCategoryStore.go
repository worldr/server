// Code generated by mockery v1.0.0. DO NOT EDIT.

// Regenerate this file using `make store-mocks`.

package mocks

import (
	model "github.com/mattermost/mattermost-server/v5/model"
	mock "github.com/stretchr/testify/mock"
)

// ChannelCategoryStore is an autogenerated mock type for the ChannelCategoryStore type
type ChannelCategoryStore struct {
	mock.Mock
}

// Delete provides a mock function with given fields: userId, catId
func (_m *ChannelCategoryStore) Delete(userId string, catId int32) *model.AppError {
	ret := _m.Called(userId, catId)

	var r0 *model.AppError
	if rf, ok := ret.Get(0).(func(string, int32) *model.AppError); ok {
		r0 = rf(userId, catId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.AppError)
		}
	}

	return r0
}

// Get provides a mock function with given fields: catId
func (_m *ChannelCategoryStore) Get(catId int32) (*model.ChannelCategory, *model.AppError) {
	ret := _m.Called(catId)

	var r0 *model.ChannelCategory
	if rf, ok := ret.Get(0).(func(int32) *model.ChannelCategory); ok {
		r0 = rf(catId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ChannelCategory)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(int32) *model.AppError); ok {
		r1 = rf(catId)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetForUser provides a mock function with given fields: userId
func (_m *ChannelCategoryStore) GetForUser(userId string) (*model.ChannelCategoriesList, *model.AppError) {
	ret := _m.Called(userId)

	var r0 *model.ChannelCategoriesList
	if rf, ok := ret.Get(0).(func(string) *model.ChannelCategoriesList); ok {
		r0 = rf(userId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ChannelCategoriesList)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(userId)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// SaveOrUpdate provides a mock function with given fields: cat
func (_m *ChannelCategoryStore) SaveOrUpdate(cat *model.ChannelCategory) (*model.ChannelCategory, *model.AppError) {
	ret := _m.Called(cat)

	var r0 *model.ChannelCategory
	if rf, ok := ret.Get(0).(func(*model.ChannelCategory) *model.ChannelCategory); ok {
		r0 = rf(cat)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.ChannelCategory)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(*model.ChannelCategory) *model.AppError); ok {
		r1 = rf(cat)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}
