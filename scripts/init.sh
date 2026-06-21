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
    # Escape sed replacement metacharacters (\, &, and the | delimiter) so a
    # user-supplied module path/name containing them isn't mangled. Backslash
    # must be escaped first.
    replacement="${replacement//\\/\\\\}"
    replacement="${replacement//&/\\&}"
    replacement="${replacement//|/\\|}"
    # Find all text files tracked or present in the repo, excluding binary/generated paths
    find . \
        -type f \
        \( -name "*.go" -o -name "*.mod" -o -name "*.md" -o -name "*.html" -o -name "*.env*" -o -name "Makefile" -o -name "*.yaml" -o -name "*.yml" -o -name "*.sh" -o -name "*.css" \) \
        ! -path "./.git/*" \
        ! -path "./vendor/*" \
        -exec grep -lF "$pattern" {} \; \
        | xargs -r sed "${SED_INPLACE[@]}" "s|${pattern}|${replacement}|g"
}

# Park the module path behind a placeholder first, then substitute the real
# value last. Otherwise the bare "go-spark" pass below would corrupt a module
# path that itself contains "go-spark" (e.g. github.com/me/go-spark-fork).
MODULE_PLACEHOLDER="@@GOSPARK_MODULE@@"

echo "Updating module path..."
replace_in_files "$TEMPLATE_MODULE" "$MODULE_PLACEHOLDER"

echo "Updating project name..."
replace_in_files "$TEMPLATE_PROJECT_TITLE" "$PROJECT_TITLE"
replace_in_files "$TEMPLATE_PROJECT" "$PROJECT_NAME"

replace_in_files "$MODULE_PLACEHOLDER" "$MODULE_PATH"

echo "Tidying Go modules..."
go mod tidy

# Create .env from the example and fill in the required secrets with random
# values so the app runs out of the box. An existing .env is left untouched.
ENV_GENERATED=false
if [[ ! -f .env ]]; then
    cp .env.example .env
    if command -v openssl >/dev/null 2>&1; then
        for key in SECRET_KEY_BASE AUTH_TOTP_KEY AUTH_PASSWORD_PEPPER; do
            secret="$(openssl rand -hex 32)"
            sed "${SED_INPLACE[@]}" "s|^${key}=.*|${key}=${secret}|" .env
        done
        ENV_GENERATED=true
        echo "Created .env with freshly generated secrets."
    else
        echo "Created .env, but openssl was not found — set SECRET_KEY_BASE, AUTH_TOTP_KEY,"
        echo "and AUTH_PASSWORD_PEPPER manually (generate each with: openssl rand -hex 32)."
    fi
fi

echo ""
echo "Done! Next steps:"
echo ""
if [[ "$ENV_GENERATED" == "true" ]]; then
    echo "  # .env was created with randomly generated secrets — change them in .env if you like"
else
    echo "  # set the required secrets in .env (SECRET_KEY_BASE, AUTH_TOTP_KEY, AUTH_PASSWORD_PEPPER)"
fi
echo "  make migrate-up"
echo "  make start"
echo ""

# Self-destruct so init cannot be run again on an established project
rm -- "$0"
# Remove the init target from the Makefile now that the script is gone
sed "${SED_INPLACE[@]}" '/^init:/,/^$/d' Makefile
sed "${SED_INPLACE[@]}" 's/init //' Makefile
rmdir --ignore-fail-on-non-empty scripts 2>/dev/null || true
