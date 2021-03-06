// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-server/v5/config"
	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/mattermost/mattermost-server/v5/utils"
)

const (
	ERROR_TERMS_OF_SERVICE_NO_ROWS_FOUND = "store.sql_terms_of_service_store.get.no_rows.app_error"
)

func (s *Server) Config() *model.Config {
	return s.configStore.Get()
}

func (a *App) Config() *model.Config {
	return a.Srv().Config()
}

func (s *Server) EnvironmentConfig() map[string]interface{} {
	return s.configStore.GetEnvironmentOverrides()
}

func (a *App) EnvironmentConfig() map[string]interface{} {
	return a.Srv().EnvironmentConfig()
}

func (s *Server) UpdateConfig(f func(*model.Config)) {
	old := s.Config()
	updated := old.Clone()
	f(updated)
	if _, err := s.configStore.Set(updated); err != nil {
		mlog.Error("Failed to update config", mlog.Err(err))
	}
}

func (a *App) UpdateConfig(f func(*model.Config)) {
	a.Srv().UpdateConfig(f)
}

func (s *Server) ReloadConfig() error {
	debug.FreeOSMemory()
	if err := s.configStore.Load(); err != nil {
		return err
	}
	return nil
}

func (a *App) ReloadConfig() error {
	return a.Srv().ReloadConfig()
}

func (a *App) ClientConfig() map[string]string {
	return a.Srv().clientConfig
}

func (a *App) ClientConfigHash() string {
	return a.Srv().clientConfigHash
}

func (a *App) LimitedClientConfig() map[string]string {
	return a.Srv().limitedClientConfig
}

// Registers a function with a given listener to be called when the config is reloaded and may have changed. The function
// will be called with two arguments: the old config and the new config. AddConfigListener returns a unique ID
// for the listener that can later be used to remove it.
func (s *Server) AddConfigListener(listener func(*model.Config, *model.Config)) string {
	return s.configStore.AddListener(listener)
}

func (a *App) AddConfigListener(listener func(*model.Config, *model.Config)) string {
	return a.Srv().AddConfigListener(listener)
}

// Removes a listener function by the unique ID returned when AddConfigListener was called
func (s *Server) RemoveConfigListener(id string) {
	s.configStore.RemoveListener(id)
}

func (a *App) RemoveConfigListener(id string) {
	a.Srv().RemoveConfigListener(id)
}

// ensurePostActionCookieSecret ensures that the key for encrypting PostActionCookie exists
// and future calls to PostActionCookieSecret will always return a valid key, same on all
// servers in the cluster
func (a *App) ensurePostActionCookieSecret() error {
	if a.Srv().postActionCookieSecret != nil {
		return nil
	}

	var secret *model.SystemPostActionCookieSecret

	value, err := a.Srv().Store.System().GetByName(model.SYSTEM_POST_ACTION_COOKIE_SECRET)
	if err == nil {
		if err := json.Unmarshal([]byte(value.Value), &secret); err != nil {
			return err
		}
	}

	// If we don't already have a key, try to generate one.
	if secret == nil {
		newSecret := &model.SystemPostActionCookieSecret{
			Secret: make([]byte, 32),
		}
		_, err := rand.Reader.Read(newSecret.Secret)
		if err != nil {
			return err
		}

		system := &model.System{
			Name: model.SYSTEM_POST_ACTION_COOKIE_SECRET,
		}
		v, err := json.Marshal(newSecret)
		if err != nil {
			return err
		}
		system.Value = string(v)
		// If we were able to save the key, use it, otherwise log the error.
		if appErr := a.Srv().Store.System().Save(system); appErr != nil {
			mlog.Error("Failed to save PostActionCookieSecret", mlog.Err(appErr))
		} else {
			secret = newSecret
		}
	}

	// If we weren't able to save a new key above, another server must have beat us to it. Get the
	// key from the database, and if that fails, error out.
	if secret == nil {
		value, err := a.Srv().Store.System().GetByName(model.SYSTEM_POST_ACTION_COOKIE_SECRET)
		if err != nil {
			return err
		}

		if err := json.Unmarshal([]byte(value.Value), &secret); err != nil {
			return err
		}
	}

	a.Srv().postActionCookieSecret = secret.Secret
	return nil
}

