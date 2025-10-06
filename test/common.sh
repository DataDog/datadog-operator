# AWS_ACCOUNT=agent-sandbox
# export AWS_REGION=us-east-1
AWS_ACCOUNT=container-integrations
export AWS_REGION=eu-west-3

export CLUSTER_NAME=${USER}-karpenter-test

export AWS_PROFILE=exec-sso-${AWS_ACCOUNT}-account-admin

export VPC=$(aws ec2 describe-vpcs \
                 --filters "Name=tag:Name,Values=$AWS_ACCOUNT" \
                 --query 'Vpcs[0].VpcId' \
                 --output text)
