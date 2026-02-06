#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Asshi Deployment Helper${NC}"
echo "-----------------------"

# 1. Check for uncommitted changes
if [[ -n $(git status --porcelain) ]]; then
    echo -e "${RED}Warning: You have uncommitted changes.${NC}"
    git status --short
    read -p "Do you want to commit them now? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        read -p "Enter commit message: " msg
        git add .
        git commit -m "$msg"
        echo "Changes committed."
    else
        echo "Aborting deployment. Please clean your working directory."
        exit 1
    fi
fi

# 2. Pull latest
echo "Pulling latest changes..."
git pull origin main

# 3. Determine new version
LAST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo -e "Last version: ${GREEN}$LAST_TAG${NC}"

read -p "Enter new version tag (e.g., v0.1.2): " NEW_TAG

if [[ -z "$NEW_TAG" ]]; then
    echo "No version entered. Aborting."
    exit 1
fi

if [[ "$NEW_TAG" == "$LAST_TAG" ]]; then
    echo -e "${RED}Error: Version $NEW_TAG already exists.${NC}"
    exit 1
fi

# 4. Confirm
echo
echo "Ready to release:"
echo " - Commit any pending changes (Done)"
echo " - Push to origin/main"
echo " - Create tag: $NEW_TAG"
echo " - Push tag to trigger GitHub Action release"
echo

read -p "Proceed? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

# 5. Execute
echo "Pushing code..."
git push origin main

echo "Creating tag $NEW_TAG..."
git tag "$NEW_TAG"
git push origin "$NEW_TAG"

echo -e "${GREEN}Success! Release $NEW_TAG triggered.${NC}"
echo "Monitor build at: https://github.com/allisonhere/asshi/actions"
