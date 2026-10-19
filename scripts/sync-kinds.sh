#!/bin/bash
# Copies the control plane's list of provider command kinds from one commit of OpenViCloud/titan into
# contract/ProviderCommandKinds.cs and records that commit in contract/SOURCE. The routing test reads the copy; CI
# checks that the copy is exactly the file at contract/SOURCE.
# Usage: scripts/sync-kinds.sh <titan commit, tag or branch>
set -euo pipefail
ref=${1:?usage: scripts/sync-kinds.sh <titan commit, tag or branch>}
cd "$(dirname "$0")/.."
path=control-plane/Titan.Domain/Aggregates/Platform/ProviderCommand/ProviderCommandKinds.cs
sha=$(gh api "repos/OpenViCloud/titan/commits/$ref" --jq .sha)
gh api "repos/OpenViCloud/titan/contents/$path?ref=$sha" \
  -H "Accept: application/vnd.github.raw" > contract/ProviderCommandKinds.cs
echo "$sha" > contract/SOURCE
echo "contract/ProviderCommandKinds.cs synced from OpenViCloud/titan@$sha; mention it in the pull request"
