#!/bin/bash

VERSION="$1" # 1st argument passed to script is the operator version
shift
REPOS=("$@") # Make an array of repos with the remaining arguments passed to script

OPERATOR_SUBPATH="datadog-operator"
BUNDLE_NAME="bundle"
WORKING_DIR=$PWD
PR_BRANCH_NAME="datadog-operator-$VERSION"

mkdir tmp

clone_and_sync_fork() {
  echo "Cloning fork DataDog/$repo."
  cd "$WORKING_DIR"/tmp || exit
  gh repo clone DataDog/"$repo"
  cd ./"$repo" || exit

  # sync forked repo on a new branch because of repo restrictions against merging upstream to default "main" branch
  echo "Syncing fork DataDog/$repo with upstream $ORG/$repo."
  git checkout -b "$PR_BRANCH_NAME"
  git push -f --set-upstream origin "$PR_BRANCH_NAME"
  gh repo sync DataDog/"$repo" \
  --branch "$PR_BRANCH_NAME" \
  --source "$ORG"/"$repo" \
  --force
  git pull
}

update_bundle() {
  dest_path=operators/$OPERATOR_SUBPATH/"$VERSION"
  echo "Updating bundle at \`$dest_path\` with source: \`$BUNDLE_NAME\`"
  mkdir "$dest_path"
  cp -R "$CI_PROJECT_DIR"/$BUNDLE_NAME/* "$dest_path"
}

create_pr() {
  echo "Creating pull request for repo: $ORG/$repo"
  message="operator $OPERATOR_SUBPATH ($VERSION)"
  body="Update operator $OPERATOR_SUBPATH ($VERSION).<br><br>Pull request triggered by $GITLAB_USER_EMAIL."
  git add -A
  git commit -s -m "$message"
  git push -f --set-upstream origin "$PR_BRANCH_NAME"
  gh pr create --title "$message" \
               --body "$body" \
               --repo "$ORG"/"$repo" \
               --base main
}


for repo in "${REPOS[@]}"
do
  # set up env vars for each repo
  case "$repo" in
    community-operators | community-operators-prod)
      OPERATOR_SUBPATH="datadog-operator"
      BUNDLE_NAME="bundle"
      ;;&
    community-operators-prod | redhat-marketplace-operators | certified-operators)
      ORG="redhat-openshift-ecosystem"
      ;;&
    community-operators)
      ORG="k8s-operatorhub"
      ;;
    redhat-marketplace-operators)
      OPERATOR_SUBPATH="datadog-operator-certified-rhmp"
      BUNDLE_NAME="bundle-redhat-mp"
      ;;
    certified-operators)
      OPERATOR_SUBPATH="datadog-operator-certified"
      BUNDLE_NAME="bundle-redhat"
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
