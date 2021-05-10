module github.com/DataDog/datadog-operator

go 1.15

require (
	github.com/DataDog/datadog-api-client-go v1.0.0-beta.18
	github.com/DataDog/extendeddaemonset v0.5.1-0.20210315105301-41547d4ff09c
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/go-logr/logr v0.3.0
	github.com/go-openapi/spec v0.20.3
	github.com/gobwas/glob v0.2.3
	github.com/google/go-cmp v0.5.2
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/mholt/archiver/v3 v3.5.0
	github.com/olekukonko/tablewriter v0.0.0-20170122224234-a0225b3f23b5
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/zorkian/go-datadog-api v2.30.0+incompatible
	go.uber.org/zap v1.15.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8s.io/api v0.20.2
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/cli-runtime v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/klog/v2 v2.4.0
	k8s.io/kube-aggregator v0.20.2
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	sigs.k8s.io/controller-runtime v0.7.2
	sigs.k8s.io/yaml v1.2.0
)
