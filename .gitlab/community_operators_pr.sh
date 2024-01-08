#!/bin/bash

set +o xtrace

version="$1"
shift
repos=("$@") # array of repos

echo $version
echo "${repos[@]}"

OPERATOR_DIR="datadog-operator" # need a better name for this var
BUNDLE_DIR="bundle"
WORKING_DIR=$PWD
PR_BRANCH_NAME="datadog-operator-$version"

mkdir tmp

clone_and_sync_fork() {
  cd $WORKING_DIR/tmp
  gh repo clone DataDog/$repo
  cd ./$repo
  gh repo sync DataDog/$repo \
  --source $ORG/$repo \
  --force
}

update_bundle() {
  cd $WORKING_DIR/tmp/$repo
  mkdir operators/$OPERATOR_DIR/$version
  cp -R $CI_PROJECT_DIR/$BUNDLE_PATH/* operators/$OPERATOR_DIR/$version
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

  clone_and_sync_fork
  update_bundle
#  create_pr

done

rm -rf $WORKING_DIR/tmp