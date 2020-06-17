// Code generated by mockery v1.1.0. DO NOT EDIT.

// Regenerate this file using `make store-mocks`.

package mocks

import (
	model "github.com/mattermost/mattermost-server/v5/model"
	mock "github.com/stretchr/testify/mock"
)

// JobStore is an autogenerated mock type for the JobStore type
type JobStore struct {
	mock.Mock
}

// Delete provides a mock function with given fields: id
func (_m *JobStore) Delete(id string) (string, *model.AppError) {
	ret := _m.Called(id)

	var r0 string
	if rf, ok := ret.Get(0).(func(string) string); ok {
		r0 = rf(id)
	} else {
		r0 = ret.Get(0).(string)
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

// Get provides a mock function with given fields: id
func (_m *JobStore) Get(id string) (*model.Job, *model.AppError) {
	ret := _m.Called(id)

	var r0 *model.Job
	if rf, ok := ret.Get(0).(func(string) *model.Job); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Job)
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

// GetAllByStatus provides a mock function with given fields: status
func (_m *JobStore) GetAllByStatus(status string) ([]*model.Job, *model.AppError) {
	ret := _m.Called(status)

	var r0 []*model.Job
	if rf, ok := ret.Get(0).(func(string) []*model.Job); ok {
		r0 = rf(status)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(status)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetAllByType provides a mock function with given fields: jobType
func (_m *JobStore) GetAllByType(jobType string) ([]*model.Job, *model.AppError) {
	ret := _m.Called(jobType)

	var r0 []*model.Job
	if rf, ok := ret.Get(0).(func(string) []*model.Job); ok {
		r0 = rf(jobType)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string) *model.AppError); ok {
		r1 = rf(jobType)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetAllByTypePage provides a mock function with given fields: jobType, offset, limit
func (_m *JobStore) GetAllByTypePage(jobType string, offset int, limit int) ([]*model.Job, *model.AppError) {
	ret := _m.Called(jobType, offset, limit)

	var r0 []*model.Job
	if rf, ok := ret.Get(0).(func(string, int, int) []*model.Job); ok {
		r0 = rf(jobType, offset, limit)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, int, int) *model.AppError); ok {
		r1 = rf(jobType, offset, limit)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetAllPage provides a mock function with given fields: offset, limit
func (_m *JobStore) GetAllPage(offset int, limit int) ([]*model.Job, *model.AppError) {
	ret := _m.Called(offset, limit)

	var r0 []*model.Job
	if rf, ok := ret.Get(0).(func(int, int) []*model.Job); ok {
		r0 = rf(offset, limit)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(int, int) *model.AppError); ok {
		r1 = rf(offset, limit)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetCountByStatusAndType provides a mock function with given fields: status, jobType
func (_m *JobStore) GetCountByStatusAndType(status string, jobType string) (int64, *model.AppError) {
	ret := _m.Called(status, jobType)

	var r0 int64
	if rf, ok := ret.Get(0).(func(string, string) int64); ok {
		r0 = rf(status, jobType)
	} else {
		r0 = ret.Get(0).(int64)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, string) *model.AppError); ok {
		r1 = rf(status, jobType)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// GetNewestJobByStatusAndType provides a mock function with given fields: status, jobType
func (_m *JobStore) GetNewestJobByStatusAndType(status string, jobType string) (*model.Job, *model.AppError) {
	ret := _m.Called(status, jobType)

	var r0 *model.Job
	if rf, ok := ret.Get(0).(func(string, string) *model.Job); ok {
		r0 = rf(status, jobType)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, string) *model.AppError); ok {
		r1 = rf(status, jobType)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// Save provides a mock function with given fields: job
func (_m *JobStore) Save(job *model.Job) (*model.Job, *model.AppError) {
	ret := _m.Called(job)

	var r0 *model.Job
	if rf, ok := ret.Get(0).(func(*model.Job) *model.Job); ok {
		r0 = rf(job)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(*model.Job) *model.AppError); ok {
		r1 = rf(job)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// UpdateOptimistically provides a mock function with given fields: job, currentStatus
func (_m *JobStore) UpdateOptimistically(job *model.Job, currentStatus string) (bool, *model.AppError) {
	ret := _m.Called(job, currentStatus)

	var r0 bool
	if rf, ok := ret.Get(0).(func(*model.Job, string) bool); ok {
		r0 = rf(job, currentStatus)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(*model.Job, string) *model.AppError); ok {
		r1 = rf(job, currentStatus)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// UpdateStatus provides a mock function with given fields: id, status
func (_m *JobStore) UpdateStatus(id string, status string) (*model.Job, *model.AppError) {
	ret := _m.Called(id, status)

	var r0 *model.Job
	if rf, ok := ret.Get(0).(func(string, string) *model.Job); ok {
		r0 = rf(id, status)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*model.Job)
		}
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, string) *model.AppError); ok {
		r1 = rf(id, status)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}

// UpdateStatusOptimistically provides a mock function with given fields: id, currentStatus, newStatus
func (_m *JobStore) UpdateStatusOptimistically(id string, currentStatus string, newStatus string) (bool, *model.AppError) {
	ret := _m.Called(id, currentStatus, newStatus)

	var r0 bool
	if rf, ok := ret.Get(0).(func(string, string, string) bool); ok {
		r0 = rf(id, currentStatus, newStatus)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 *model.AppError
	if rf, ok := ret.Get(1).(func(string, string, string) *model.AppError); ok {
		r1 = rf(id, currentStatus, newStatus)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*model.AppError)
		}
	}

	return r0, r1
}
