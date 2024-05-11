# Release process

## Overview

The release process is based on freezing `main`, merging fixes to a dedicated release branch and releasing release candidates as things progress. Once we have a final version, the release branch is merged into `main` and the freeze is lifted.

## Steps

### Create the release branch and the first release candidate

1. Checkout the repository on the correct branch and changeset (`main`).
2. Create a new branch: `git checkout -b vX.Y`.
3. Prepare the first release candidate by running the bundle generation: `make VERSION=x.v.z-rc.1 bundle`.
4. Commit all the changes generated from the previous command:

   ```console
   $ git add .
   $ git commit -S -m "release vX.Y.X-rc.1"
   ```

5. Add release tag: `git tag vX.Y.Z-rc.1`.
6. Push the generated commit and tag to the repostory branch.

   ```console
   $ git push origin vX.Y
   $ git push origin vX.Y.Z-rc.1
   ```

### Create a release candidate after a bug fix

**Note:** The fix must be merged to the release branch `vX.Y`, not `main`.

1. Update the release branch `vX.Y` locally by pulling the bug fix merged upstream (`git fetch`, `git pull`)
2. Prepare the release candidate by running the bundle generation: `make VERSION=x.v.z-rc.w bundle`.
3. Commit all the changes generated from the previous command:

   ```console
   $ git add .
   $ git commit -S -m "release vX.Y.X-rc.W"
   ```

4. Add release tag: `git tag vX.Y.Z-rc.W`.
5. Push the generated commit and tag to the repostory branch.

   ```console
   $ git push origin vX.Y
   $ git push origin vX.Y.Z-rc.W
   ```

### Create the final version

1. Update the release branch `vX.Y` locally by pulling the bug fix merged upstream (`git fetch`, `git pull`)
2. Prepare the final release version by running the bundle generation: `make VERSION=x.v.z bundle`.
3. Commit all the changes generated from the previous command:

   ```console
   $ git add .
   $ git commit -S -m "release vX.Y.X"
   ```

4. Add release tag: `git tag vX.Y.Z`.
5. Push the generated commit and tag to the repostory branch.

   ```console
   $ git push origin vX.Y
   $ git push origin vX.Y.Z
   ```

6. Merge `vX.Y` into `main`

