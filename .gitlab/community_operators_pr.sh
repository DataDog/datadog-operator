#!/bin/bash

version="$1"
shift
repos=("$@") # array of repos

echo $version
echo "${repos[@]}"

OPERATOR_DIR="datadog-operator"
BUNDLE_DIR="bundle"
WORKING_DIR=$dir
PR_BRANCH_NAME="datadog-operator-$version"

clone_and_sync_fork() {
  gh repo clone DataDog/$repo /tmp
  cd /tmp/$repo
  gh repo sync $repo \
  --source $org/$repo \
  --force
}

update_bundle() {
  cd /tmp/$repo
  mkdir operators/$OPERATOR_DIR/$version
  cp -R $RUNNER_WORKING_DIR/$BUNDLE_DIR/* operators/$OPERATOR_DIR/$version
}

create_pr() {
  message="operator datadog-operator ($version)"
  body="operator datadog-operator ($version)"
  git checkout -b $PR_BRANCH_NAME
  git add -A
  git commit -s -m "$message"
  git push -f --set-upstream origin $PR_BRANCH_NAME
  gh pr create --title "$message" \
               --body "$body" \
               --repo DataDog/$repo \
               --base main \
               --draft
}


for repo in "${repos[@]}"
do
  case "$repo" in
    community-operators | community-operators-prod)
      ORG="k8s-operatorhub"
      OPERATOR_DIR="datadog-operator"
      BUNDLE_PATH="bundle"
      ;;&
    community-operators-prod | redhat-marketplace-operators | certified-operators)
      ORG="redhat-openshift-ecosystem"
      ;;&
    redhat-marketplace-operators)
      OPERATOR_DIR="datadog-operator-certified-rhmp"
      BUNDLE_PATH="bundle-redhat-mp"
      ;;
    certified-operators)
      OPERATOR_DIR="datadog-operator-certified"
      BUNDLE_PATH="bundle-redhat"
      ;;
    *)
      ;;
  esac

  echo "REPO $repo"
  echo "ORG $ORG"
  echo "OPERATOR_DIR $OPERATOR_DIR"
  echo "BUNDLE_PATH" $BUNDLE_PATH
  echo "==============================="

  clone_and_sync_fork
  update_bundle
#  create_pr

done


clone_and_sync_fork() {
  gh repo clone DataDog/$repo /tmp
  cd /tmp/$repo
  gh repo sync $repo \
  --source $org/$repo \
  --force
}

update_bundle() {
  cd /tmp/$repo
  mkdir operators/$OPERATOR_DIR/$version
  cp -R $RUNNER_WORKING_DIR/$BUNDLE_DIR/* operators/$OPERATOR_DIR/$version
}

#create_pr() {
#  message="operator datadog-operator ($version)"
#  body="operator datadog-operator ($version)"
#  git checkout -b $PR_BRANCH_NAME
#  git add -A
#  git commit -s -m "$message"
#  git push -f --set-upstream origin $PR_BRANCH_NAME
#  gh pr create --title "$message" \
#               --body "$body" \
#               --repo DataDog/$repo \
#               --base main \
#               --draft
#}