func GenerateSigningKey(fieldName string) (*model.System, *model.SystemAsymmetricSigningKey, error) {
	newECDSAKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	newKey := &model.SystemAsymmetricSigningKey{
		ECDSAKey: &model.SystemECDSAKey{
			Curve: "P-256",
			X:     newECDSAKey.X,
			Y:     newECDSAKey.Y,
			D:     newECDSAKey.D,
		},
	}
	system := &model.System{
		Name: fieldName,
	}
	v, err := json.Marshal(newKey)
	if err != nil {
		return nil, nil, err
	}
	system.Value = string(v)
	return system, newKey, nil
}

// GetCertSigningKey reads the certificate pinning Ed25519 key from the database,
// generates one if none exists.
func GetCertSigningKey(s store.SystemStore) (*model.SystemEd25519Key, *model.AppError) {
	var key *model.SystemEd25519Key
	value, err := s.GetByName(model.SYSTEM_CERTIFICATE_SIGNING_KEY)
	if err == nil {
		if err := json.Unmarshal([]byte(value.Value), &key); err != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.db", nil, err.Error(), http.StatusInternalServerError)
		}
	}
	if key == nil {
		pk, sk, keyErr := ed25519.GenerateKey(nil)
		if keyErr != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.generate", nil, keyErr.Error(), http.StatusInternalServerError)
		}
		key = &model.SystemEd25519Key{
			Public:  hex.EncodeToString(pk),
			Secret:  hex.EncodeToString(sk),
			Version: 1,
		}
		system := &model.System{
			Name: model.SYSTEM_CERTIFICATE_SIGNING_KEY,
		}
		v, keyErr := json.Marshal(key)
		if keyErr != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.serialise", nil, keyErr.Error(), http.StatusInternalServerError)
		}
		system.Value = string(v)

		// If we were able to save the key, use it, otherwise respond with error.
		if appErr := s.Save(system); appErr != nil {
			return nil, appErr
		}
	}
	return key, nil
}

// EnsureAsymmetricSigningKey ensures that an asymmetric signing key exists and future calls to
// AsymmetricSigningKey will always return a valid signing key.
func (a *App) ensureAsymmetricSigningKey() error {
	if a.Srv().asymmetricSigningKey != nil {
		return nil
	}

	var key *model.SystemAsymmetricSigningKey

	value, err := a.Srv().Store.System().GetByName(model.SYSTEM_ASYMMETRIC_SIGNING_KEY)
	if err == nil {
		if err := json.Unmarshal([]byte(value.Value), &key); err != nil {
			return err
		}
	}

	// If we don't already have a key, try to generate one.
	if key == nil {
		system, newKey, err := GenerateSigningKey(model.SYSTEM_ASYMMETRIC_SIGNING_KEY)
		if err != nil {
			return err
		}
		// If we were able to save the key, use it, otherwise log the error.
		if appErr := a.Srv().Store.System().Save(system); appErr != nil {
			mlog.Error("Failed to save AsymmetricSigningKey", mlog.Err(appErr))
		} else {
			key = newKey
		}
	}

	// If we weren't able to save a new key above, another server must have beat us to it. Get the
	// key from the database, and if that fails, error out.
	if key == nil {
		value, err := a.Srv().Store.System().GetByName(model.SYSTEM_ASYMMETRIC_SIGNING_KEY)
		if err != nil {
			return err
		}

		if err := json.Unmarshal([]byte(value.Value), &key); err != nil {
			return err
		}
	}

	var curve elliptic.Curve
	switch key.ECDSAKey.Curve {
	case "P-256":
		curve = elliptic.P256()
	default:
		return fmt.Errorf("unknown curve: " + key.ECDSAKey.Curve)
	}
	a.Srv().asymmetricSigningKey = &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: curve,
			X:     key.ECDSAKey.X,
			Y:     key.ECDSAKey.Y,
		},
		D: key.ECDSAKey.D,
	}
	a.regenerateClientConfig()
	return nil
}

