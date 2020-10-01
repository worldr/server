package sqlstore

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/pkg/errors"
)

const (
	VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	VAULT_WAIT_UNSEAL_ATTEMPTS           = 30
	VAULT_WAIT_UNSEAL_TIMEOUT_SECS       = 10
	VAULT_SERVER_DOMAIN                  = "http://vault.worldr:8200/"
	VAULT_SERVER_SEAL_STATUS_URL         = VAULT_SERVER_DOMAIN + "/v1/sys/seal-status"
	VAULT_SERVER_LOGIN_URL               = VAULT_SERVER_DOMAIN + "/v1/auth/kubernetes/login"
	VAULT_KV_SECRET_URL                  = VAULT_SERVER_DOMAIN + "/v1/kv/pg-tde"
	VAULT_SECRET_PG_TDE_KEY              = "TopSecretKey"
	VAULT_SECRET_PG_USERNAME             = "" // TODO: later.
	VAULT_SECRET_PG_PASSWORD             = "" // TODO: later.
)

type IVault interface {
	Getk8sServiceAccountToken(tokenFile string) (string, error)
	WaitForVaultToUnseal(url string, wait time.Duration, retry int) error
	Login(url string, token string) (string, error)
	GetSecret(url string, secret string, token string) (string, error)
	SendKeyToListener(service string, topSecretKey string) error
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

// Get the k8s service account token.
// There are three possible states:
//   1. There is no token file. Fine, use unencrypted database.
//   2. There is a token file but we cannot read it. This is bad and cannot happen.
//   3. There is a token file and we can read it. All is good, proceed.
func Getk8sServiceAccountToken(name string) (string, error) {

	_, err := os.Stat(name)
	if err != nil {
		return "", nil
	}

	file, err := os.Open(name)
	if err != nil {
		return "", errors.Wrap(err, "Cannot open token file")
	}
	defer file.Close()

	token, _ := ioutil.ReadAll(file)
	return string(token), nil
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
	if status_message.Sealed == true {
		// Dammit, we have to wait.
		return errors.New("Vault is still sealed, we cannot use it")
	}
	// All is good, continue.
	mlog.Info("Vault is unsealed. We can use it")
	return nil // This is the only success possible.
}

// Wait for the Vault to be unsealed.
// There is no point trying GET operations if the Vault is not unlocked.
func (v Vault) WaitForVaultToUnseal(url string, wait time.Duration, retry int) error {
	for i := 0; i < retry; i++ {
		err := getVaultUnsealStatus(url)
		if err != nil {
			mlog.Info(fmt.Sprintf("Waiting for Vault to unseal for another %d seconds because %s", wait, err.Error()))
			time.Sleep(wait * time.Second)
		} else {
			return nil
		}
	}
	return errors.New("Could not verify if Vault is unsealed")
}

// Login into Vault.
func (v Vault) Login(url string, token string) (string, error) {
	requestBody, _ := json.Marshal(map[string]string{
		"jwt":  token,
		"role": "app-server",
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		mlog.Warn(fmt.Sprintf("Cannot POST because %s", err))
		return "", errors.Wrap(err, "Cannot POST")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("Error code not 2XX but %d", resp.StatusCode)
		mlog.Warn(msg)
		return "", errors.New(msg)
	}

	message := new(VaultLogin)
	err = json.NewDecoder(resp.Body).Decode(message)
	if err != nil {
		mlog.Warn(fmt.Sprintf("Cannot unmarshal JSON because %s", err))
		return "", errors.Wrap(err, "Cannot read body")
	}

	return message.Auth.ClientToken, nil
}

// Get secret from Vault.
func (v Vault) GetSecret(url string, secret string, token string) (string, error) {
	// Get the seal status from the Vault.

	req, _ := http.NewRequest("GET", url, nil)
	// I cannot fathom how to make this fail. Is it even possible?
	//if err != nil {
	//	mlog.Warn(fmt.Sprintf("New request is invalid because %s", err))
	//	return "", errors.Wrap(err, "Bad new request")
	//}
	req.Header.Set("X-Vault-Token", os.ExpandEnv("$CLIENT_TOKEN"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		mlog.Warn(fmt.Sprintf("Cannot get response because %s", err))
		return "", errors.Wrap(err, "Cannot get KV secrets")
	}

	// Got a reply, check if vault is unsealed.
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("got return code %d, trying again", resp.StatusCode)
		mlog.Warn(msg)
		return "", errors.New(msg)
	}
	message := new(VaultKVSecret)
	err = json.NewDecoder(resp.Body).Decode(message)
	if err != nil {
		mlog.Warn(fmt.Sprintf("Cannot masrshal JSON because %s", err))
		return "", errors.Wrap(err, "cannot unmarshal JSON, trying again")
	}
	return message.Data.TopSecretKey, nil // This is the only success possible.
}

// Send key via secure socket.
func SendKeyToListener(service string, topSecretKey string) error {
	TLSClientConfig := &tls.Config{InsecureSkipVerify: true} // FIXME!

	conn, err := tls.Dial("tcp", service, TLSClientConfig)
	if err != nil {
		return errors.Wrap(err, "Cannot connect")
	}

	conn.Write([]byte(topSecretKey))

	var buf [1024]byte
	count, err := conn.Read(buf[0:])
	if err != nil {
		return errors.Wrap(err, "Cannot read")
	}

	mlog.Info("Key listener quoth '" + string(buf[0:count]) + "'")
	return nil
}

// A key talker provider.
// The service should be a "host:port" string.
//
// This need to happen:
//
//  1. Check if we are using a Vault server: Does the k8s auth service exits?
//  2. Read the k8s auth service account token.
//  3. Wait for the vault server to be unsealed.
//  4. Get an access token from the vault.
//  5. Finally, get the PG TDE password from Vault.
//  6. Send the password to the database so it can either unseal or initialise.
//
// Easy.
func KeyTalker(service string, vault IVault) error {
	// 1. Check if we are using a Vault server: Does the k8s auth service exits?
	// 2. Read the k8s auth service account token.
	tokenK8s, err := vault.Getk8sServiceAccountToken(VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE)
	if err != nil {
		mlog.Critical("Cannot read token", mlog.Err(err))
		return errors.Wrap(err, "There should be a k8s token but we cannot read it: Aborting")
	}
	if tokenK8s == "" {
		mlog.Warn("This does not need Vault, using unencrypted database.")
		return nil
	}

	// 3. Wait for the vault server to be unsealed.
	err = vault.WaitForVaultToUnseal(VAULT_SERVER_SEAL_STATUS_URL, VAULT_WAIT_UNSEAL_TIMEOUT_SECS, VAULT_WAIT_UNSEAL_TIMEOUT_SECS)
	if err != nil {
		mlog.Critical("Vault seal problem.")
		return errors.Wrap(err, "Vault seal problem")
	}

	//  4. Get an access token from the vault.
	tokenVault, err := vault.Login(VAULT_SERVER_LOGIN_URL, tokenK8s)
	if err != nil {
		mlog.Critical("Cannot login to Vault")
		return errors.Wrap(err, "Cannot login to Vault")
	}

	// 5. Finally, get the PG TDE password from Vault.
	topSecretKey, err := vault.GetSecret(VAULT_KV_SECRET_URL, VAULT_SECRET_PG_TDE_KEY, tokenVault)
	if err != nil {
		mlog.Critical("Cannot get PG TDE secret!")
		return errors.Wrap(err, "Cannot get PG TDE secret!")
	}
	mlog.Info(topSecretKey)

	// 6. Send the password to the database so it can either unseal or initialise.
	err = vault.SendKeyToListener(service, topSecretKey)
	if err != nil {
		mlog.Critical("Cannot send PG TDE secret!")
		return errors.Wrap(err, "Cannot send PG TDE secret!")
	}

	// This is a success!
	return nil
}
