# Cloud identity (delegated) authentication

The Datadog Operator can authenticate to the Datadog API with its **AWS IAM role** instead of a Datadog application key. This works with:

- [EKS Pod Identity][1]
- [IAM Roles for Service Accounts (IRSA)][2]
- Other sources of temporary AWS credentials, such as assumed roles and instance profiles

Static IAM user keys are not supported, because the identity proof requires a session token.

The flow works like this:

1. The Operator resolves AWS credentials for its pod through the AWS SDK default chain.
2. It signs an STS `GetCallerIdentity` request with those credentials. The signature proves its identity and is bound to your Datadog org UUID.
3. It sends that proof to `/api/v2/delegated-token` on your Datadog site.
4. Datadog verifies the proof with AWS STS. It looks up the Datadog identity linked to the IAM role and returns a short-lived access token.
5. The Operator sends the token as a `Bearer` token. It caches the token and renews it two minutes before it expires. If renewal fails, the Operator keeps using the current token until it expires. If Datadog rejects a token with `401 Unauthorized`, the Operator drops it and requests a new one on the next reconcile.

The role needs no IAM permission policies, only a trust policy that lets the Operator pod assume it. The signed `GetCallerIdentity` request is never sent to AWS by the Operator; Datadog sends it to STS to verify it.

The Operator still talks to AWS to obtain the role's credentials. With EKS Pod Identity, it calls the EKS Pod Identity Agent on the node. With IRSA, the AWS SDK calls STS `AssumeRoleWithWebIdentity`, so the Operator needs network access to STS.

## Scope

Delegated authentication applies to the controllers that manage Datadog resources through the API:

- `DatadogMonitor`
- `DatadogSLO`
- `DatadogDashboard`
- `DatadogGenericResource`

In this mode those controllers do not need `DD_APP_KEY`.

`DD_API_KEY` is still used for intake. Intake covers Operator metrics and events, Remote Configuration, and metadata. The `DatadogAgent` credentials are not affected.

## Setup

### 1. Create the IAM role and associate it with the Operator service account

With EKS Pod Identity, the trust policy is:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": { "Service": "pods.eks.amazonaws.com" },
    "Action": ["sts:AssumeRole", "sts:TagSession"]
  }]
}
```

The role needs no permission policies. Then associate it with the Operator service account:

```shell
aws eks create-pod-identity-association \
  --cluster-name <CLUSTER> \
  --namespace <OPERATOR_NAMESPACE> \
  --service-account <OPERATOR_SERVICE_ACCOUNT> \
  --role-arn arn:aws:iam::<ACCOUNT_ID>:role/<ROLE_NAME>
```

#### Using IRSA instead

IRSA needs a different trust policy. The trust policy above only allows EKS Pod Identity. For IRSA:

- The cluster needs an IAM OIDC identity provider.
- The role's trust policy must allow `sts:AssumeRoleWithWebIdentity` from that provider, scoped to the Operator's namespace and service account.
- The service account is annotated with `eks.amazonaws.com/role-arn: arn:aws:iam::<ACCOUNT_ID>:role/<ROLE_NAME>`.

Follow the AWS guide to [assign an IAM role to a Kubernetes service account][3] for the full steps.

### 2. Link the IAM role to a Datadog identity

Create an identity mapping that links the role to the Datadog identity the Operator should act as. Follow the Datadog guide to [configure AWS identity mapping][4]. In Datadog, go to **Organization Settings** > **Workload Identity Federation** > **Identity Mappings**.

- **Prerequisites:** the AWS account must be added to the [Datadog AWS integration][5], and you need the `workload_identity_federation_config_read` and `workload_identity_federation_config_write` permissions.
- **AWS identity:** use the role's STS assumed-role ARN, not its IAM role ARN. Pods get a new session name every time, so end the ARN with a wildcard:
  ```
  arn:aws:sts::<ACCOUNT_ID>:assumed-role/<ROLE_NAME>/*
  ```
- **Datadog identity:** a service account is recommended. Give it the permissions the Operator needs, such as monitors, SLOs and dashboards write access.

Workload Identity Federation is not available on Datadog for Government sites.

If the role is not mapped, the token request fails. The error, including Datadog's explanation, appears in the Operator logs as `unable to get credentials`. The status of the `DatadogMonitor`, `DatadogSLO`, `DatadogDashboard` or `DatadogGenericResource` is not updated, and the reconcile is retried.

### 3. Configure the Operator

Set these environment variables on the Operator deployment. They use the same names as the Datadog Agent `delegated_auth` settings.

| Variable | Required | Description |
|---|---|---|
| `DD_DELEGATED_AUTH_ORG_UUID` | yes | UUID of the Datadog organization. Setting it enables delegated authentication. |
| `DD_DELEGATED_AUTH_PROVIDER` | no | Cloud provider. Only `aws` is supported, and it is the default. |
| `DD_DELEGATED_AUTH_AWS_REGION` | no | Region of the STS endpoint used in the identity proof. Defaults to the region resolved by the AWS SDK. EKS Pod Identity sets `AWS_REGION` automatically. Without any region, the global endpoint is used. |
| `DD_SITE` / `DD_URL` | no | Datadog site. The token is requested from the same site as the API calls. |
| `DD_API_KEY` | yes | Still required for intake. |

For example, with the Helm chart:

```yaml
apiKeyExistingSecret: datadog-secret
env:
  - name: DD_DELEGATED_AUTH_ORG_UUID
    value: "<ORG_UUID>"
```

If the configuration is invalid, for example an unsupported provider, the Operator logs an error and falls back to API and application keys.

[1]: https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html
[2]: https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html
[3]: https://docs.aws.amazon.com/eks/latest/userguide/associate-service-account-role.html
[4]: https://docs.datadoghq.com/account_management/workload_identity_federation/#configure-aws-identity-mapping-in-datadog
[5]: https://docs.datadoghq.com/integrations/amazon_web_services/
