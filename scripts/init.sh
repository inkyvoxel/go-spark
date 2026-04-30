#!/usr/bin/env bash
set -euo pipefail

TEMPLATE_MODULE="github.com/inkyvoxel/go-spark"
TEMPLATE_PROJECT="go-spark"
TEMPLATE_PROJECT_TITLE="Go Spark"

echo ""
echo "Welcome to go-spark. Let's set up your project."
echo ""

# Prompt for project name
while true; do
    read -rp "Project name (e.g. my-app): " PROJECT_NAME
    PROJECT_NAME="$(echo "$PROJECT_NAME" | xargs)"
    if [[ -n "$PROJECT_NAME" ]]; then
        break
    fi
    echo "  Project name cannot be empty."
done

# Prompt for Go module path
while true; do
    read -rp "Go module path (e.g. github.com/you/my-app): " MODULE_PATH
    MODULE_PATH="$(echo "$MODULE_PATH" | xargs)"
    if [[ -n "$MODULE_PATH" ]]; then
        break
    fi
    echo "  Module path cannot be empty."
done

echo ""
echo "  Project name : $PROJECT_NAME"
echo "  Module path  : $MODULE_PATH"
echo ""
read -rp "Apply these changes? [y/N] " CONFIRM
CONFIRM="$(echo "$CONFIRM" | tr '[:upper:]' '[:lower:]')"
if [[ "$CONFIRM" != "y" && "$CONFIRM" != "yes" ]]; then
    echo "Aborted."
    exit 0
fi

echo ""

# Derive a title-cased project title from the project name
# (replace hyphens/underscores with spaces, title-case each word)
PROJECT_TITLE="$(echo "$PROJECT_NAME" | sed 's/[-_]/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2); print}')"

OS="$(uname -s)"
# macOS sed requires an empty string for -i; GNU sed does not
if [[ "$OS" == "Darwin" ]]; then
    SED_INPLACE=(-i '')
else
    SED_INPLACE=(-i)
fi

replace_in_files() {
    local pattern="$1"
    local replacement="$2"
    # Find all text files tracked or present in the repo, excluding binary/generated paths
    find . \
        -type f \
        \( -name "*.go" -o -name "*.mod" -o -name "*.md" -o -name "*.html" -o -name "*.env*" -o -name "Makefile" -o -name "*.yaml" -o -name "*.yml" -o -name "*.sh" -o -name "*.css" \) \
        ! -path "./.git/*" \
        ! -path "./vendor/*" \
        -exec grep -lF "$pattern" {} \; \
        | xargs -r sed "${SED_INPLACE[@]}" "s|${pattern}|${replacement}|g"
}

echo "Updating module path..."
replace_in_files "$TEMPLATE_MODULE" "$MODULE_PATH"

echo "Updating project name..."
replace_in_files "$TEMPLATE_PROJECT_TITLE" "$PROJECT_TITLE"
replace_in_files "$TEMPLATE_PROJECT" "$PROJECT_NAME"

echo "Tidying Go modules..."
go mod tidy

echo ""
echo "Done! Next steps:"
echo ""
echo "  cp .env.example .env"
echo "  make migrate-up"
echo "  make start"
echo ""

# Self-destruct so init cannot be run again on an established project
rm -- "$0"
# Remove the init target from the Makefile now that the script is gone
sed "${SED_INPLACE[@]}" '/^init:/,/^\t@bash scripts\/init\.sh/d' Makefile
sed "${SED_INPLACE[@]}" 's/init //' Makefile
rmdir --ignore-fail-on-non-empty scripts 2>/dev/null || true
