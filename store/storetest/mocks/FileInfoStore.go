// Code generated by mockery v1.1.0. DO NOT EDIT.

// Regenerate this file using `make store-mocks`.

package mocks

import (
	model "github.com/mattermost/mattermost-server/v5/model"
	mock "github.com/stretchr/testify/mock"
)

// FileInfoStore is an autogenerated mock type for the FileInfoStore type
type FileInfoStore struct {
	mock.Mock
}

// AttachToPost provides a mock function with given fields: fileId, postId, creatorId
func (_m *FileInfoStore) AttachToPost(fileId string, postId string, creatorId string) *model.AppError {
	ret := _m.Called(fileId, postId, creatorId)

	var r0 *model.AppError
	if rf, ok := ret.Get(0).(func(string, string, string) *model.AppError); ok {
		r0 = rf(fileId, postId, creatorId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.AppError)
		}
	}

	return r0
}

// ClearCaches provides a mock function with given fields:
func (_m *FileInfoStore) ClearCaches() {
	_m.Called()
}

// DeleteForPost provides a mock function with given fields: postId
func (_m *FileInfoStore) DeleteForPost(postId string) (string, *model.AppError) {
	ret := _m.Called(postId)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(postId)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(postId)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// Get provides a mock function with given fields: id
func (_m *FileInfoStore) Get(id string) (*model.FileInfo, *model.AppError) {
	ret := _m.Called(id)

	var r0 *model.FileInfo
	if rf, ok := ret.Get(0).(func(string) *model.FileInfo); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.FileInfo)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(id)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetByPath provides a mock function with given fields: path
func (_m *FileInfoStore) GetByPath(path string) (*model.FileInfo, *model.AppError) {
	ret := _m.Called(path)

	var r0 *model.FileInfo
	if rf, ok := ret.Get(0).(func(string) *model.FileInfo); ok {
		r0 = rf(path)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.FileInfo)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(path)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetForPost provides a mock function with given fields: postId, readFromMaster, includeDeleted, allowFromCache
func (_m *FileInfoStore) GetForPost(postId string, readFromMaster bool, includeDeleted bool, allowFromCache bool) ([]*model.FileInfo, *model.AppError) {
	ret := _m.Called(postId, readFromMaster, includeDeleted, allowFromCache)

	var r0 []*model.FileInfo
	if rf, ok := ret.Get(0).(func(string, bool, bool, bool) []*model.FileInfo); ok {
		r0 = rf(postId, readFromMaster, includeDeleted, allowFromCache)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.FileInfo)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, bool, bool, bool) *model.AppError); ok {
		r1 = rf(postId, readFromMaster, includeDeleted, allowFromCache)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetForUser provides a mock function with given fields: userId
func (_m *FileInfoStore) GetForUser(userId string) ([]*model.FileInfo, *model.AppError) {
	ret := _m.Called(userId)

	var r0 []*model.FileInfo
	if rf, ok := ret.Get(0).(func(string) []*model.FileInfo); ok {
		r0 = rf(userId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.FileInfo)
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

// GetWithOptions provides a mock function with given fields: page, perPage, opt
func (_m *FileInfoStore) GetWithOptions(page int, perPage int, opt *model.GetFileInfosOptions) ([]*model.FileInfo, *model.AppError) {
	ret := _m.Called(page, perPage, opt)

	var r0 []*model.FileInfo
	if rf, ok := ret.Get(0).(func(int, int, *model.GetFileInfosOptions) []*model.FileInfo); ok {
		r0 = rf(page, perPage, opt)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.FileInfo)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(int, int, *model.GetFileInfosOptions) *model.AppError); ok {
		r1 = rf(page, perPage, opt)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// InvalidateFileInfosForPostCache provides a mock function with given fields: postId, deleted
func (_m *FileInfoStore) InvalidateFileInfosForPostCache(postId string, deleted bool) {
	_m.Called(postId, deleted)
}

// PermanentDelete provides a mock function with given fields: fileId
func (_m *FileInfoStore) PermanentDelete(fileId string) *model.AppError {
	ret := _m.Called(fileId)

	var r0 *model.AppError
	if rf, ok := ret.Get(0).(func(string) *model.AppError); ok {
		r0 = rf(fileId)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.AppError)
		}
	}

	return r0
}

// PermanentDeleteBatch provides a mock function with given fields: endTime, limit
func (_m *FileInfoStore) PermanentDeleteBatch(endTime int64, limit int64) (int64, *model.AppError) {
	ret := _m.Called(endTime, limit)

	var r0 int64
	if rf, ok := ret.Get(0).(func(int64, int64) int64); ok {
		r0 = rf(endTime, limit)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(int64, int64) *model.AppError); ok {
		r1 = rf(endTime, limit)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// PermanentDeleteByUser provides a mock function with given fields: userId
func (_m *FileInfoStore) PermanentDeleteByUser(userId string) (int64, *model.AppError) {
	ret := _m.Called(userId)

	var r0 int64
	if rf, ok := ret.Get(0).(func(string) int64); ok {
		r0 = rf(userId)
	} else {
		r0 = ret.Get(0).(int64)
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

// Save provides a mock function with given fields: info
func (_m *FileInfoStore) Save(info *model.FileInfo) (*model.FileInfo, *model.AppError) {
	ret := _m.Called(info)

	var r0 *model.FileInfo
	if rf, ok := ret.Get(0).(func(*model.FileInfo) *model.FileInfo); ok {
		r0 = rf(info)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.FileInfo)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(*model.FileInfo) *model.AppError); ok {
		r1 = rf(info)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}
