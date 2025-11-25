#!/usr/bin/env bash
# Integration test for cfgmerge-krm Kustomize plugin
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Get the directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "==> Running cfgmerge-krm Kustomize integration test"

# Check if cfgmerge-krm is available
if ! command -v cfgmerge-krm &> /dev/null; then
    echo -e "${YELLOW}Warning: cfgmerge-krm not found in PATH${NC}"
    echo "Attempting to build it..."

    # Try to build from repo root
    REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
    if [ -f "$REPO_ROOT/cmd/cfgmerge-krm/main.go" ]; then
        (cd "$REPO_ROOT" && go build -o "$SCRIPT_DIR/cfgmerge-krm" ./cmd/cfgmerge-krm)
        export PATH="$SCRIPT_DIR:$PATH"
        echo -e "${GREEN}Built cfgmerge-krm successfully${NC}"
    else
        echo -e "${RED}ERROR: Could not find cfgmerge-krm source${NC}"
        exit 1
    fi
fi

# Check if kustomize is available
if ! command -v kustomize &> /dev/null; then
    echo -e "${RED}ERROR: kustomize not found in PATH${NC}"
    echo "Install kustomize: https://kubectl.docs.kubernetes.io/installation/kustomize/"
    exit 1
fi

# Build the dev environment
echo "==> Running kustomize build envs/dev"
cd "$SCRIPT_DIR/envs/dev"
kustomize build --enable-alpha-plugins --enable-exec . > "$SCRIPT_DIR/actual-output.yaml" 2>&1 || {
    echo -e "${RED}ERROR: kustomize build failed${NC}"
    cat "$SCRIPT_DIR/actual-output.yaml"
    rm -f "$SCRIPT_DIR/actual-output.yaml"
    exit 1
}

# Compare with expected output
echo "==> Comparing output with expected"
cd "$SCRIPT_DIR"

if diff -u expected-output.yaml actual-output.yaml > /dev/null; then
    echo -e "${GREEN}==> Test passed! Output matches expected.${NC}"
    rm -f actual-output.yaml
    exit 0
else
    echo -e "${RED}==> Test failed! Output differs from expected.${NC}"
    echo ""
    echo "Diff:"
    diff -u expected-output.yaml actual-output.yaml || true
    echo ""
    echo "Actual output saved to: $SCRIPT_DIR/actual-output.yaml"
    exit 1
fi
