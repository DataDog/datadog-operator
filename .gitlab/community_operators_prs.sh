#!/bin/bash

VERSION="$1" # 1st argument passed to script is the operator version
shift
REPOS=("$@") # Make an array of repos with the remaining arguments passed to script

OPERATOR_SUBPATH="datadog-operator"
BUNDLE_PATH="bundle"
WORKING_DIR=$PWD
PR_BRANCH_NAME="datadog-operator-$VERSION"
CI_PROJECT_DIR="<YOUR_PATH_TO_LOCAL_DATADOG_OPERATOR_REPO"

mkdir tmp

clone_and_sync_fork() {
  cd "$WORKING_DIR"/tmp || exit
  gh repo clone DataDog/"$repo"
  cd ./"$repo" || exit
  gh repo sync DataDog/"$repo" \
  --source "$ORG"/"$repo" \
  --force
}

update_bundle() {
  cd "$WORKING_DIR"/tmp/"$repo" || exit
  mkdir operators/$OPERATOR_SUBPATH/"$VERSION"
  cp -R "$CI_PROJECT_DIR"/$BUNDLE_PATH/* operators/$OPERATOR_SUBPATH/"$VERSION"
}

create_pr() {
  message="operator datadog-operator ($VERSION)"
  body="Update operator datadog-operator ($VERSION).<br><br>Pull request triggered by $GITLAB_USER_EMAIL."
  git checkout -b "$PR_BRANCH_NAME"
  git add -A
  git commit -s -m "$message"
  git push -f --set-upstream origin "$PR_BRANCH_NAME"
  gh pr create --title "$message" \
               --body "$body" \
               --repo $ORG/"$repo" \
               --base main
}


for repo in "${REPOS[@]}"
do
  # set up env vars for each repo
  case "$repo" in
    community-operators | community-operators-prod)
      OPERATOR_SUBPATH="datadog-operator"
      BUNDLE_PATH="bundle"
      ;;&
    community-operators-prod | redhat-marketplace-operators | certified-operators)
      ORG="redhat-openshift-ecosystem"
      ;;&
    community-operators)
      ORG="k8s-operatorhub"
      ;;
    redhat-marketplace-operators)
      OPERATOR_SUBPATH="datadog-operator-certified-rhmp"
      BUNDLE_PATH="bundle-redhat-mp"
      ;;
    certified-operators)
      OPERATOR_SUBPATH="datadog-operator-certified"
      BUNDLE_PATH="bundle-redhat"
      ;;
    *)
      ;;
  esac

  clone_and_sync_fork
  update_bundle
  create_pr

done

# clean up /tmp
rm -rf "$WORKING_DIR"/tmp
