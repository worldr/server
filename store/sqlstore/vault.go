package sqlstore

import (
	"io/ioutil"
	"os"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/pkg/errors"
)

const (
	VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE = "/var/run/secrets/kubernetes.io/serviceaccount/token"
)

type IVault interface {
	Getk8sServiceAccountToken() (string, error)
}

type Vault struct {
}

func (v Vault) Getk8sServiceAccountToken() (string, error) {
	if _, err := os.Stat(VAULT_K8S_SERVICE_ACCOUNT_TOKEN_FILE); os.IsNotExist(err) {
		// File does not exist. No point in continuing this.
		mlog.Info("Cannot find k8s service account token.")
		return "", nil
	} else {
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
}

// A key talker provider.
// The service should be a "host:port" string.
//
// This need to happen:
//
//  1. Check if we are using a Vault server: Does the k8s auth service exits?
//  2. Read the k8s auth service account token.
//  3. Wait for the vault server to be unsealed.
//  4. Get an access token from the above.
//  5. Finally, get the PG TDE password from Vault.
//  6. Send the password to the database so it can either unseal or initialise.
//
// Easy.
func KeyTalker(service string, vault IVault) error {
	// 1. Check if we are using a Vault server: Does the k8s auth service exits?
	// 2. Read the k8s auth service account token.
	token, err := vault.Getk8sServiceAccountToken()
	if err != nil {
		mlog.Critical("Cannot read token", mlog.Err(err))
		return errors.Wrap(err, "There should be a k8s token but we cannot read it: Aborting")
	}
	mlog.Info(token)

	// 2. Wait for the vault server to be unsealed.
	//vault.wait

	// This is a success!
	return nil
}
