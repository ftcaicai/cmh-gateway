#!/usr/bin/env bash
# Exercise the actual gate in a throwaway repository, not the implementation tree.
set -euo pipefail
source_root=$(git rev-parse --show-toplevel)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/cmh-diff-gate.XXXXXX")
cleanup() {
  case "$fixture" in
    "${TMPDIR:-/tmp}"/cmh-diff-gate.*) rm -rf -- "$fixture" ;;
    *) echo "refusing unexpected cleanup target" >&2; exit 2 ;;
  esac
}
trap cleanup EXIT
mkdir -p "$fixture/ops/ci" "$fixture/ext/docs"
cp "$source_root/ops/ci/check-upstream-diff.sh" "$fixture/ops/ci/check-upstream-diff.sh"
cd "$fixture"
git init -q
printf 'package main\n' > main.go
git add main.go
git -c user.name=Fixture -c user.email=fixture@example.invalid commit -qm baseline
git tag v1.0.0
printf '## 白名单\n\n```\n' > ext/docs/HOOKS.md
for ((i=1; i<=49; i++)); do
  printf 'official-%s.go\n' "$i" >> ext/docs/HOOKS.md
done
printf '```\n' >> ext/docs/HOOKS.md

# Result output stays in the extension zone so the gate doesn't count it.
# Capture via an absolute ignored directory, rather than adding a whitelist waiver.
check() {
  local expected=$1 marker=$2 status=0
  bash ops/ci/check-upstream-diff.sh v1.0.0 > ops/result.txt 2>&1 || status=$?
  if [[ "$status" != "$expected" ]] || ! grep -qF "$marker" ops/result.txt; then
    printf 'gate fixture failed: expected exit %s and marker %s, actual exit %s\n' "$expected" "$marker" "$status" >&2
    exit 1
  fi
}
check 0 '0 / 48'
for ((i=1; i<=48; i++)); do printf '// fixture\n' > "official-$i.go"; done
check 0 '48 / 48'
printf '// fixture\n' > official-49.go
check 1 'hook file budget exceeded: 49 > 48'
printf '' > official-49.go
# Empty new files still count. Delete only the verified fixture-owned file.
[[ "$PWD/official-49.go" == "$fixture/official-49.go" ]]
rm -- official-49.go
for ((i=2; i<=48; i++)); do
  [[ "$PWD/official-$i.go" == "$fixture/official-$i.go" ]]
  rm -- "official-$i.go"
done
for ((i=1; i<=6000; i++)); do printf '// fixture\n'; done > official-1.go
check 0 '6000 / 6000'
printf '// excess\n' >> official-1.go
check 1 'hook line budget exceeded: 6001 > 6000'
printf '// forbidden\n' > unregistered.go
check 1 'NOT registered'
printf 'PASS: empty,48/49 files,6000/6001 lines,and exact whitelist rejection\n'
