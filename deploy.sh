#!/bin/bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
DIST_DIR="$PROJECT_DIR/dist"
BINARY_NAME="assho"

# Timing
STEP_START=0
TOTAL_START=0

# ============================================================================
# UTILITIES
# ============================================================================

print_header() {
    clear
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║${NC}             ${BOLD}${CYAN}Assho Release Builder${NC}                         ${BLUE}║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

print_step() {
    local step=$1
    local total=$2
    local msg=$3
    STEP_START=$(date +%s)
    echo ""
    echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${CYAN}[$step/$total]${NC} ${BOLD}$msg${NC}"
    echo -e "${MAGENTA}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

print_substep() {
    echo -e "  ${DIM}→${NC} $1"
}

print_success() {
    local elapsed=$(($(date +%s) - STEP_START))
    echo -e "  ${GREEN}✓${NC} $1 ${DIM}(${elapsed}s)${NC}"
}

print_error() {
    echo -e "  ${RED}✗${NC} $1"
}

print_warning() {
    echo -e "  ${YELLOW}⚠${NC} $1"
}

print_info() {
    echo -e "  ${BLUE}ℹ${NC} $1"
}

print_file_size() {
    local file=$1
    if [ -f "$file" ]; then
        local size=$(du -h "$file" | cut -f1)
        local name=$(basename "$file")
        echo -e "  ${GREEN}✓${NC} ${name} ${DIM}(${size})${NC}"
    fi
}

format_time() {
    local seconds=$1
    if [ "$seconds" -ge 60 ]; then
        local mins=$((seconds / 60))
        local secs=$((seconds % 60))
        echo "${mins}m ${secs}s"
    else
        echo "${seconds}s"
    fi
}

spinner() {
    local pid=$1
    local msg=$2
    local spin='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local i=0

    tput civis  # Hide cursor
    while kill -0 "$pid" 2>/dev/null;
    do
        i=$(( (i + 1) % 10 ))
        printf "\r  ${CYAN}${spin:$i:1}${NC} %s" "$msg"
        sleep 0.1
    done
    tput cnorm  # Show cursor
    printf "\r"
}

# ============================================================================
# VERSION MANAGEMENT
# ============================================================================

read_version() {
    VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
}

suggest_next_patch() {
    NEXT_VERSION=""
    # Remove 'v' prefix if present for calculation
    local clean_version=${VERSION#v}
    if [[ $clean_version =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        NEXT_VERSION="v${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
    fi
}

bump_version() {
    read_version
    suggest_next_patch

    echo -e "\n  Current version: ${GREEN}$VERSION${NC}"

    if [ -n "$NEXT_VERSION" ]; then
        echo -e "  Suggested next:  ${CYAN}$NEXT_VERSION${NC}\n"
        read -p "  Use $NEXT_VERSION? [Y/n] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Nn]$ ]]; then
            VERSION="$NEXT_VERSION"
            print_success "Version set to $VERSION"
        else
            read -p "  Enter version (e.g. v1.2.3): " VERSION
            print_success "Version set to $VERSION"
        fi
    else
        read -p "  Enter version (e.g. v1.2.3): " VERSION
        print_success "Version set to $VERSION"
    fi
}

# ============================================================================
# BUILD FUNCTIONS
# ============================================================================

clean_dist() {
    print_substep "Removing old build artifacts..."
    rm -rf "$DIST_DIR"
    mkdir -p "$DIST_DIR"
    print_success "Cleaned dist folder"
}

build_all() {
    print_substep "Building cross-platform binaries..."
    
    local platforms=(
        "linux/amd64/assho-linux-amd64"
        "linux/arm64/assho-linux-arm64"
        "darwin/amd64/assho-darwin-amd64"
        "darwin/arm64/assho-darwin-arm64"
    )

    for plat in "${platforms[@]}"; do
        IFS="/" read -r os arch name <<< "$plat"
        GOOS=$os GOARCH=$arch go build -ldflags="-s -w" -o "$DIST_DIR/$name" . > /dev/null 2>&1 &
        local pid=$!
        spinner $pid "Building for $os/$arch..."
        wait $pid
        print_file_size "$DIST_DIR/$name"
    done
    
    print_success "All binaries built"
}

# ============================================================================
# GIT & RELEASE FUNCTIONS
# ============================================================================

commit_changes() {
    if [ -z "$(git status --porcelain)" ]; then
        print_info "No changes to commit"
    else
        print_substep "Staging and committing changes..."
        git add .
        git commit -m "chore: release $VERSION" > /dev/null
        print_success "Committed: release $VERSION"
    fi
}

push_to_origin() {
    print_substep "Pushing to origin..."
    git push origin main > /dev/null 2>&1 &
    local pid=$!
    spinner $pid "Pushing..."
    wait $pid && print_success "Pushed to origin"
}

create_release() {
    if ! command -v gh &> /dev/null; then
        print_error "GitHub CLI (gh) not installed"
        return 1
    fi

    # Handle existing tag
    if git rev-parse "$VERSION" &> /dev/null; then
        print_warning "Tag $VERSION already exists"
        read -p "  Delete and recreate? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git tag -d "$VERSION" > /dev/null
            git push origin --delete "$VERSION" 2>/dev/null || true
            print_success "Old tag deleted"
        else
            return 1
        fi
    fi

    print_substep "Creating and pushing tag $VERSION..."
    git tag -a "$VERSION" -m "Release $VERSION"
    git push origin "$VERSION" > /dev/null 2>&1
    print_success "Tag $VERSION pushed"

    print_substep "Creating GitHub release..."
    gh release create "$VERSION" \
        --title "Assho $VERSION" \
        --notes "Release $VERSION" \
        --repo allisonhere/assho > /dev/null 2>&1
    print_success "Release created"

    print_substep "Uploading assets..."
    for f in "$DIST_DIR"/*; do
        if [ -f "$f" ]; then
            gh release upload "$VERSION" "$f" --repo allisonhere/assho > /dev/null 2>&1
            print_file_size "$f"
        fi
    done
    print_success "Assets uploaded"
    echo -e "  ${GREEN}→${NC} https://github.com/allisonhere/assho/releases/tag/$VERSION"
}

full_release() {
    TOTAL_START=$(date +%s)
    local total_steps=6

    print_step 1 $total_steps "Version bump"
    bump_version

    print_step 2 $total_steps "Commit changes"
    commit_changes

    print_step 3 $total_steps "Push to origin"
    push_to_origin

    print_step 4 $total_steps "Cleaning dist"
    clean_dist

    print_step 5 $total_steps "Building binaries"
    build_all

    print_step 6 $total_steps "GitHub Release"
    create_release

    local total_time=$(($(date +%s) - TOTAL_START))
    echo -e "\n${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${GREEN}  ✓ Release $VERSION complete!${NC} ${DIM}($(format_time $total_time))${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

# ============================================================================
# MAIN MENU
# ============================================================================

show_status() {
    read_version
    suggest_next_patch
    echo -e "  ${BOLD}Version:${NC}  ${GREEN}$VERSION${NC}"
    [ -n "$NEXT_VERSION" ] && echo -e "  ${BOLD}Next:${NC}     ${DIM}$NEXT_VERSION${NC}"
    
    local changes=$(git status --porcelain | wc -l)
    if [ "$changes" -gt 0 ]; then
        echo -e "  ${BOLD}Git:${NC}      ${YELLOW}$changes uncommitted change(s)${NC}"
    else
        echo -e "  ${BOLD}Git:${NC}      ${GREEN}clean${NC}"
    fi
    echo ""
}

main_menu() {
    while true; do
        print_header
        show_status

        echo -e "  ${BOLD}${CYAN}Actions${NC}"
        echo -e "  ${DIM}─────────────────────────────${NC}"
        echo "   1) Bump version"
        echo "   2) Commit changes"
        echo "   3) Push to main"
        echo "   4) Clean dist"
        echo "   5) Build all binaries"
        echo ""
        echo -e "  ${BOLD}${CYAN}Release${NC}"
        echo -e "  ${DIM}─────────────────────────────${NC}"
        echo "   6) Create GitHub release only"
        echo -e "   7) ${GREEN}Full release (recommended)${NC}"
        echo ""
        echo "   0) Exit"
        echo ""

        read -p "  Choose [0-7]: " choice

        case $choice in
            1) bump_version ;;
            2) commit_changes ;;
            3) push_to_origin ;;
            4) clean_dist ;;
            5) build_all ;;
            6) create_release ;;
            7) full_release ;;
            0) echo -e "\n  ${DIM}Bye!${NC}\n"; exit 0 ;;
            *) print_error "Invalid choice" ;;
        esac

        echo ""
        read -p "  Press Enter to continue..." -r
    done
}

main_menu
