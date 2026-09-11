module github.com/DataDog/datadog-operator

go 1.26.7

require (
	github.com/DataDog/datadog-agent/pkg/util/scrubber v0.82.3 // indirect
	github.com/DataDog/datadog-api-client-go/v2 v2.64.0
	github.com/Masterminds/semver/v3 v3.5.0
	github.com/go-logr/logr v1.4.4
	github.com/gobwas/glob v0.2.3
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/hako/durafmt v0.0.0-20210608085754-5c1018a4e16b
	github.com/imdario/mergo v0.3.16
	github.com/mholt/archiver/v3 v3.5.1
	github.com/olekukonko/tablewriter v1.1.4
	github.com/onsi/gomega v1.43.0
	github.com/pkg/errors v0.9.1 // indirect
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.12.1
	go.uber.org/zap v1.28.0
	gopkg.in/DataDog/dd-trace-go.v1 v1.74.8
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.37.0
	k8s.io/apiextensions-apiserver v0.37.0
	k8s.io/apimachinery v0.37.0
	k8s.io/cli-runtime v0.37.0
	k8s.io/client-go v0.37.0
	k8s.io/klog/v2 v2.140.0
	k8s.io/kube-aggregator v0.37.0
	k8s.io/kube-openapi v0.0.0-20260721132016-d427ff9ee9ad
	sigs.k8s.io/controller-runtime v0.24.1
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/DataDog/datadog-agent/pkg/config/create v0.82.3
	github.com/DataDog/datadog-agent/pkg/config/model v0.82.3
	github.com/DataDog/datadog-agent/pkg/config/remote v0.82.3
	github.com/DataDog/datadog-agent/pkg/proto v0.82.3
	github.com/DataDog/datadog-agent/pkg/remoteconfig/state v0.82.3
	github.com/DataDog/datadog-operator/api v0.0.0-20250130131115-7f198adcc856
	github.com/aws/aws-sdk-go-v2 v1.45.1
	github.com/aws/aws-sdk-go-v2/config v1.33.1
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.75.1
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.78.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.325.1
	github.com/aws/aws-sdk-go-v2/service/eks v1.95.1
	github.com/aws/aws-sdk-go-v2/service/iam v1.61.1
	github.com/aws/aws-sdk-go-v2/service/sts v1.47.1
	github.com/aws/karpenter-provider-aws v1.14.1
	github.com/aws/smithy-go v1.28.1
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc
	github.com/fatih/color v1.19.0
	github.com/go-logr/zapr v1.3.0
	github.com/gobuffalo/flect v1.0.3
	github.com/google/go-containerregistry v0.22.0
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/mattn/go-runewidth v0.0.28
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/onsi/ginkgo/v2 v2.32.1
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/prometheus/client_golang v1.24.1
	github.com/samber/lo v1.53.0
	go.etcd.io/bbolt v1.5.0
	golang.org/x/exp v0.0.0-20260824195058-e88cd73687aa
	golang.org/x/text v0.41.0
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
	helm.sh/helm/v3 v3.21.4
	k8s.io/kubectl v0.37.0
	k8s.io/utils v0.0.0-20260707023825-cf1189d6abe3
	oras.land/oras-go/v2 v2.6.2
	sigs.k8s.io/karpenter v1.14.1
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go/auth v0.20.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute v1.64.0 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/DataDog/agent-payload/v5 v5.0.205 // indirect
	github.com/DataDog/datadog-agent/comp/core/delegatedauth v0.82.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/builder v0.82.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/flare/types v0.82.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/secrets/def v0.82.3 // indirect
	github.com/DataDog/datadog-agent/comp/core/telemetry v0.82.3 // indirect
	github.com/DataDog/datadog-agent/comp/def v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/collector/check/defaults v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/basic v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/buildschema v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/env v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/helper v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/nodetreemodel v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/setup v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/config/structure v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/fips v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/template v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/log v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/stats v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/trace/traceutil v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/backoff v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/cache v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/defaultpaths v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/executable v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/filesystem v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/grpc v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/http v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/log v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/pointer v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/system v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/uuid v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/util/winutil v0.82.3 // indirect
	github.com/DataDog/datadog-agent/pkg/version v0.82.3 // indirect
	github.com/DataDog/datadog-go/v5 v5.9.0 // indirect
	github.com/DataDog/dd-trace-go/v2 v2.3.0 // indirect
	github.com/DataDog/go-acl v1.0.1 // indirect
	github.com/DataDog/go-tuf v1.1.1-0.5.2 // indirect
	github.com/DataDog/gostackparse v0.7.0 // indirect
	github.com/DataDog/zstd v1.5.8-0.20260421145859-31a7e515a571 // indirect
	github.com/MakeNowJust/heredoc v1.0.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/sprig/v3 v3.3.0 // indirect
	github.com/Masterminds/squirrel v1.5.4 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.20.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.19.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.5.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.5.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.14.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.35.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.40.1 // indirect
	github.com/awslabs/operatorpkg v0.0.0-20260708223819-4da4c353c5fa // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/chai2010/gettext-go v1.0.2 // indirect
	github.com/clipperhouse/displaywidth v0.10.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.6.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/creack/pty v1.1.21 // indirect
	github.com/cyphar/filepath-securejoin v0.7.0 // indirect
	github.com/docker/cli v29.7.2+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.9.5 // indirect
	github.com/dsnet/compress v0.0.2-0.20210315054119-f66993602bf5 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/exponent-io/jsonpath v0.0.0-20210407135951-1de76d718b3f // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/fxamacker/cbor/v2 v2.9.1 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-gorp/gorp/v3 v3.1.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
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
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/goccy/go-json v0.10.4 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/cel-go v0.30.0 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.15 // indirect
	github.com/googleapis/gax-go/v2 v2.22.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/gosuri/uitable v0.0.4 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmoiron/sqlx v1.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/klauspost/pgzip v1.2.5 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lann/builder v0.0.0-20180802200727-47ae307949d0 // indirect
	github.com/lann/ps v0.0.0-20150810152359-62de8c46ede0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de // indirect
	github.com/lufia/plan9stats v0.0.0-20260330125221-c963978e514e // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/mdlayher/vsock v1.3.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.5.1 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nwaples/rardecode v1.1.3 // indirect
	github.com/olekukonko/cat v0.0.0-20250911104152-50322a0618f6 // indirect
	github.com/olekukonko/errors v1.2.0 // indirect
	github.com/olekukonko/ll v0.1.6 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.18 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/richardartoul/molecule v1.0.1-0.20240531184615-7ca0df43c0b3 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rubenv/sql-migrate v1.8.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/secure-systems-lab/go-securesystemslib v0.11.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.6 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.19.0 // indirect
	go.uber.org/fx v1.24.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.56.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	golang.org/x/xerrors v0.0.0-20240903120638-7835f813f4da // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/api v0.279.0 // indirect
	google.golang.org/genproto v0.0.0-20260319201613-d00831a3d3e7 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260618152121-87f3d3e198d3 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/apiserver v0.37.0 // indirect
	k8s.io/cloud-provider v0.35.0 // indirect
	k8s.io/component-base v0.37.0 // indirect
	k8s.io/component-helpers v0.37.0 // indirect
	k8s.io/csi-translation-lib v0.35.0 // indirect
	k8s.io/gengo/v2 v2.0.0-20260408192533-25e2208e0dc3 // indirect
	k8s.io/streaming v0.37.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.36.0 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/kustomize/api v0.21.1 // indirect
	sigs.k8s.io/kustomize/kyaml v0.21.1 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.4.2 // indirect
)
