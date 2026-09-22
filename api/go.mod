module github.com/DataDog/datadog-operator/api

go 1.26.0

toolchain go1.26.7

require (
	github.com/DataDog/datadog-api-client-go/v2 v2.64.0
	github.com/google/go-cmp v0.7.0
	github.com/stretchr/testify v1.12.1
	// Keep these Kubernetes dependencies compatible with datadog-agent. Any bump
	// must be coordinated with a matching dependency bump in datadog-agent.
	k8s.io/api v0.35.3
	k8s.io/apiextensions-apiserver v0.35.1
	k8s.io/apimachinery v0.35.3
	k8s.io/kube-openapi v0.0.0-20251125145642-4e65d59e963e
	k8s.io/utils v0.0.0-20251222233032-718f0e51e6d2
	sigs.k8s.io/controller-runtime v0.23.3
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/DataDog/zstd v1.5.8-0.20260421145859-31a7e515a571 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-openapi/jsonpointer v1.0.0 // indirect
	github.com/go-openapi/jsonreference v1.0.0 // indirect
	github.com/go-openapi/swag v0.27.1 // indirect
	github.com/go-openapi/swag/cmdutils v0.27.1 // indirect
	github.com/go-openapi/swag/conv v0.27.1 // indirect
	github.com/go-openapi/swag/fileutils v0.27.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.27.1 // indirect
	github.com/go-openapi/swag/loading v0.27.1 // indirect
	github.com/go-openapi/swag/mangling v0.27.1 // indirect
	github.com/go-openapi/swag/netutils v0.27.1 // indirect
	github.com/go-openapi/swag/pools v0.27.1 // indirect
	github.com/go-openapi/swag/stringutils v0.27.1 // indirect
	github.com/go-openapi/swag/typeutils v0.27.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.27.1 // indirect
	github.com/goccy/go-json v0.10.4 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/onsi/ginkgo/v2 v2.32.1 // indirect
	github.com/onsi/gomega v1.43.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.4.2 // indirect
)
