// Code generated by mockery v1.1.0. DO NOT EDIT.

// Regenerate this file using `make store-mocks`.

package mocks

import (
	mock "github.com/stretchr/testify/mock"

	time "time"
)

// IVault is an autogenerated mock type for the IVault type
type IVault struct {
	mock.Mock
}

// GetSecret provides a mock function with given fields: url, secret, token
func (_m *IVault) GetSecret(url string, secret string, token string) (string, error) {
	ret := _m.Called(url, secret, token)

	var r0 string
	if rf, ok := ret.Get(0).(func(string, string, string) string); ok {
		r0 = rf(url, secret, token)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, string, string) error); ok {
		r1 = rf(url, secret, token)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Getk8sServiceAccountToken provides a mock function with given fields: tokenFile
func (_m *IVault) Getk8sServiceAccountToken(tokenFile string) (*string, error) {
	ret := _m.Called(tokenFile)

	var r0 *string
	if rf, ok := ret.Get(0).(func(string) *string); ok {
		r0 = rf(tokenFile)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(tokenFile)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Login provides a mock function with given fields: url, token
func (_m *IVault) Login(url string, token *string) (*string, error) {
	ret := _m.Called(url, token)

	var r0 *string
	if rf, ok := ret.Get(0).(func(string, *string) *string); ok {
		r0 = rf(url, token)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string, *string) error); ok {
		r1 = rf(url, token)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// WaitForVaultToUnseal provides a mock function with given fields: url, wait, retry
func (_m *IVault) WaitForVaultToUnseal(url string, wait time.Duration, retry int) error {
	ret := _m.Called(url, wait, retry)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, time.Duration, int) error); ok {
		r0 = rf(url, wait, retry)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
