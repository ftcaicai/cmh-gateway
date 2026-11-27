#!/usr/bin/env bash
# Official-zone diff gate (ext/AGENTS.md §2.7).
#
# Compares the working tree against the recorded upstream baseline and fails
# when any official-zone file differs unless it is listed in the whitelist
# block of ext/docs/HOOKS.md. Extension-zone paths are never checked.
#
# Usage:
#   ops/ci/check-upstream-diff.sh [<baseline-ref>]
# Baseline defaults to the tag recorded in ext/docs/sync-log.md (last row).
set -euo pipefail

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

hooks_file="ext/docs/HOOKS.md"
sync_log="ext/docs/sync-log.md"

# ---- resolve baseline -------------------------------------------------------
baseline="${1:-}"
if [[ -z "$baseline" ]]; then
  # Last table row of sync-log.md, second column, first token that looks like a tag.
  baseline=$(grep -E '^\|' "$sync_log" | tail -n 1 | awk -F'|' '{print $3}' | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' | head -n 1 || true)
fi
if [[ -z "$baseline" ]]; then
  echo "::error::cannot determine upstream baseline (pass it as argument or record it in $sync_log)"
  exit 2
fi
if ! git rev-parse -q --verify "${baseline}^{commit}" >/dev/null; then
  echo "::error::baseline '$baseline' is not a known commit/tag; run 'git fetch upstream --tags'"
  exit 2
fi

# ---- extension zone (never checked) ----------------------------------------
# Keep in sync with ext/AGENTS.md §1.1.
ext_zone_regex='^(ext/|web-cmh/|brandweb/|ops/|plugins/tasks/cmh-[^/]+/)'

# ---- whitelist from HOOKS.md -----------------------------------------------
# The fenced block after the "## 白名单" heading, one path per line.
whitelist=$(awk '
  /^## 白名单/ {in_section=1; next}
  in_section && /^```/ {if (in_block) exit; in_block=1; next}
  in_block {print}
' "$hooks_file" | sed '/^[[:space:]]*$/d')

# ---- compute official-zone changes -----------------------------------------
# Compare baseline with HEAD (committed) plus the working tree (staged/unstaged),
# so the gate works both in CI (clean checkout) and locally before commit.
changed=$( { git diff --name-only "$baseline" HEAD; git diff --name-only HEAD; git ls-files --others --exclude-standard; } \
  | sort -u \
  | grep -Ev "$ext_zone_regex" || true)

violations=()
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  if ! grep -qxF "$f" <<<"$whitelist"; then
    violations+=("$f")
  fi
done <<<"$changed"

# ---- hook budget ------------------------------------------------------------
# Hard limits from ext/AGENTS.md §3.2.
# H1 admission RFC-specific cumulative ceiling, explicitly approved 2026-11-13.
# Registration remains exact-file based; this is not a directory-wide waiver.
# Scope: ext/docs/rfc/2026-11-13-admission-hook-budget.md (not funding H3/H7).
max_files=48
exception_file='middleware/task_plugin.go'
exception_max_lines=40
max_lines=6000
auth_base=17d70f116946f8099e977cfaa654d9590f96cc33
auth_files=$'controller/user.go\ncontroller/login_verification.go\ncontroller/auth_session.go\nservice/login_verification.go\nservice/auth_session.go\nmodel/auth_flow.go\nmodel/login_verification.go\nmodel/user_session.go\nmiddleware/auth.go'
if ! git rev-parse -q --verify "${auth_base}^{commit}" >/dev/null; then
  echo '::error::fixed authentication baseline is missing'
  exit 2
fi
auth_count=0
auth_lines=0
# Sidebar entry to the OEM console (R5), approved 2026-11-27: three named
# existing files measured against the sync baseline, outside H1 and H8.
# Scope: ext/docs/rfc/2026-11-27-official-nav-cmh-entry.md.
nav_files=$'web/src/hooks/use-sidebar-data.ts\nweb/src/components/layout/components/nav-group.tsx\nweb/src/components/command-menu.tsx'
nav_max_lines=20
nav_count=0
nav_lines=0
allowed_changed=$(grep -xF -f <(printf '%s\n' "$whitelist") <<<"$changed" || true)
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  if grep -qxF "$f" <<<"$auth_files"; then
    if ! git cat-file -e "${auth_base}:${f}" 2>/dev/null; then
      echo "::error::authentication scope requires existing fixed-base file: $f"
      exit 1
    fi
    stats=$(git diff --numstat "$auth_base" -- "$f")
    if ! awk '{if ($1 !~ /^[0-9]+$/ || $2 !~ /^[0-9]+$/) exit 1}' <<<"$stats"; then
      echo "::error::unmeasurable authentication diff: $f"
      exit 1
    fi
    n=$(awk '{a+=$1; d+=$2} END {print a+d+0}' <<<"$stats")
    auth_count=$((auth_count + 1))
    auth_lines=$((auth_lines + n))
  elif grep -qxF "$f" <<<"$nav_files"; then
    if ! git cat-file -e "${baseline}:${f}" 2>/dev/null; then
      echo "::error::navigation entry scope requires existing baseline file: $f"
      exit 1
    fi
    stats=$(git diff --numstat "$baseline" -- "$f")
    if ! awk '{if ($1 !~ /^[0-9]+$/ || $2 !~ /^[0-9]+$/) exit 1}' <<<"$stats"; then
      echo "::error::unmeasurable navigation entry diff: $f"
      exit 1
    fi
    n=$(awk '{a+=$1; d+=$2} END {print a+d+0}' <<<"$stats")
    nav_count=$((nav_count + 1))
    nav_lines=$((nav_lines + n))
  fi
done <<<"$allowed_changed"
allowed_changed=$(grep -vxF -f <(printf '%s\n%s\n' "$auth_files" "$nav_files") <<<"$allowed_changed" || true)
files_count=$(sed '/^[[:space:]]*$/d' <<<"$allowed_changed" | wc -l | tr -d ' ')
lines_count=0
if [[ -n "$allowed_changed" ]]; then
  # Tracked files: diff against baseline. Untracked (new) files: count their lines,
  # because `git diff` does not see them until they are added.
  while IFS= read -r f; do
    [[ -z "$f" ]] && continue
    if git cat-file -e "${baseline}:${f}" 2>/dev/null || git ls-files --error-unmatch "$f" >/dev/null 2>&1; then
      stats=$(git diff --numstat "$baseline" -- "$f")
      if awk '{if ($1 !~ /^[0-9]+$/ || $2 !~ /^[0-9]+$/) exit 1}' <<<"$stats"; then
        n=$(awk '{a+=$1; d+=$2} END {print a+d+0}' <<<"$stats")
      else
        echo "::error::unmeasurable official diff: $f"
        exit 1
      fi
    else
      n=$(wc -l <"$f" | tr -d ' ')
    fi
    lines_count=$((lines_count + n))
  done <<<"$allowed_changed"
fi

exception_lines=0
if grep -qxF "$exception_file" <<<"$allowed_changed"; then
  if git cat-file -e "${baseline}:${exception_file}" 2>/dev/null; then
    max_files=49
    exception_lines=$(git diff --numstat "$baseline" -- "$exception_file" | awk '{a+=$1; d+=$2} END {print a+d+0}')
  else
    exception_lines=$(wc -l <"$exception_file" | tr -d ' ')
    echo "::error::shared endpoint exception requires the existing baseline file"
    exit 1
  fi
fi

# ---- dependency direction (ext/AGENTS.md §2.2) -----------------------------
# No official-zone Go file may import ext/ or ext/<module>, except main.go's
# single blank import that installs the hooks. ext/core is allowed everywhere.
bad_imports=$(git grep -n -E '"github.com/QuantumNous/new-api/ext(/[^"]*)?"' -- '*.go' ':!ext/**' ':!main.go' \
  | grep -v '/ext/core"' || true)
main_ext_imports=$(grep -n -E '"github.com/QuantumNous/new-api/ext(/[^"]*)?"' main.go | grep -v -E '/ext/core"|^[0-9]+:\s*_ "github.com/QuantumNous/new-api/ext"' || true)

# ---- report -----------------------------------------------------------------
echo "baseline: $baseline"
echo "official-zone files changed (whitelisted): $files_count / $max_files"
echo "H1 official-zone lines changed (whitelisted): $lines_count / $max_lines"
echo "authentication files changed: $auth_count / 9"
echo "authentication lines changed from $auth_base: $auth_lines / 1800"
echo "combined official files changed: $((files_count + auth_count)) / 58"
echo "navigation entry files changed: $nav_count / 3"
echo "navigation entry lines changed: $nav_lines / $nav_max_lines"

status=0
if (( nav_lines > nav_max_lines )); then
  echo "::error::navigation entry line budget exceeded: $nav_lines > $nav_max_lines"
  status=1
fi
if (( auth_count > 9 || auth_lines > 1800 || files_count + auth_count > 58 || lines_count + auth_lines > 7800 )); then
  echo '::error::authentication or combined hook budget exceeded'
  status=1
fi
if (( ${#violations[@]} > 0 )); then
  echo "::error::official-zone files changed but NOT registered in $hooks_file:"
  printf '  - %s\n' "${violations[@]}"
  status=1
fi
if (( files_count > max_files )); then
  echo "::error::hook file budget exceeded: $files_count > $max_files"
  status=1
fi
if (( exception_lines > exception_max_lines )); then
  echo "::error::shared endpoint exception line budget exceeded: $exception_lines > $exception_max_lines"
  status=1
fi
if (( lines_count > max_lines )); then
  echo "::error::hook line budget exceeded: $lines_count > $max_lines"
  status=1
fi
if [[ -n "$bad_imports" ]]; then
  echo "::error::official-zone files import the extension zone (only ext/core is allowed):"
  sed 's/^/  - /' <<<"$bad_imports"
  status=1
fi
if [[ -n "$main_ext_imports" ]]; then
  echo "::error::main.go may only blank-import ext (and import ext/core):"
  sed 's/^/  - /' <<<"$main_ext_imports"
  status=1
fi
if (( files_count * 100 >= max_files * 80 || lines_count * 100 >= max_lines * 80 )); then
  echo "::warning::H1 admission budget is at least 80% used; review registered scope before adding more changes."
fi
if (( status == 0 )); then
  echo "OK: official zone untouched except registered hooks."
fi
exit $status
