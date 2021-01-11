package sqlstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/utils"
	"github.com/pkg/errors"
)

const (
	VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	VAULT_WAIT_UNSEAL_ATTEMPTS           = 30
	VAULT_WAIT_UNSEAL_TIMEOUT_SECS       = 10
	VAULT_SERVER_DOMAIN                  = "http://vault.worldr:8200"
	VAULT_SERVER_SEAL_STATUS_URL         = VAULT_SERVER_DOMAIN + "/v1/sys/seal-status"
	VAULT_SERVER_LOGIN_URL               = VAULT_SERVER_DOMAIN + "/v1/auth/kubernetes/login"
	VAULT_KV_SECRET_URL                  = VAULT_SERVER_DOMAIN + "/v1/kv/pg-tde"
	VAULT_SECRET_PG_TDE_KEY              = "TopSecretKey"
	VAULT_SECRET_PG_USERNAME             = "" // TODO: later.
	VAULT_SECRET_PG_PASSWORD             = "" // TODO: later.
	VAULT_UNSEAL_SERVICE_URL             = "http://unseal-service.worldr:8077"
	VAULT_UNSEAL_SERVICE_API             = VAULT_UNSEAL_SERVICE_URL + "/api/v1/unseal"
)

type vaultUnsealAPIJSONresponse struct {
	Memorandum        string   `json:"memorandum"`
	UnsealAPIStatuses []string `json:"unseal_API_statuses"`
}

type IVault interface {
	GetSecret(url string, secret string, token string) (string, error)
	Getk8sServiceAccountToken(tokenFile string) (*string, error)
	Login(url string, token *string) (*string, error)
	WaitForVaultToUnseal(url string, wait time.Duration, retry int) error
}

type Vault struct {
}

type VaultSealStatusMessage struct {
	Type         string `json:"type"`
	Initialized  bool   `json:"initialized"`
	Sealed       bool   `json:"sealed"`
	T            int    `json:"t"`
	N            int    `json:"n"`
	Progress     int    `json:"progress"`
	Nonce        string `json:"nonce"`
	Version      string `json:"version"`
	Migration    bool   `json:"migration"`
	ClusterName  string `json:"cluster_name"`
	ClusterID    string `json:"cluster_id"`
	RecoverySeal bool   `json:"recovery_seal"`
	StorageType  string `json:"storage_type"`
}

type VaultLogin struct {
	RequestID     string      `json:"request_id"`
	LeaseID       string      `json:"lease_id"`
	Renewable     bool        `json:"renewable"`
	LeaseDuration int         `json:"lease_duration"`
	Data          interface{} `json:"data"`
	WrapInfo      interface{} `json:"wrap_info"`
	Warnings      interface{} `json:"warnings"`
	Auth          struct {
		ClientToken   string   `json:"client_token"`
		Accessor      string   `json:"accessor"`
		Policies      []string `json:"policies"`
		TokenPolicies []string `json:"token_policies"`
		Metadata      struct {
			Role                     string `json:"role"`
			ServiceAccountName       string `json:"service_account_name"`
			ServiceAccountNamespace  string `json:"service_account_namespace"`
			ServiceAccountSecretName string `json:"service_account_secret_name"`
			ServiceAccountUID        string `json:"service_account_uid"`
		} `json:"metadata"`
		LeaseDuration int    `json:"lease_duration"`
		Renewable     bool   `json:"renewable"`
		EntityID      string `json:"entity_id"`
		TokenType     string `json:"token_type"`
		Orphan        bool   `json:"orphan"`
	} `json:"auth"`
}

type VaultKVSecret struct {
	RequestID     string `json:"request_id"`
	LeaseID       string `json:"lease_id"`
	Renewable     bool   `json:"renewable"`
	LeaseDuration int    `json:"lease_duration"`
	Data          struct {
		TopSecretKey string `json:"TopSecretKey"`
	} `json:"data"`
	WrapInfo interface{} `json:"wrap_info"`
	Warnings interface{} `json:"warnings"`
	Auth     interface{} `json:"auth"`
}

// Getk8sServiceAccountToken gets the k8s service account token.
// There are three possible states:
//   1. There is no token file. Fine, use unencrypted database.
//   2. There is a token file but we cannot read it. This is bad and cannot happen.
//   3. There is a token file and we can read it. All is good, proceed.
func (*Vault) Getk8sServiceAccountToken(name string) (*string, error) {
	token, err := utils.ReadTextFile(name)
	if err != nil {
		mlog.Warn("Cannot read k8s token file data", mlog.String("k8s token file", name))
	}
	return token, err
}

func getVaultUnsealStatus(url string) error {
	// Get the seal status from the Vault.
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "Could not check if Vault is sealed or not")
	}
	// Got a reply, check if vault is unsealed.
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("got return code %d, trying again", resp.StatusCode))
	}
	status_message := new(VaultSealStatusMessage)
	err = json.NewDecoder(resp.Body).Decode(status_message)
	if err != nil {
		return errors.Wrap(err, "cannot unmarshal JSON, trying again")
	}
	if status_message.Sealed {
		// Dammit, we have to wait.
		return errors.New("Vault is still sealed, we cannot use it")
	}
	// All is good, continue.
	mlog.Info("Vault is unsealed. We can use it")
	return nil // This is the only success possible.
}

