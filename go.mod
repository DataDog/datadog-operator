module github.com/DataDog/datadog-operator

go 1.16

require (
	github.com/DataDog/datadog-api-client-go v1.7.0
	github.com/DataDog/extendeddaemonset v0.5.1-0.20210315105301-41547d4ff09c
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/spec v0.20.3
	github.com/gobwas/glob v0.2.3
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.2.0 // indirect
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/mholt/archiver/v3 v3.5.0
	github.com/olekukonko/tablewriter v0.0.0-20170122224234-a0225b3f23b5
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/ulikunitz/xz v0.5.8 // indirect
	github.com/zorkian/go-datadog-api v2.30.0+incompatible
	go.uber.org/zap v1.17.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.21.2
	k8s.io/apiextensions-apiserver v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/cli-runtime v0.21.2
	k8s.io/client-go v0.21.2
	k8s.io/klog/v2 v2.8.0
	k8s.io/kube-aggregator v0.21.2
	k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/yaml v1.2.0
)
