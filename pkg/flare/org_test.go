// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSiteFromSettings(t *testing.T) {
	assert.Equal(t, OrgValue{Value: "datadoghq.eu", Source: "DD_SITE from env"}, SiteFromSettings(" datadoghq.eu ", "https://app.datadoghq.com", "env"))
	assert.Equal(t, OrgValue{Value: "https://app.datad0g.com", Source: "DD_DD_URL or DD_URL from env (DD_SITE is unset)"}, SiteFromSettings("", "https://app.datad0g.com", "env"))
	assert.Equal(t, OrgValue{Value: DefaultSite, Source: "default: DD_SITE, DD_DD_URL and DD_URL are unset in env"}, SiteFromSettings("", "", "env"))
}

func orgTestPod(env []corev1.EnvVar, envFrom []corev1.EnvFromSource) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "manager",
			Command: []string{OperatorBinary},
			Env:     env,
			EnvFrom: envFrom,
		}}},
	}
}

func TestPodOrgInfo(t *testing.T) {
	unknownPod := PodOrgInfo(nil)
	assert.Equal(t, UnknownOrgValue("the Operator pod is unknown"), unknownPod.Site)
	assert.Equal(t, Unknown, unknownPod.OrgUUID.Value)
	assert.Contains(t, unknownPod.OrgUUID.Reason, "Remote Configuration client")
	assert.Equal(t, Unknown, unknownPod.OrgID.Value)

	info := PodOrgInfo(orgTestPod([]corev1.EnvVar{{Name: "DD_API_KEY", Value: "0123456789abcdef0123456789abcdef"}, {Name: "DD_SITE", Value: "us3.datadoghq.com"}}, nil))
	assert.Equal(t, OrgValue{Value: "us3.datadoghq.com", Source: "DD_SITE from the pod spec of datadog-operator-abc, container manager"}, info.Site)

	info = PodOrgInfo(orgTestPod([]corev1.EnvVar{{Name: "DD_URL", Value: "https://app.datad0g.com"}}, nil))
	assert.Equal(t, "https://app.datad0g.com", info.Site.Value)

	info = PodOrgInfo(orgTestPod(nil, nil))
	assert.Equal(t, DefaultSite, info.Site.Value)
	assert.Contains(t, info.Site.Source, "default")

	info = PodOrgInfo(orgTestPod([]corev1.EnvVar{{Name: "DD_SITE", ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "dd-config"}, Key: "site"}}}}, nil))
	assert.Equal(t, UnknownOrgValue("DD_SITE is set from ConfigMap dd-config, key site in the pod spec of datadog-operator-abc, container manager, which the flare doesn't read"), info.Site)

	info = PodOrgInfo(orgTestPod(nil, []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "dd-env"}}}}))
	assert.Equal(t, Unknown, info.Site.Value)
	assert.Contains(t, info.Site.Reason, "envFrom")
}
