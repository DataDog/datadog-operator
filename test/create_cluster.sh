#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
source common.sh

SG=$(aws ec2 create-security-group \
         --description "Security group for ${CLUSTER_NAME} EKS cluster" \
         --group-name "${CLUSTER_NAME}" \
         --vpc-id ${VPC} \
         --output text)

SG_APPGATE=$(aws ec2 describe-security-groups \
                 --filter Name=vpc-id,Values=${VPC} Name=group-name,Values=appgate-gateway \
                 --query 'SecurityGroups[*].[GroupId]' \
                 --output text)

aws ec2 authorize-security-group-ingress \
    --group-id "${SG}" \
    --protocol tcp \
    --port 22 \
    --source-group "${SG_APPGATE}"

aws ec2 authorize-security-group-ingress \
    --group-id "${SG}" \
    --protocol tcp \
    --port 443 \
    --source-group "${SG_APPGATE}"

yq -i "
   .metadata.name = \"${CLUSTER_NAME}\" |
   .metadata.tags.Creator = \"${USER}\" |
   .managedNodeGroups[0].tags.Creator = \"${USER}\" |
   .vpc.id = \"${VPC}\" |
   .vpc.securityGroup = \"${SG}\"
" eksctl.yaml

eksctl create cluster \
       --config-file eksctl.yaml

kubeconfig="$(mktemp)"
trap 'rm ${kubeconfig}' EXIT

eksctl utils write-kubeconfig \
       --config-file eksctl.yaml \
       --kubeconfig "${kubeconfig}"

kubectl --kubeconfig "${kubeconfig}" apply -f workload.yaml
