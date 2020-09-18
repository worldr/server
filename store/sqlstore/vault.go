package sqlstore

import (
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
	VAULT_SERVER_LOGIN_URL               = VAULT_SERVER_DOMAIN + "v1/auth/kubernetes/login"
)

type IVault interface {
	Getk8sServiceAccountToken(tokenFile string) (string, error)
	WaitForVaultToUnseal(url string, wait time.Duration, retry int) error
	Login(url string) (string, error)
}

type Vault struct {
}

type VaultSealStatusMessage struct {
	Type          string `json:"type"`
	Initialized   bool   `json:"initialise"`
	Sealed        bool   `json:"sealed"`
	T             int64  `json:"t"`
	N             int64  `json:"n"`
	Progress      int64  `json:"progress"`
	Nonce         string `json:"nonce"`
	Version       string `json:"version"`
	Migration     bool   `json:"migration"`
	Cluster_name  string `json:"cluster_name"`
	Cluster_id    string `json:"cluster_id"`
	Recovery_seal bool   `json:"recovery_seal"`
	Storage_type  string `json:"storage_type"`
}

// Get the k8s service account token.
// There are three possible states:
//   1. There is no token file. Fine, use unencrypted database.
//   2. There is a token file but we cannot read it. This is bad. Abort.
//   3. There is a token file and we can read it. All is good, proceed.
func (v Vault) Getk8sServiceAccountToken(tokenFile string) (string, error) {
	if _, err := os.Stat(tokenFile); os.IsNotExist(err) {
		// File does not exist. No point in continuing this.
		mlog.Info("Cannot find k8s service account token.")
		return "", nil
	}
	return "", errors.New("Nope!")
	// File does exist.
	mlog.Info("Found a k8s service account token.")
	out, err := ioutil.ReadFile(VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE)
	if err != nil {
		mlog.Warn("Cannot read k8s service account token", mlog.Err(err))
		return "", err
	} else {
		return string(out), nil
	}
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
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "cannot read body, trying again")
	}
	var status_message VaultSealStatusMessage
	err = json.Unmarshal(body, &status_message)
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
func (v Vault) Login(url string) (string, error) {
	return "", nil
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
	tokenVault, err := vault.Login(VAULT_SERVER_LOGIN_URL)
	if err != nil {
		mlog.Critical("Cannot login to Vault")
		return errors.Wrap(err, "Cannot login to Vault")
	}
	mlog.Info(tokenVault)

	// This is a success!
	return nil
}