func unsealVaultServicePOST(url string) error {
	requestBody, _ := json.Marshal(map[string]string{})
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		mlog.Warn("Cannot POST", mlog.Err(err))
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("Error code not 2XX but %d", resp.StatusCode)
		mlog.Warn(msg, mlog.Err(err))
		return errors.New(msg)
	}
	data := new(vaultUnsealAPIJSONresponse)
	err = json.NewDecoder(resp.Body).Decode(data)
	if err != nil {
		mlog.Warn("Cannot unmarshal JSON", mlog.Err(err))
		return err
	}
	mlog.Info("Unseal service says ", mlog.String("memorandum", data.Memorandum))
	for _, line := range data.UnsealAPIStatuses {
		mlog.Info("Unseal service status", mlog.String("status", line))
	}
	return nil
}

// Wait for the Vault to be unsealed.
// There is no point trying GET operations if the Vault is not unlocked.
func (*Vault) WaitForVaultToUnseal(url string, wait time.Duration, retry int) error {
	for i := 0; i < retry; i++ {
		err := getVaultUnsealStatus(url)
		if err != nil {
			mlog.Info("Waiting for Vault to unseal for another ", mlog.Err(err))
			err := unsealVaultServicePOST(VAULT_UNSEAL_SERVICE_API)
			if err != nil {
				mlog.Info("Could not auto-unseal Vault", mlog.Err(err))
			}
			time.Sleep(wait * time.Second)
		} else {
			return nil
		}
	}
	return errors.New("Could not verify if Vault is unsealed")
}

// Login into Vault.
func (*Vault) Login(url string, token *string) (*string, error) {
	requestBody, _ := json.Marshal(map[string]string{
		"jwt":  *token,
		"role": "app-server",
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		mlog.Warn("Cannot POST", mlog.Err(err))
		return nil, errors.Wrap(err, "Cannot POST")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("Error code not 2XX but %d", resp.StatusCode)
		mlog.Warn(msg)
		return nil, errors.New(msg)
	}

	message := new(VaultLogin)
	err = json.NewDecoder(resp.Body).Decode(message)
	if err != nil {
		mlog.Warn("Cannot unmarshal JSON", mlog.Err(err))
		return nil, errors.Wrap(err, "Cannot read body")
	}

	mlog.Info("Vault login was successful")
	return &message.Auth.ClientToken, nil
}

// Get secret from Vault.
func (*Vault) GetSecret(url string, secret string, token string) (string, error) {
	// Get the seal status from the Vault.

	req, _ := http.NewRequest("GET", url, nil)
	// I cannot fathom how to make this fail. Is it even possible?
	//if err != nil {
	//	mlog.Warn("New request is invalid", mlog.Error(err))
	//	return "", errors.Wrap(err, "Bad new request")
	//}
	req.Header.Set("X-Vault-Token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		mlog.Warn("Cannot get response", mlog.Err(err))
		return "", errors.Wrap(err, "Cannot get KV secrets")
	}

	// Got a reply, check if vault is unsealed.
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("%s returned code %d, trying again", url, resp.StatusCode)
		mlog.Warn(msg)
		return "", errors.New(msg)
	}
	message := new(VaultKVSecret)
	err = json.NewDecoder(resp.Body).Decode(message)
	if err != nil {
		mlog.Warn("Cannot masrshal JSON", mlog.Err(err))
		return "", errors.Wrap(err, "cannot unmarshal JSON, trying again")
	}
	return message.Data.TopSecretKey, nil // This is the only success possible.
}

// AuthoriseAndGet gets a value for a given key from Vault.
//
// This need to happen:
//
//  1. Check whether the k8s auth service exists, if not–report an error.
//  2. Read the k8s auth service account token.
//  3. Wait for the vault server to be unsealed.
//  4. Get an access token from the vault.
//  5. Finally, get the value from Vault.
func AuthoriseAndGet(vault IVault, url, key string) (*string, error) {
	//  1. Check whether the k8s auth service exists, if not–report an error.
	//  2. Read the k8s auth service account token.
	tokenK8s, err := vault.Getk8sServiceAccountToken(VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE)
	if err != nil || tokenK8s == nil || len(*tokenK8s) == 0 {
		mlog.Critical("K8s token is unavailable or empty", mlog.Err(err))
		return nil, errors.Wrap(err, "There should be a non-empty k8s token when the server is configured to use Vault!")
	}

	// 3. Wait for the vault server to be unsealed.
	err = vault.WaitForVaultToUnseal(VAULT_SERVER_SEAL_STATUS_URL, VAULT_WAIT_UNSEAL_TIMEOUT_SECS, VAULT_WAIT_UNSEAL_TIMEOUT_SECS)
	if err != nil {
		mlog.Critical("Vault seal problem.")
		return nil, errors.Wrap(err, "Vault seal problem")
	}

	//  4. Get an access token from the vault.
	tokenVault, err := vault.Login(VAULT_SERVER_LOGIN_URL, tokenK8s)
	if err != nil {
		mlog.Critical("Cannot login to Vault")
		return nil, errors.Wrap(err, "Cannot login to Vault")
	}

	// 5. Finally, get the value for the key from Vault.
	topSecretValue, err := vault.GetSecret(url, key, *tokenVault)
	if err != nil {
		mlog.Critical("Cannot get PG TDE secret!")
		return nil, errors.Wrap(err, "Cannot get PG TDE secret!")
	}

	// This is a success!
	return &topSecretValue, nil
}
