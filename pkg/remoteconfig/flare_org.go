// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"errors"
	"expvar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const (
	// orgBucket is the bucket of the Remote Configuration database where the
	// uptane client stores the org UUID it fetched from the org data
	// endpoint, under the key <root version>-uuid.
	orgBucket    = "org"
	orgUUIDKey   = "-uuid"
	dbOpenWait   = time.Second
	statusExpvar = "remoteConfigStatus"
)

// datadogConfigPath matches datadog/<org_id>/<product>/<config_id>/<file>,
// as the Remote Configuration state package parses it.
var datadogConfigPath = regexp.MustCompile(`^datadog/(\d+)/[^/]+/[^/]+/[^/]+$`)

// FlareOrg is what the Remote Configuration client knows of the Datadog org
// it's connected to, as written to Operator flares. It holds no keys.
type FlareOrg struct {
	Started bool
	Site    string
	// OrgUUID is "" with OrgUUIDError saying why when it's unknown.
	OrgUUID      string
	OrgUUIDError string
	// OrgEnabled and APIKeyAuthorized are "true", "false", or "" if the
	// service read no org status yet.
	OrgEnabled       string
	APIKeyAuthorized string
}

// FlareOrg returns what the Remote Configuration client knows of the org.
// The org UUID is read from a copy of the client database, like the Agent
// flare copies it: the service keeps the database open and locked.
func (r *RemoteConfigUpdater) FlareOrg() FlareOrg {
	org := FlareOrg{}
	r.clientMu.RLock()
	org.Started = r.rcClient != nil
	if r.serviceConf.cfg != nil {
		org.Site = r.serviceConf.cfg.GetString("site")
	}
	databasePath := r.rcDatabasePath
	r.clientMu.RUnlock()

	switch {
	case !org.Started:
		org.OrgUUIDError = "the Remote Configuration client is not started"
	case databasePath == "":
		org.OrgUUIDError = "the Remote Configuration database is unknown"
	default:
		uuid, err := storedOrgUUID(databasePath)
		if err != nil {
			org.OrgUUIDError = err.Error()
		}
		org.OrgUUID = uuid
	}
	org.OrgEnabled, org.APIKeyAuthorized = remoteConfigOrgStatus()
	return org
}

// storedOrgUUID returns the org UUID stored in a Remote Configuration
// database: the one of the latest root version. It reads a copy of the
// database, so it doesn't wait for the lock of the service that has it open.
func storedOrgUUID(databasePath string) (uuid string, err error) {
	// The copy can be torn by a concurrent write, and bbolt panics on some
	// corrupt pages.
	defer func() {
		if p := recover(); p != nil {
			uuid, err = "", fmt.Errorf("unable to read the copy of the Remote Configuration database: %v", p)
		}
	}()
	copyPath, err := copyToTemp(databasePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errors.New("the Remote Configuration database doesn't exist yet")
		}
		return "", fmt.Errorf("unable to copy the Remote Configuration database: %w", err)
	}
	defer os.Remove(copyPath)

	db, err := bbolt.Open(copyPath, 0o400, &bbolt.Options{ReadOnly: true, Timeout: dbOpenWait})
	if err != nil {
		return "", fmt.Errorf("unable to open the copy of the Remote Configuration database: %w", err)
	}
	defer db.Close()

	var latest uint64
	found := false
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(orgBucket))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			version, ok := strings.CutSuffix(string(k), orgUUIDKey)
			if !ok || len(v) == 0 {
				return nil
			}
			// Keys that aren't <root version>-uuid are skipped.
			if n, parseErr := strconv.ParseUint(version, 10, 64); parseErr == nil && (!found || n > latest) {
				found, latest, uuid = true, n, string(v)
			}
			return nil
		})
	})
	if err != nil {
		return "", fmt.Errorf("unable to read the Remote Configuration database: %w", err)
	}
	if !found {
		return "", errors.New("the Remote Configuration database holds no org UUID yet: the client stores it after its first successful update")
	}
	return uuid, nil
}

func copyToTemp(path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	dst, err := os.CreateTemp("", "datadog-operator-flare-rc-*.db")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(dst, src)
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(dst.Name())
		return "", err
	}
	return dst.Name(), nil
}

// remoteConfigOrgStatus returns the org status the Remote Configuration
// service exports, as the Agent status shows it.
func remoteConfigOrgStatus() (orgEnabled, apiKeyAuthorized string) {
	status, ok := expvar.Get(statusExpvar).(*expvar.Map)
	if !ok || status == nil {
		return "", ""
	}
	read := func(key string) string {
		if v, ok := status.Get(key).(*expvar.String); ok && v != nil {
			return v.Value()
		}
		return ""
	}
	return read("orgEnabled"), read("apiKeyScoped")
}

// ConfigPathOrgID returns the org ID of a Remote Configuration config path,
// datadog/<org_id>/<product>/<config_id>/<file>. Paths of the employee
// source have no org ID.
func ConfigPathOrgID(configPath string) (string, error) {
	if strings.HasPrefix(configPath, "employee/") {
		return "", fmt.Errorf("the Remote Configuration path %s is of the employee source, which has no org ID", configPath)
	}
	m := datadogConfigPath.FindStringSubmatch(configPath)
	if m == nil {
		return "", fmt.Errorf("the Remote Configuration path %q is not datadog/<org_id>/<product>/<config_id>/<file>", configPath)
	}
	return m[1], nil
}
