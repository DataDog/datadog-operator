module github.com/DataDog/datadog-operator

go 1.13

require (
	github.com/datadog/extendeddaemonset v0.1.1-0.20200618074032-94ec1f3a5192
	github.com/dsnet/compress v0.0.1 // indirect
	github.com/frankban/quicktest v1.9.0 // indirect
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.4
	github.com/google/go-cmp v0.4.0
	github.com/hako/durafmt v0.0.0-20191009132224-3f39dc1ed9f4
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/mholt/archiver v3.1.1+incompatible
	github.com/mikefarah/yq/v3 v3.0.0-20200418141808-3ccd32a47e54
	github.com/nwaples/rardecode v1.0.0 // indirect
	github.com/olekukonko/tablewriter v0.0.2
	github.com/operator-framework/operator-sdk v0.19.0
	github.com/pierrec/lz4 v2.4.1+incompatible // indirect
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.5.1
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/zorkian/go-datadog-api v2.25.0+incompatible
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/cli-runtime v0.18.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.18.2
	k8s.io/gengo v0.0.0-20200114144118-36b2048a9120
	k8s.io/kube-aggregator v0.17.3
	k8s.io/kube-openapi v0.0.0-20200121204235-bf4fb3bd569c
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/yaml v1.2.0
)

// From operator-sdk, see https://github.com/operator-framework/operator-sdk/tree/master/website/content/en/docs/upgrading-sdk-version
replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	k8s.io/client-go => k8s.io/client-go v0.18.2 // Required by prometheus-operator
)
