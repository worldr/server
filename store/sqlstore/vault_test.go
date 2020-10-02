package sqlstore

// gotest -v vault_test.go vault.go -cover -coverprofile=coverage.out && go tool cover -func=coverage.out && go tool cover --html=coverage.out -o coverage.html && firefox coverage.html

import (
	"fmt"
	"testing"

	"github.com/mattermost/mattermost-server/v5/plugin/plugintest/mock"
	"github.com/mattermost/mattermost-server/v5/store/sqlstore/mocks"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

const (
	KEY_LISTENER_SERVER            = "localhost"
	FAKE_K8S_SERVICE_ACCOUNT_TOKEN = "eyJhbGciOiJSUzI1NiIsImtpZCI6IlZ6dUtoUWxrYUkxX2cwcU44bDhoY1FaYkkxU2k0dUt2UTlhaVU1Q2Nxa2sifQ.eyJpc3MiOiJrdWJlcm5ldGVzL3NlcnZpY2VhY2NvdW50Iiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9uYW1lc3BhY2UiOiJ3b3JsZHIiLCJrdWJlcm5ldGVzLmlvL3NlcnZpY2VhY2NvdW50L3NlY3JldC5uYW1lIjoidmF1bHQtYXV0aC10b2tlbi13ODRuNCIsImt1YmVybmV0ZXMuaW8vc2VydmljZWFjY291bnQvc2VydmljZS1hY2NvdW50Lm5hbWUiOiJ2YXVsdC1hdXRoIiwia3ViZXJuZXRlcy5pby9zZXJ2aWNlYWNjb3VudC9zZXJ2aWNlLWFjY291bnQudWlkIjoiMjNjOGNlYWQtZGEyYi00ZDU2LWI4NjktZGY5ODIzOTRiMmIyIiwic3ViIjoic3lzdGVtOnNlcnZpY2VhY2NvdW50Ondvcmxkcjp2YXVsdC1hdXRoIn0.OFhEd2TmeEOrsB6iPlwuupIqoinym8rX9Q0EZ8stGT-acq4PGw2lTZIuq_bui8ZX870YS4_f2Jxo3F8Fcaq1641yQsx_M9X2NtHY1QnAD171X8RenxyKZhi58bWOCT5uCxzu8_1HBUQDb-FLHcS1lKB2lTkJ-KgJJEMPOTzOME-_wG5obejEjMt5CkiPodzoI7qPAgPTKsX6dRv9D2TYpm4DhgFQmDIIzX4smwzqm9cuDwrl2tUf2GzIpyZypTK9d8NBAks4cP1stW7ikQgOD7AukkNy8aR0iFNHAH8e-lBkPcbHShGZ6CS2Dx0rlBn0MaHDQRmVy-1p33rMH6D2ow"
	FAKE_VAULT_TOKEN               = "s.xzT4LmxwJItxrHdh5O595Wln"
	FAKE_VAULT_PG_TDE_KEY          = "923930EDA20758FF5E4AB11B5A4DE357"
	VAULT_TEST_URL                 = "http://unit.test.vault.worldr:8200"
	VAULT_UNSEAL_PATH              = "v1/sys/seal-status"
	VAULT_LOGIN_PATH               = "/v1/auth/kubernetes/login"
	VAULT_KV_SECRET_PATH           = "/v1/kv/pg-tde"
	VAULT_SEAL_STATUS_UNSEALED     = `{
	  "type": "shamir",
	  "initialise": true,
	  "sealed": false,
	  "t": 3,
	  "n": 5,
	  "progress": 0,
	  "nonce": "",
	  "version": "1.5.2",
	  "migration": false,
	  "cluster_name": "vault-cluster-8f6a9c63",
	  "cluster_id": "7ebd3e9e-1a7c-8102-341c-4c727fdf0e7f",
	  "recovery_seal": false,
	  "storage_type": "file"
	} `
	VAULT_SEAL_STATUS_SEALED = `{
	  "type": "shamir",
	  "initialise": true,
	  "sealed": true,
	  "t": 3,
	  "n": 5,
	  "progress": 0,
	  "nonce": "",
	  "version": "1.5.2",
	  "migration": false,
	  "cluster_name": "vault-cluster-8f6a9c63",
	  "cluster_id": "7ebd3e9e-1a7c-8102-341c-4c727fdf0e7f",
	  "recovery_seal": false,
	  "storage_type": "file"
	} `
	VAULT_LOGIN_RESPONSE = `{
	  "request_id": "b219d0a4-935c-61cb-fcd4-fe73ac923f38",
	  "lease_id": "",
	  "renewable": false,
	  "lease_duration": 0,
	  "data": null,
	  "wrap_info": null,
	  "warnings": null,
	  "auth": {
		"client_token": "s.xzT4LmxwJItxrHdh5O595Wln",
		"accessor": "bDvWwQezq8kGwh6Lua7xvoR0",
		"policies": [
		  "default",
		  "pg-tde-kv-ro"
		],
		"token_policies": [
		  "default",
		  "pg-tde-kv-ro"
		],
		"metadata": {
		  "role": "app-server",
		  "service_account_name": "vault-auth",
		  "service_account_namespace": "worldr",
		  "service_account_secret_name": "vault-auth-token-w84n4",
		  "service_account_uid": "23c8cead-da2b-4d56-b869-df982394b2b2"
		},
		"lease_duration": 86400,
		"renewable": true,
		"entity_id": "f8ec7abf-8723-e9d0-4ff3-cf82ec4308db",
		"token_type": "service",
		"orphan": true
	  }
	}`
	VAULT_KV_SECRET_RESPONSE = `{
	  "request_id": "439cd3c5-4fa9-75c2-8a6a-775fa698528a",
	  "lease_id": "",
	  "renewable": false,
	  "lease_duration": 2764800,
	  "data": {
		"TopSecretKey": "923930EDA20758FF5E4AB11B5A4DE357"
	  },
	  "wrap_info": null,
	  "warnings": null,
	  "auth": null
	}`
)

//    _____________________________
//___/ These are the UNHAPPY PATHS \____________________________________________
//
// All these tests go in order of possible failures.

func TestKeyTalkerFailOnAccountToken(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return("", nil)

	err := KeyTalker("localhost:8888", mockVault)
	assert.Nil(t, err)
}

func TestKeyTalkerFailOnReadK8ServiceAccountToken(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return("", errors.New("Unit test cannot read token"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

func TestKeyTalkerFailOnVaultSealed(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return(FAKE_K8S_SERVICE_ACCOUNT_TOKEN, nil)
	mockVault.On("WaitForVaultToUnseal",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
		mock.AnythingOfType("int")).Return(errors.New("Unit test cannot read token"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

func TestKeyTalkerFailOnLogin(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return(FAKE_K8S_SERVICE_ACCOUNT_TOKEN, nil)
	mockVault.On("WaitForVaultToUnseal",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
		mock.AnythingOfType("int")).Return(nil)
	mockVault.On("Login",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return("", errors.New("unit test cannot login"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

func TestKeyTalkerFailOnSecret(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return(FAKE_K8S_SERVICE_ACCOUNT_TOKEN, nil)
	mockVault.On("WaitForVaultToUnseal",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
		mock.AnythingOfType("int")).Return(nil)
	mockVault.On("Login",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(FAKE_VAULT_TOKEN, nil)
	mockVault.On("GetSecret",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return("", errors.New("unit test cannot get PG TDE key"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

func TestKeyTalkerFailOnKeySend(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return(FAKE_K8S_SERVICE_ACCOUNT_TOKEN, nil)
	mockVault.On("WaitForVaultToUnseal",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
		mock.AnythingOfType("int")).Return(nil)
	mockVault.On("Login",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(FAKE_VAULT_TOKEN, nil)
	mockVault.On("GetSecret",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(FAKE_VAULT_PG_TDE_KEY, nil)
	mockVault.On("SendKeyToListener",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(errors.New("unit test cannot send PG TDE key"))

	err := KeyTalker("localhost:8888", mockVault)
	assert.NotNil(t, err)
}

//    ________________________
//___/ This is the HAPPY PATH \_________________________________________________

func TestKeyTalkerHappyPath(t *testing.T) {
	mockVault := &mocks.IVault{}
	mockVault.On("Getk8sServiceAccountToken",
		mock.AnythingOfType("string")).Return(FAKE_K8S_SERVICE_ACCOUNT_TOKEN, nil)
	mockVault.On("WaitForVaultToUnseal",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("time.Duration"),
		mock.AnythingOfType("int")).Return(nil)
	mockVault.On("Login",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(FAKE_VAULT_TOKEN, nil)
	mockVault.On("GetSecret",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(FAKE_VAULT_PG_TDE_KEY, nil)
	mockVault.On("SendKeyToListener",
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string")).Return(nil)

	err := KeyTalker("localhost:8888", mockVault)
	assert.Nil(t, err)
}

//    ________________________________
//___/ These are helper methods/tests \_________________________________________
//

// K8s service account: no token file.
func TestGetk8sServiceAccountTokenNoTokenFile(t *testing.T) {
	vault := Vault{}
	token, err := vault.Getk8sServiceAccountToken("cthulhu/fhtagn")
	assert.Nil(t, err)
	assert.Equal(t, "", token)
}

// K8s service account: cannot open the k8s token file.
func TestGetk8sServiceAccountTokenCannotOpen(t *testing.T) {
	vault := Vault{}
	token, err := vault.Getk8sServiceAccountToken("/etc/shadow")
	assert.NotNil(t, err)
	fmt.Printf("ERROR: %s\n", err)
	assert.Equal(t, "", token)
}

// K8s service account: success.
func TestGetk8sServiceAccountTokenSuccess(t *testing.T) {
	vault := Vault{}
	token, err := vault.Getk8sServiceAccountToken("test_token.txt")
	assert.Nil(t, err)
	assert.Equal(t, "Fear the old blood", token)
}

// Wait for vault to unseal: happy path.
func TestWaitForVaultToUnseal(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(200).
		JSON(VAULT_SEAL_STATUS_UNSEALED)

	// Your test code starts here...
	vault := Vault{}
	err := vault.WaitForVaultToUnseal(VAULT_TEST_URL+"/"+VAULT_UNSEAL_PATH, 0, 1)
	assert.Nil(t, err)

	// Verify that we don't have pending mocks
	assert.Equal(t, true, gock.IsDone())
}

// Wait for vault to unseal: Got 404, we have to wait for a little while.
func TestWaitForVaultToUnseal404(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(404)
	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(200).
		JSON(VAULT_SEAL_STATUS_UNSEALED)

	// Your test code starts here...
	vault := Vault{}
	err := vault.WaitForVaultToUnseal(VAULT_TEST_URL+"/"+VAULT_UNSEAL_PATH, 0, 2)
	assert.Nil(t, err)
}

// Wait for vault to unseal: No body, we have to wait for a little while.
func TestWaitForVaultToUnsealNoBodyFailure(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(200)

	// Your test code starts here...
	vault := Vault{}
	err := vault.WaitForVaultToUnseal(VAULT_TEST_URL+"/"+VAULT_UNSEAL_PATH, 0, 1)
	assert.NotNil(t, err) // This HAS to fail.
}

// Wait for vault to unseal: reply error.
func TestWaitForVaultToUnsealReplyError(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		ReplyError(errors.New("Computer says NON!"))

	// Your test code starts here...
	vault := Vault{}
	err := vault.WaitForVaultToUnseal(VAULT_TEST_URL+"/bar", 0, 1)
	assert.NotNil(t, err) // This HAS to fail.
}

// Wait for vault to unseal: we have to wait for a little while.
func TestWaitForVaultToUnsealVaultIsSealed(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(200).
		JSON(VAULT_SEAL_STATUS_SEALED)
	gock.New(VAULT_TEST_URL).
		Get("v1/sys/seal-status").
		Reply(200).
		JSON(VAULT_SEAL_STATUS_UNSEALED)

	// Your test code starts here...
	vault := Vault{}
	err := vault.WaitForVaultToUnseal(VAULT_TEST_URL+"/"+VAULT_UNSEAL_PATH, 0, 2)
	assert.Nil(t, err)
}

// Vault login: happy path.
func TestLoginHappyPath(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Post(VAULT_LOGIN_PATH).
		MatchType("json").
		Reply(200).
		JSON(VAULT_LOGIN_RESPONSE)

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.Login(VAULT_TEST_URL+VAULT_LOGIN_PATH, "Fear the old blood")
	assert.Nil(t, err)
	assert.Equal(t, FAKE_VAULT_TOKEN, token)
}

// Vault login: not a 2XX status code.
func TestLoginStatusCode(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Post(VAULT_LOGIN_PATH).
		MatchType("json").
		Reply(500)

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.Login(VAULT_TEST_URL+VAULT_LOGIN_PATH, "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault login: Bad body.
func TestLoginBadBody(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Post(VAULT_LOGIN_PATH).
		MatchType("json").
		Reply(200)

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.Login(VAULT_TEST_URL+VAULT_LOGIN_PATH, "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault login: cannot POST.
func TestLoginBadPOST(t *testing.T) {
	vault := Vault{}
	token, err := vault.Login("https://[::1]:22"+VAULT_LOGIN_PATH, "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault get KV secret: Happy path
func TestGetVaultSecretHappyPath(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get(VAULT_KV_SECRET_PATH).
		Reply(200).
		JSON(VAULT_KV_SECRET_RESPONSE)

	// Your test code starts here…
	vault := Vault{}
	topSecretKey, err := vault.GetSecret(VAULT_TEST_URL+VAULT_KV_SECRET_PATH, "TopSecretKey", "Fear the old blood")
	assert.Nil(t, err)
	assert.Equal(t, FAKE_VAULT_PG_TDE_KEY, topSecretKey)

	// Verify that we don't have pending mocks
	assert.Equal(t, true, gock.IsDone())
}

// Vault get KV secret: request failed.
func TestGetSecretRequestFailed(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get(VAULT_KV_SECRET_PATH).
		MatchType("json")

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.GetSecret(VAULT_TEST_URL+VAULT_KV_SECRET_PATH, "TopSecretKey", "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault get KV secret: status code is not 2XX.
func TestGetSecretStatusCode(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get(VAULT_KV_SECRET_PATH).
		Reply(500)

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.GetSecret(VAULT_TEST_URL+VAULT_KV_SECRET_PATH, "TopSecretKey", "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault get KV secret: JSON is garbage.
func TestGetSecretJSONFailed(t *testing.T) {
	defer gock.Off() // Flush pending mocks after test execution

	gock.New(VAULT_TEST_URL).
		Get(VAULT_KV_SECRET_PATH).
		Reply(200).
		JSON("computer says NON")

	// Your test code starts here…
	vault := Vault{}
	token, err := vault.GetSecret(VAULT_TEST_URL+VAULT_KV_SECRET_PATH, "TopSecretKey", "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}

// Vault get KV secret: NewRequest is garbage.
func TestGetSecretNewRequestFailed(t *testing.T) {
	// Your test code starts here…
	vault := Vault{}
	token, err := vault.GetSecret("https://[::1]:22", "TopSecretKey", "Fear the old blood")
	assert.NotNil(t, err)
	assert.Equal(t, "", token)
}
