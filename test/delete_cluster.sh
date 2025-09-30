#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
source common.container-integrations.sh

set +e  # Ignore error and proceed to deep clean.
        # `eksctl` can claim that the cluster doesn’t exist anymore
        # while there’s still an orphan CloudFormation stack.

eksctl delete cluster \
       --name "${CLUSTER_NAME}" \
       --wait \
       --force

aws cloudformation delete-stack \
    --stack-name "eksctl-${CLUSTER_NAME}-cluster"

SG=$(aws ec2 describe-security-groups \
         --filter Name=vpc-id,Values="${VPC}" Name=group-name,Values="${CLUSTER_NAME}" \
         --query 'SecurityGroups[*].[GroupId]' \
         --output text)
[[ -n ${SG} ]] && aws ec2 delete-security-group \
                        --group-id "${SG}"