func (a *App) ensureInstallationDate() error {
	_, err := a.getSystemInstallDate()
	if err == nil {
		return nil
	}

	installDate, err := a.Srv().Store.User().InferSystemInstallDate()
	var installationDate int64
	if err == nil && installDate > 0 {
		installationDate = installDate
	} else {
		installationDate = utils.MillisFromTime(time.Now())
	}

	err = a.Srv().Store.System().SaveOrUpdate(&model.System{
		Name:  model.SYSTEM_INSTALLATION_DATE_KEY,
		Value: strconv.FormatInt(installationDate, 10),
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) CompanyConfig() (*model.CompanyConfig, *model.AppError) {
	if s.companyConfig == nil {
		path := s.Config().ServiceSettings.CompanyConfig
		if path == nil || len(*path) == 0 {
			return nil, model.NewAppError("CompanyConfig", "company_config.undefined", nil, "company configuration is not defined on the server", http.StatusNotImplemented)
		}

		var config *model.CompanyConfig
		f, err := os.Open(*path)
		if err != nil {
			if os.IsNotExist(err) {
				mlog.Warn("Company config file does not exist, creating a default one", mlog.String("path", *path))
				config = &model.CompanyConfig{}
				config.SetDefaults()
				var data []byte
				data, err = json.MarshalIndent(config, "", "	")
				if err == nil {
					err = ioutil.WriteFile(*path, data, 0660)
				}
			} else {
				mlog.Error("Failed to open company config file", mlog.String("path", *path), mlog.Err(err))
				return nil, model.NewAppError("CompanyConfig", "company_config.open", nil, err.Error(), http.StatusInternalServerError)
			}
		} else {
			config, err = model.CompanyConfigFromJson(f)
		}

		if err != nil {
			mlog.Error("Failed to read or parse company config file", mlog.String("path", *path), mlog.Err(err))
			return nil, model.NewAppError("CompanyConfig", "company_config.parse", nil, err.Error(), http.StatusInternalServerError)
		}

		mlog.Info("Company configuration", mlog.String("json", config.ToJson()), mlog.String("path", *path))
		s.companyConfig = config

		defer func() {
			closeErr := f.Close()
			if err == nil && closeErr != nil {
				err = errors.Wrapf(closeErr, "failed to close %s", *path)
			}
		}()
	}
	return s.companyConfig, nil
}

func SetDefaultCompanyConfig(srv *Server, app *App, tempWorkspace string) {
	srv.SetDefaultCompanyConfig()
	app.UpdateConfig(func(cfg *model.Config) {
		if tempWorkspace == "" {
			tmp, err := ioutil.TempDir("", "apptest")
			if err != nil {
				panic(err)
			}
			tempWorkspace = tmp
		}
		cfg.ServiceSettings.CompanyConfig = model.NewString(filepath.Join(tempWorkspace, "company.cfg"))
	})
}

func (s *Server) SetDefaultCompanyConfig() {
	if s.companyConfig == nil {
		s.companyConfig = &model.CompanyConfig{}
		s.companyConfig.SetDefaults()
	}
}

func (s *Server) ResetCompanyConfig() {
	s.companyConfig = nil
}

func (s *Server) CertSignature() (*model.VersionedValue, *model.AppError) {
	if s.certSignature == nil {
		key, err := s.CertSigningKey()
		if err != nil {
			return nil, err
		}
		cert, fileErr := ioutil.ReadFile(*s.Config().ServiceSettings.TLSCertFile)
		if fileErr != nil {
			return nil, model.NewAppError("CertSignature", "cert_signature_create", nil, fileErr.Error(), http.StatusInternalServerError)
		}
		sk, hexErr := hex.DecodeString(key.Secret)
		if hexErr != nil {
			return nil, model.NewAppError("CertSignature", "cert_signature_decode_sk", nil, hexErr.Error(), http.StatusInternalServerError)
		}
		signature := ed25519.Sign(sk, cert)
		s.certSignature = &model.VersionedValue{
			Version:   strconv.Itoa(key.Version),
			Signature: hex.EncodeToString(signature),
		}
	}
	return s.certSignature, nil
}

func (s *Server) CertSigningKey() (*model.SystemEd25519Key, *model.AppError) {
	if s.certSigningKey == nil {
		key, err := GetCertSigningKey(s.Store.System())
		if err != nil {
			return nil, err
		}
		s.certSigningKey = key
	}
	return s.certSigningKey, nil
}

// AsymmetricSigningKey will return a private key that can be used for asymmetric signing.
func (s *Server) AsymmetricSigningKey() *ecdsa.PrivateKey {
	return s.asymmetricSigningKey
}

func (a *App) AsymmetricSigningKey() *ecdsa.PrivateKey {
	return a.Srv().AsymmetricSigningKey()
}

func (s *Server) PostActionCookieSecret() []byte {
	return s.postActionCookieSecret
}

func (a *App) PostActionCookieSecret() []byte {
	return a.Srv().PostActionCookieSecret()
}

func (a *App) regenerateClientConfig() {
	clientConfig := config.GenerateClientConfig(a.Config(), a.DiagnosticId(), a.License())
	limitedClientConfig := config.GenerateLimitedClientConfig(a.Config(), a.DiagnosticId(), a.License())

	if clientConfig["EnableCustomTermsOfService"] == "true" {
		termsOfService, err := a.GetLatestTermsOfService()
		if err != nil {
			mlog.Err(err)
		} else {
			clientConfig["CustomTermsOfServiceId"] = termsOfService.Id
			limitedClientConfig["CustomTermsOfServiceId"] = termsOfService.Id
		}
	}

	if key := a.AsymmetricSigningKey(); key != nil {
		der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
		clientConfig["AsymmetricSigningPublicKey"] = base64.StdEncoding.EncodeToString(der)
		limitedClientConfig["AsymmetricSigningPublicKey"] = base64.StdEncoding.EncodeToString(der)
	}

	clientConfigJSON, _ := json.Marshal(clientConfig)
	a.Srv().clientConfig = clientConfig
	a.Srv().limitedClientConfig = limitedClientConfig
	a.Srv().clientConfigHash = fmt.Sprintf("%x", md5.Sum(clientConfigJSON))
}

func (a *App) GetCookieDomain() string {
	if *a.Config().ServiceSettings.AllowCookiesForSubdomains {
		if siteURL, err := url.Parse(*a.Config().ServiceSettings.SiteURL); err == nil {
			return siteURL.Hostname()
		}
	}
	return ""
}

func (a *App) GetSiteURL() string {
	return *a.Config().ServiceSettings.SiteURL
}

// ClientConfigWithComputed gets the configuration in a format suitable for sending to the client.
func (a *App) ClientConfigWithComputed() map[string]string {
	respCfg := map[string]string{}
	for k, v := range a.ClientConfig() {
		respCfg[k] = v
	}

	// These properties are not configurable, but nevertheless represent configuration expected
	// by the client.
	respCfg["NoAccounts"] = strconv.FormatBool(a.IsFirstUserAccount())
	respCfg["MaxPostSize"] = strconv.Itoa(a.MaxPostSize())
	respCfg["InstallationDate"] = ""
	if installationDate, err := a.getSystemInstallDate(); err == nil {
		respCfg["InstallationDate"] = strconv.FormatInt(installationDate, 10)
	}

	return respCfg
}

// LimitedClientConfigWithComputed gets the configuration in a format suitable for sending to the client.
func (a *App) LimitedClientConfigWithComputed() map[string]string {
	respCfg := map[string]string{}
	for k, v := range a.LimitedClientConfig() {
		respCfg[k] = v
	}

	// These properties are not configurable, but nevertheless represent configuration expected
	// by the client.
	respCfg["NoAccounts"] = strconv.FormatBool(a.IsFirstUserAccount())

	return respCfg
}

// GetConfigFile proxies access to the given configuration file to the underlying config store.
func (a *App) GetConfigFile(name string) ([]byte, error) {
	data, err := a.Srv().configStore.GetFile(name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get config file %s", name)
	}

	return data, nil
}

// GetSanitizedConfig gets the configuration for a system admin without any secrets.
func (a *App) GetSanitizedConfig() *model.Config {
	cfg := a.Config().Clone()
	cfg.Sanitize()

	return cfg
}

// GetEnvironmentConfig returns a map of configuration keys whose values have been overridden by an environment variable.
func (a *App) GetEnvironmentConfig() map[string]interface{} {
	return a.EnvironmentConfig()
}

// SaveConfig replaces the active configuration, optionally notifying cluster peers.
func (a *App) SaveConfig(newCfg *model.Config, sendConfigChangeClusterMessage bool) *model.AppError {
	oldCfg, err := a.Srv().configStore.Set(newCfg)
	if errors.Cause(err) == config.ErrReadOnlyConfiguration {
		return model.NewAppError("saveConfig", "ent.cluster.save_config.error", nil, err.Error(), http.StatusForbidden)
	} else if err != nil {
		return model.NewAppError("saveConfig", "app.save_config.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	if a.Metrics() != nil {
		if *a.Config().MetricsSettings.Enable {
			a.Metrics().StartServer()
		} else {
			a.Metrics().StopServer()
		}
	}

	if a.Cluster() != nil {
		newCfg = a.Srv().configStore.RemoveEnvironmentOverrides(newCfg)
		oldCfg = a.Srv().configStore.RemoveEnvironmentOverrides(oldCfg)
		err := a.Cluster().ConfigChanged(oldCfg, newCfg, sendConfigChangeClusterMessage)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *App) IsESIndexingEnabled() bool {
	return a.Elasticsearch() != nil && *a.Config().ElasticsearchSettings.EnableIndexing
}

func (a *App) IsESSearchEnabled() bool {
	esInterface := a.Elasticsearch()
	license := a.License()
	return esInterface != nil && *a.Config().ElasticsearchSettings.EnableSearching && license != nil && *license.Features.Elasticsearch
}

func (a *App) IsESAutocompletionEnabled() bool {
	esInterface := a.Elasticsearch()
	license := a.License()
	return esInterface != nil && *a.Config().ElasticsearchSettings.EnableAutocomplete && license != nil && *license.Features.Elasticsearch
}

func (a *App) HandleMessageExportConfig(cfg *model.Config, appCfg *model.Config) {
	// If the Message Export feature has been toggled in the System Console, rewrite the ExportFromTimestamp field to an
	// appropriate value. The rewriting occurs here to ensure it doesn't affect values written to the config file
	// directly and not through the System Console UI.
	if *cfg.MessageExportSettings.EnableExport != *appCfg.MessageExportSettings.EnableExport {
		if *cfg.MessageExportSettings.EnableExport && *cfg.MessageExportSettings.ExportFromTimestamp == int64(0) {
			// When the feature is toggled on, use the current timestamp as the start time for future exports.
			cfg.MessageExportSettings.ExportFromTimestamp = model.NewInt64(model.GetMillis())
		} else if !*cfg.MessageExportSettings.EnableExport {
			// When the feature is disabled, reset the timestamp so that the timestamp will be set if
			// the feature is re-enabled from the System Console in future.
			cfg.MessageExportSettings.ExportFromTimestamp = model.NewInt64(0)
		}
	}
}
