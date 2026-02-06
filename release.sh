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

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$PROJECT_DIR/dist-electron"
ARCH_DIR="$PROJECT_DIR/packaging/arch"
AUR_DIR="$HOME/aur-alogi"

# Timing
STEP_START=0
TOTAL_START=0

# ============================================================================
# UTILITIES
# ============================================================================

print_header() {
    clear
    echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║${NC}             ${BOLD}${CYAN}Alogi Release Builder${NC}                        ${BLUE}║${NC}"
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
    while kill -0 "$pid" 2>/dev/null; do
        i=$(( (i + 1) % 10 ))
        printf "\r  ${CYAN}${spin:$i:1}${NC} %s" "$msg"
        sleep 0.1
    done
    tput cnorm  # Show cursor
    printf "\r"
}

run_with_spinner() {
    local msg=$1
    shift

    "$@" > /tmp/release_cmd_output.log 2>&1 &
    local pid=$!
    spinner $pid "$msg"
    wait $pid
    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        print_success "$msg"
    else
        print_error "$msg"
        echo -e "${DIM}"
        tail -20 /tmp/release_cmd_output.log
        echo -e "${NC}"
        return $exit_code
    fi
}

# ============================================================================
# VERSION MANAGEMENT
# ============================================================================

read_version() {
    if [ ! -f "$PROJECT_DIR/package.json" ]; then
        print_error "package.json not found"
        exit 1
    fi
    VERSION=$(node -p "require('$PROJECT_DIR/package.json').version" 2>/dev/null) || {
        print_error "Unable to read version from package.json"
        exit 1
    }
    if [ -z "$VERSION" ]; then
        print_error "package.json version is empty"
        exit 1
    fi
}

suggest_next_patch() {
    NEXT_VERSION=""
    if [[ $VERSION =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        NEXT_VERSION="${BASH_REMATCH[1]}.${BASH_REMATCH[2]}.$((BASH_REMATCH[3] + 1))"
    fi
}

add_changelog_entry() {
    if [ ! -f "$PROJECT_DIR/CHANGELOG.md" ]; then
        print_warning "CHANGELOG.md not found; skipping"
        return 0
    fi
    if grep -q "^## \\[$VERSION\\]" "$PROJECT_DIR/CHANGELOG.md"; then
        print_info "CHANGELOG already has entry for $VERSION"
        return 0
    fi

    local today=$(date +%F)
    local entry="## [$VERSION] - $today\n\n### Added\n- TBD\n\n### Changed\n- TBD\n\n### Fixed\n- TBD\n"
    local tmp=$(mktemp)
    awk -v entry="$entry" '
        BEGIN { inserted=0 }
        /^## \[/ && inserted==0 { print entry; inserted=1 }
        { print }
        END { if (inserted==0) print entry }
    ' "$PROJECT_DIR/CHANGELOG.md" > "$tmp" && mv "$tmp" "$PROJECT_DIR/CHANGELOG.md"
    print_success "Added CHANGELOG entry for v$VERSION"
    print_warning "Remember to replace TBD entries!"
}

bump_version() {
    read_version
    suggest_next_patch

    echo -e "\n  Current version: ${GREEN}v$VERSION${NC}"

    if [ -n "$NEXT_VERSION" ]; then
        echo -e "  Suggested next:  ${CYAN}v$NEXT_VERSION${NC}\n"
        read -p "  Bump to v$NEXT_VERSION? [Y/n] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Nn]$ ]]; then
            npm -C "$PROJECT_DIR" version "$NEXT_VERSION" --no-git-tag-version > /dev/null
            read_version
            print_success "Version bumped to v$VERSION"
        else
            bump_version_advanced
        fi
    else
        bump_version_advanced
    fi

    read_version
    echo ""
    read -p "  Add CHANGELOG entry? [Y/n] " -n 1 -r
    echo
    [[ ! $REPLY =~ ^[Nn]$ ]] && add_changelog_entry
}

bump_version_advanced() {
    echo -e "\n  ${BOLD}Select version bump:${NC}"
    echo "    1) patch  (x.x.X)"
    echo "    2) minor  (x.X.0)"
    echo "    3) major  (X.0.0)"
    echo "    4) custom"
    echo "    5) cancel"
    read -p "  Choose [1-5]: " bump_choice

    case $bump_choice in
        1) npm -C "$PROJECT_DIR" version patch --no-git-tag-version > /dev/null ;;
        2) npm -C "$PROJECT_DIR" version minor --no-git-tag-version > /dev/null ;;
        3) npm -C "$PROJECT_DIR" version major --no-git-tag-version > /dev/null ;;
        4)
            read -p "  Enter version (e.g. 1.2.3): " custom_version
            [ -z "$custom_version" ] && { print_error "Version cannot be empty"; return 1; }
            npm -C "$PROJECT_DIR" version "$custom_version" --no-git-tag-version > /dev/null
            ;;
        5) return 0 ;;
        *) print_error "Invalid choice"; return 1 ;;
    esac
    read_version
    print_success "Version set to v$VERSION"
}

