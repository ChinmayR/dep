set -euo pipefail
set +x

: "${ARTIFACTORY_URL:=http://artifactory.uber.internal:4587/artifactory}"
: "${ARTIFACTORY_REPO:=test-monorepo-v1}" # TODO replace with go-code

CURRENT_BRANCH=`git branch | grep \* | cut -d ' ' -f2-`
TOOL_VERSION=$1
BINARY_NAME=$(basename `pwd`)
SIMPLE_TOOL_VERSION=${TOOL_VERSION%-UBER} # Remove -UBER suffix from the version name if it's present.
SIMPLE_TOOL_VERSION=${SIMPLE_TOOL_VERSION#v} # Remove prefix "v" from the version tag.

_sha256sum() {
    if command -v sha256sum &>/dev/null; then
        # Linux
        sha256sum "$@"
    elif command -v gsha256sum &>/dev/null; then
        # macOS + brew install coreutils
        gsha256sum "$@"
    else
        # macOS
        shasum -a 256 "$@"
    fi
}

calculateSHA256() { _sha256sum "$1" | cut -d " " -f 1 ; }

upload_to_artifactory() {
    URI=$(curl --user ${ARTIFACTORY_AUTH} --header "X-Checksum-Sha256:${1}" -X PUT "${ARTIFACTORY_URL}/${ARTIFACTORY_REPO}/tools/${2}/${3}/dep-${3}-{$4}_${5}.bin" -T "${2}" --fail 2>&1 | grep uri | cut -d\" -f4)
    echo "  sha256_${4}_${5} = $1" >> $tmpfile
}

build_and_upload_binary() {
  local OS=$1
  local ARCH=$2
  # Now clean up the workspace and check out desired release tag.
  git clean -df
  git -c advice.detachedHead=false checkout $TOOL_VERSION
  # Now we can buidl the binary, calculate the chechsum and uplaod it to artifactory.
  GOOS=$OS GOARCH=$ARCH go build
  if [ ! -f $BINARY_NAME ]; then
      echo "Could not find ${BINARY_NAME} which should be produced by go build."
      exit 1
  fi
  BINARY_SHA256=$(calculateSHA256 $BINARY_NAME)
  upload_to_artifactory ${BINARY_SHA256} ${BINARY_NAME} ${SIMPLE_TOOL_VERSION} ${OS} ${ARCH}
}

tmpfile=$(mktemp /tmp/buckconfig-patch.XXXXXX)

git reset HEAD --hard
git clean -df
echo "[${BINARY_NAME}]" > $tmpfile
echo "  version = ${SIMPLE_TOOL_VERSION}" >> $tmpfile
build_and_upload_binary "darwin" "amd64" $1
build_and_upload_binary "linux" "amd64" $1

git clean -df
git checkout $CURRENT_BRANCH

echo "Now copy paste this into .buckconfig at monorepo root:"
cat $tmpfile
rm $tmpfile
