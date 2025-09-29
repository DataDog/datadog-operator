#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
source common.container-integrations.sh

SG=$(aws ec2 create-security-group \
         --description "Security group for ${CLUSTER_NAME} EKS cluster" \
         --group-name "${CLUSTER_NAME}" \
         --vpc-id ${VPC} \
         --output text)

PL_APPGATE=$(aws ec2 describe-managed-prefix-lists \
                 --filters "Name=prefix-list-name,Values=vpn-services-commercial-appgate" \
                 --query "PrefixLists[*].PrefixListId" \
                 --output text)

for tcp_port in 22 443; do
    aws ec2 authorize-security-group-ingress \
        --group-id "${SG}" \
        --ip-permissions "IpProtocol=tcp,FromPort=${tcp_port},ToPort=${tcp_port},PrefixListIds=[{PrefixListId=${PL_APPGATE}}]"
done

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