# ============================================================================
# BUILD FUNCTIONS
# ============================================================================

clean_builds() {
    print_substep "Removing old build artifacts..."
    rm -rf "$DIST_DIR/linux-unpacked"
    rm -f "$DIST_DIR"/*.deb
    rm -f "$DIST_DIR"/alogi-*-linux-unpacked.tar.gz
    rm -f "$DIST_DIR"/*.tar.gz
    rm -f "$DIST_DIR"/*.pkg.tar.zst
    rm -f "$ARCH_DIR"/*.pkg.tar.zst
    rm -f "$ARCH_DIR/linux-unpacked.tar.gz"
    rm -rf "$ARCH_DIR/pkg" "$ARCH_DIR/src"
    rm -rf "$PROJECT_DIR/.next/standalone" "$PROJECT_DIR/.next/trace"
    print_success "Cleaned old builds"
}

clear_yay_cache() {
    print_substep "Clearing yay cache for alogi..."
    rm -rf "$HOME/.cache/yay/alogi"
    print_success "Yay cache cleared"
}

build_nextjs() {
    print_substep "Running Next.js build..."
    cd "$PROJECT_DIR"

    npm run build > /tmp/nextjs_build.log 2>&1 &
    local pid=$!
    spinner $pid "Building Next.js + patching Turbopack..."
    wait $pid || {
        print_error "Next.js build failed"
        tail -30 /tmp/nextjs_build.log
        return 1
    }
    print_success "Next.js build complete"
}

build_deb() {
    print_substep "Building .deb package..."
    cd "$PROJECT_DIR"

    local eb_cmd="$PROJECT_DIR/node_modules/.bin/electron-builder"
    [ ! -x "$eb_cmd" ] && { print_error "electron-builder not found"; return 1; }

    "$eb_cmd" --linux deb --publish never > /tmp/deb_build.log 2>&1 &
    local pid=$!
    spinner $pid "Packaging .deb..."
    wait $pid || {
        print_error "Deb build failed"
        tail -20 /tmp/deb_build.log
        return 1
    }

    print_file_size "$DIST_DIR/Alogi-amd64.deb"
}

build_arch() {
    print_substep "Building linux-unpacked directory..."
    cd "$PROJECT_DIR"

    local eb_cmd="$PROJECT_DIR/node_modules/.bin/electron-builder"
    "$eb_cmd" --linux dir --publish never > /tmp/arch_build.log 2>&1 &
    local pid=$!
    spinner $pid "Packaging linux-unpacked..."
    wait $pid || {
        print_error "Linux dir build failed"
        tail -20 /tmp/arch_build.log
        return 1
    }
    print_success "linux-unpacked built"

    print_substep "Creating tarball..."
    tar -C "$DIST_DIR" -czf "$ARCH_DIR/linux-unpacked.tar.gz" linux-unpacked
    cp "$ARCH_DIR/linux-unpacked.tar.gz" "$DIST_DIR/alogi-$VERSION-linux-unpacked.tar.gz"
    print_file_size "$DIST_DIR/alogi-$VERSION-linux-unpacked.tar.gz"

    print_substep "Updating PKGBUILD version..."
    sed -i "s/^pkgver=.*/pkgver=$VERSION/" "$ARCH_DIR/PKGBUILD"
    print_success "PKGBUILD updated to v$VERSION"

    print_substep "Running makepkg..."
    rm -rf "$ARCH_DIR/pkg" "$ARCH_DIR/src"
    cd "$ARCH_DIR"

    makepkg -f > /tmp/makepkg.log 2>&1 &
    local pid=$!
    spinner $pid "Building Arch package..."
    wait $pid || {
        print_error "makepkg failed"
        tail -20 /tmp/makepkg.log
        return 1
    }

    cp "$ARCH_DIR"/alogi-*.pkg.tar.zst "$DIST_DIR/" 2>/dev/null || true
    print_file_size "$DIST_DIR"/alogi-*.pkg.tar.zst
}

