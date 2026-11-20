// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"expvar"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/create"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// newOrgDB creates a Remote Configuration database with the org bucket the
// uptane client writes, and returns it still open, as the running service
// keeps it.
func newOrgDB(t *testing.T, entries map[string]string) (string, *bbolt.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "remote-config-test.db")
	db, err := bbolt.Open(path, 0o600, &bbolt.Options{Timeout: time.Second})
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte("director_targets")); err != nil {
			return err
		}
		if entries == nil {
			return nil
		}
		bucket, err := tx.CreateBucketIfNotExists([]byte(orgBucket))
		if err != nil {
			return err
		}
		for k, v := range entries {
			if err := bucket.Put([]byte(k), []byte(v)); err != nil {
				return err
			}
		}
		return nil
	}))
	return path, db
}

func TestStoredOrgUUID(t *testing.T) {
	// The service keeps its database open and locked: the flare reads a copy.
	path, _ := newOrgDB(t, map[string]string{"1-uuid": "old-uuid", "12-uuid": "latest-uuid", "2-uuid": "older-uuid", "bad-uuid": "ignored", "3-other": "ignored"})
	uuid, err := StoredOrgUUID(path)
	require.NoError(t, err)
	assert.Equal(t, "latest-uuid", uuid)

	path, _ = newOrgDB(t, nil)
	_, err = StoredOrgUUID(path)
	assert.ErrorContains(t, err, "holds no org UUID yet")

	_, err = StoredOrgUUID(filepath.Join(t.TempDir(), "missing.db"))
	assert.EqualError(t, err, "the Remote Configuration database doesn't exist yet")
}

func TestFlareOrg(t *testing.T) {
	updater := &RemoteConfigUpdater{}
	org := updater.FlareOrg()
	assert.False(t, org.Started)
	assert.Equal(t, "the Remote Configuration client is not started", org.OrgUUIDError)

	cfg := create.NewConfig("datadog")
	cfg.SetDefault("site", "datad0g.com")
	cfg.BuildSchema()
	path, _ := newOrgDB(t, map[string]string{"1-uuid": "org-uuid"})
	updater = &RemoteConfigUpdater{rcClient: &rcclient.Client{}, rcDatabasePath: path, serviceConf: RcServiceConfiguration{cfg: cfg}}

	status := expvar.Get(statusExpvar).(*expvar.Map)
	enabled, authorized := &expvar.String{}, &expvar.String{}
	enabled.Set("true")
	authorized.Set("false")
	status.Set("orgEnabled", enabled)
	status.Set("apiKeyScoped", authorized)
	t.Cleanup(func() { status.Delete("orgEnabled"); status.Delete("apiKeyScoped") })

	org = updater.FlareOrg()
	assert.Equal(t, FlareOrg{Started: true, Site: "datad0g.com", OrgUUID: "org-uuid", OrgEnabled: "true", APIKeyAuthorized: "false"}, org)
}

func TestConfigPathOrgID(t *testing.T) {
	orgID, err := ConfigPathOrgID("datadog/2/AGENT_TASK/2f3c/config")
	require.NoError(t, err)
	assert.Equal(t, "2", orgID)

	_, err = ConfigPathOrgID("employee/AGENT_TASK/2f3c/config")
	assert.ErrorContains(t, err, "employee source")
	_, err = ConfigPathOrgID("")
	assert.Error(t, err)
	_, err = ConfigPathOrgID("datadog/x/AGENT_TASK/2f3c/config")
	assert.Error(t, err)
}
