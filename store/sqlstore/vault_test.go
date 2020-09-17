package sqlstore

import (
	"errors"
	"testing"

	"github.com/mattermost/mattermost-server/v5/store/sqlstore/mocks"
	"github.com/stretchr/testify/assert"
)

const (
	KEY_LISTENER_SERVER = "localhost"
	FAKE_TOKEN          = "eyJhbGciOiJSUzI1NiIsImtpZCI6IlZ6dUtoUWxrYUkxX2cwcU44bDhoY1FaYkkxU2k0dUt2UTlhaVU1Q2Nxa2sifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJ3b3JsZHIiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlY3JldC5uYW1lIjoidmF1bHQtYXV0aC10b2tlbi13ODRuNCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJ2YXVsdC1hdXRoIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQudWlkIjoiMjNjOGNlYWQtZGEyYi00ZDU2LWI4NjktZGY5ODIzOTRiMmIyIiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50Ondvcmxkcjp2YXVsdC1hdXRoIn0.OFhEd2TmeEOrsB6iPlwuupIqoinym8rX9Q0EZ8stGT-acq4PGw2lTZIuq_bui8ZX870YS4_f2Jxo3F8Fcaq1641yQsx_M9X2NtHY1QnAD171X8RenxyKZhi58bWOCT5uCxzu8_1HBUQDb-FLHcS1lKB2lTkJ-KgJJEMPOTzOME-_wG5obejEjMt5CkiPodzoI7qPAgPTKsX6dRv9D2TYpm4DhgFQmDIIzX4smwzqm9cuDwrl2tUf2GzIpyZypTK9d8NBAks4cP1stW7ikQgOD7AukkNy8aR0iFNHAH8e-lBkPcbHShGZ6CS2Dx0rlBn0MaHDQRmVy-1p33rMH6D2ow"
)

// There is no k8s service account token on the system.
func TestKeyTalkerNoK8ServiceAccountToken(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken").Return("", nil)

	err := KeyTalker("localhost:8888", mockVault)
	assert.Nil(t, err)
}

// There is a k8s service account token on the system but we cannot read it.
func TestKeyTalkerCannotReadK8ServiceAccountToken(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken").Return("", errors.New("Unit test cannot read token"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

// There is a k8s service account token on the system and we can read it.
func TestKeyTalkerGotK8ServiceAccountToken(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken").Return(FAKE_TOKEN, nil)

	err := KeyTalker("localhost:8888", mockVault)
	assert.Nil(t, err)
}