# ============================================================================
# GIT & RELEASE FUNCTIONS
# ============================================================================

commit_changes() {
    if [ -z "$(git -C "$PROJECT_DIR" status --porcelain)" ]; then
        print_info "No changes to commit"
        return 0
    fi

    echo ""
    git -C "$PROJECT_DIR" status --short
    echo ""

    local default_msg="chore: release v$VERSION"
    read -p "  Commit message [$default_msg]: " msg
    msg=${msg:-$default_msg}

    git -C "$PROJECT_DIR" add -A
    git -C "$PROJECT_DIR" commit -m "$msg" > /dev/null && print_success "Changes committed"
}

push_changes() {
    print_substep "Pushing to origin..."
    git -C "$PROJECT_DIR" push origin main > /dev/null 2>&1 &
    local pid=$!
    spinner $pid "Pushing commits..."
    wait $pid && print_success "Pushed to origin"
}

auto_commit_and_push() {
    local changes=$(git -C "$PROJECT_DIR" status --porcelain | wc -l)

    if [ "$changes" -gt 0 ]; then
        print_substep "Staging $changes changed file(s)..."
        git -C "$PROJECT_DIR" add -A
        print_success "Files staged"

        print_substep "Committing..."
        git -C "$PROJECT_DIR" commit -m "chore: release v$VERSION" > /dev/null
        print_success "Committed: release v$VERSION"
    else
        print_info "No changes to commit"
    fi

    print_substep "Pushing to origin..."
    git -C "$PROJECT_DIR" push origin main > /dev/null 2>&1 &
    local pid=$!
    spinner $pid "Pushing..."
    wait $pid && print_success "Pushed to origin"
}

create_github_release() {
    # Verify gh CLI
    if ! command -v gh &> /dev/null; then
        print_error "GitHub CLI (gh) not installed"
        return 1
    fi
    if ! gh auth status &> /dev/null 2>&1; then
        print_error "Not logged into GitHub CLI. Run 'gh auth login'"
        return 1
    fi

    local TAG="v$VERSION"

    # Handle existing tag
    if git -C "$PROJECT_DIR" rev-parse "$TAG" &> /dev/null; then
        print_warning "Tag $TAG already exists"
        read -p "  Delete and recreate? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git -C "$PROJECT_DIR" tag -d "$TAG" > /dev/null
            git -C "$PROJECT_DIR" push origin --delete "$TAG" 2>/dev/null || true
            print_success "Old tag deleted"
        else
            return 1
        fi
    fi

    # Create and push tag
    print_substep "Creating tag $TAG..."
    git -C "$PROJECT_DIR" tag -a "$TAG" -m "Release $TAG"
    git -C "$PROJECT_DIR" push origin "$TAG" > /dev/null 2>&1
    print_success "Tag $TAG pushed"

    # Get changelog notes
    local NOTES=""
    if [ -f "$PROJECT_DIR/CHANGELOG.md" ]; then
        NOTES=$(awk -v ver="$VERSION" '
            $0 ~ "^## \\["ver"\\]" {found=1; next}
            found && $0 ~ "^## \\[" {exit}
            found {print}
        ' "$PROJECT_DIR/CHANGELOG.md")
    fi
    [ -z "$NOTES" ] && NOTES="Release $TAG"

    # Create release
    print_substep "Creating GitHub release..."
    gh release create "$TAG" \
        --title "Alogi $TAG" \
        --notes "$NOTES" \
        --repo allisonhere/alogi > /dev/null 2>&1
    print_success "Release created"

    # Upload assets
    print_substep "Uploading assets..."

    local uploaded=0
    if [ -f "$DIST_DIR/Alogi-amd64.deb" ]; then
        gh release upload "$TAG" "$DIST_DIR/Alogi-amd64.deb" --repo allisonhere/alogi > /dev/null 2>&1
        print_file_size "$DIST_DIR/Alogi-amd64.deb"
        ((uploaded++)) || true
    fi

    local TARBALL="$DIST_DIR/alogi-$VERSION-linux-unpacked.tar.gz"
    if [ -f "$TARBALL" ]; then
        gh release upload "$TAG" "$TARBALL" --repo allisonhere/alogi > /dev/null 2>&1
        print_file_size "$TARBALL"
        ((uploaded++)) || true
    fi

    local ARCH_PKG=$(ls "$DIST_DIR"/alogi-*.pkg.tar.zst 2>/dev/null | head -1)
    if [ -n "$ARCH_PKG" ] && [ -f "$ARCH_PKG" ]; then
        gh release upload "$TAG" "$ARCH_PKG#alogi-arch.pkg.tar.zst" --repo allisonhere/alogi > /dev/null 2>&1
        print_file_size "$ARCH_PKG"
        ((uploaded++)) || true
    fi

    print_success "Uploaded $uploaded assets"
    echo ""
    echo -e "  ${GREEN}→${NC} https://github.com/allisonhere/alogi/releases/tag/$TAG"
}

update_aur() {
    if [ ! -d "$AUR_DIR" ]; then
        print_error "AUR directory not found at $AUR_DIR"
        print_info "Clone it first: git clone ssh://aur@aur.archlinux.org/alogi.git ~/aur-alogi"
        return 1
    fi

    # Always fetch SHA from GitHub to ensure consistency (handles CI-built tarballs)
    local GITHUB_TARBALL="https://github.com/allisonhere/alogi/releases/download/v${VERSION}/alogi-${VERSION}-linux-unpacked.tar.gz"

    print_substep "Fetching SHA256 from GitHub release..."
    local SHA256=$(curl -sL "$GITHUB_TARBALL" | sha256sum | awk '{print $1}')
    if [ -z "$SHA256" ] || [ "$SHA256" = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]; then
        print_error "Failed to fetch tarball from GitHub (empty or not found)"
        print_info "Make sure the release exists: $GITHUB_TARBALL"
        return 1
    fi
    print_success "SHA256: ${SHA256:0:16}..."

    print_substep "Pulling latest from AUR..."
    cd "$AUR_DIR"
    if [ -n "$(git status --porcelain)" ]; then
        print_warning "AUR repo has local changes, stashing before pull"
        git stash -u -m "alogi-release-auto-stash" > /dev/null 2>&1
    fi
    git pull > /dev/null 2>&1
    print_success "AUR repo updated"

    print_substep "Syncing AUR packaging files..."
    local AUR_SRC="$PROJECT_DIR/packaging/arch/aur"
    if [ -f "$AUR_SRC/PKGBUILD" ]; then
        cp -f "$AUR_SRC/PKGBUILD" "$AUR_DIR/PKGBUILD"
    fi
    if [ -f "$AUR_SRC/README.md" ]; then
        cp -f "$AUR_SRC/README.md" "$AUR_DIR/README.md"
    fi
    if [ -f "$AUR_SRC/alogi.desktop" ]; then
        cp -f "$AUR_SRC/alogi.desktop" "$AUR_DIR/alogi.desktop"
    elif [ -f "$ARCH_DIR/alogi.desktop" ]; then
        cp -f "$ARCH_DIR/alogi.desktop" "$AUR_DIR/alogi.desktop"
    fi
    if [ -f "$AUR_SRC/icon.png" ]; then
        cp -f "$AUR_SRC/icon.png" "$AUR_DIR/icon.png"
    fi
    print_success "AUR packaging files synced"

    print_substep "Updating PKGBUILD..."
    sed -i "s/^pkgver=.*/pkgver=$VERSION/" PKGBUILD
    SHA256="$SHA256" python3 - <<'PY'
import os
import re
from pathlib import Path

path = Path("PKGBUILD")
text = path.read_text()
sha = os.environ["SHA256"]

def replace_first_sha(match: re.Match) -> str:
    return f'{match.group(1)}  "{sha}"'

new_text = re.sub(r'(sha256sums=\(\s*)\"[^\"]*\"', replace_first_sha, text, count=1)
if new_text == text:
    raise SystemExit("Failed to update sha256sums in PKGBUILD")
path.write_text(new_text)
PY
    print_success "PKGBUILD updated"

    print_substep "Generating .SRCINFO..."
    makepkg --printsrcinfo > .SRCINFO
    print_success ".SRCINFO generated"

    print_substep "Committing and pushing..."
    add_files=(PKGBUILD .SRCINFO)
    [ -f alogi.desktop ] && add_files+=(alogi.desktop)
    [ -f icon.png ] && add_files+=(icon.png)
    [ -f README.md ] && add_files+=(README.md)
    git add "${add_files[@]}"
    git commit -m "Update to $VERSION" > /dev/null 2>&1
    git push > /dev/null 2>&1
    print_success "Pushed to AUR"

    echo ""
    echo -e "  ${GREEN}→${NC} https://aur.archlinux.org/packages/alogi"
}

# ============================================================================
# FULL RELEASE WORKFLOW
# ============================================================================

full_release() {
    TOTAL_START=$(date +%s)
    local total_steps=8

    print_step 1 $total_steps "Version bump"
    bump_version

    print_step 2 $total_steps "Cleaning old builds"
    clean_builds

    print_step 3 $total_steps "Building Next.js"
    build_nextjs

    print_step 4 $total_steps "Building .deb package"
    build_deb

    print_step 5 $total_steps "Building Arch package"
    build_arch

    print_step 6 $total_steps "Committing & pushing"
    auto_commit_and_push

    print_step 7 $total_steps "Creating GitHub release"
    create_github_release

    print_step 8 $total_steps "Updating AUR"
    update_aur

    # Summary
    local total_time=$(($(date +%s) - TOTAL_START))
    echo ""
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${GREEN}  ✓ Release v$VERSION complete!${NC} ${DIM}($(format_time $total_time))${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "  ${BOLD}Artifacts:${NC}"
    print_file_size "$DIST_DIR/Alogi-amd64.deb"
    print_file_size "$DIST_DIR/alogi-$VERSION-linux-unpacked.tar.gz"
    ls "$DIST_DIR"/alogi-*.pkg.tar.zst 2>/dev/null | while read f; do print_file_size "$f"; done
    echo ""
    echo -e "  ${BOLD}Links:${NC}"
    echo -e "  ${CYAN}→${NC} GitHub: https://github.com/allisonhere/alogi/releases/tag/v$VERSION"
    echo -e "  ${CYAN}→${NC} AUR:    https://aur.archlinux.org/packages/alogi"
    echo ""
}

# ============================================================================
# MAIN MENU
# ============================================================================

show_status() {
    read_version
    suggest_next_patch

    echo -e "  ${BOLD}Version:${NC}  ${GREEN}v$VERSION${NC}"
    [ -n "$NEXT_VERSION" ] && echo -e "  ${BOLD}Next:${NC}     ${DIM}v$NEXT_VERSION${NC}"

    # Disk space
    local avail=$(df -BG "$PROJECT_DIR" | tail -1 | awk '{print $4}' | sed 's/G//')
    local disk_color=$GREEN
    [ "$avail" -lt 10 ] && disk_color=$YELLOW
    [ "$avail" -lt 5 ] && disk_color=$RED
    echo -e "  ${BOLD}Disk:${NC}     ${disk_color}${avail}GB free${NC}"

    # Git status
    local changes=$(git -C "$PROJECT_DIR" status --porcelain | wc -l)
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
        echo "   3) Build all (deb + Arch)"
        echo "   4) Build deb only"
        echo "   5) Build Arch only"
        echo "   6) Clean builds"
        echo "  10) Clear yay cache (alogi)"
        echo ""
        echo -e "  ${BOLD}${CYAN}Release${NC}"
        echo -e "  ${DIM}─────────────────────────────${NC}"
        echo "   7) GitHub release only"
        echo "   8) AUR update only"
        echo -e "   9) ${GREEN}Full release (recommended)${NC}"
        echo ""
        echo "   0) Exit"
        echo ""

        read -p "  Choose [0-10]: " choice

        case $choice in
            1) bump_version ;;
            2) commit_changes ;;
            3)
                print_step 1 3 "Cleaning"; clean_builds
                print_step 2 3 "Building Next.js"; build_nextjs
                print_step 3 3 "Building packages"; build_deb; build_arch
                ;;
            4)
                print_step 1 2 "Cleaning"; clean_builds
                print_step 2 2 "Building"; build_nextjs; build_deb
                ;;
            5)
                print_step 1 1 "Building Arch"; build_arch
                ;;
            6) clean_builds ;;
            10) clear_yay_cache ;;
            7) create_github_release ;;
            8) update_aur ;;
            9) full_release ;;
            0) echo -e "\n  ${DIM}Bye!${NC}\n"; exit 0 ;;
            *) print_error "Invalid choice" ;;
        esac

        echo ""
        read -p "  Press Enter to continue..." -r
    done
}

# Run
main_menu
