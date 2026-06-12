DIST|channels|v2|distribution channels and CI workflows (submodules: .gitmodules has ONLY packaging/homebrew-mdemg + packaging/autoresearch)
PLATFORMS:
#platform|install-method|source
macos|brew install reh3376/mdemg/mdemg|submodule:packaging/homebrew-mdemg
macos+linux-bin|curl scripts/install.sh (SHA256-verifying)|main repo:scripts/install.sh; goreleaser builds darwin+linux on arm64+amd64
linux-arch|AUR PKGBUILD|dir:packaging/aur
windows|WSL2 + linux path|no native installer (no separate installer repo)
docker|mdemg init→docker-compose.yml; ghcr.io/reh3376/mdemg:latest|dir:internal/cli/compose_templates + .github/workflows/docker-publish.yml
helm|deploy/helm/mdemg/|main repo
model|mdemg model pull (Ollama Library reh3376/mdemg-llm-v1)|dir:packaging/ollama (MODEL-DIST-001/002)

COMPANION-APPS:
#platform|status
all|NONE in this repo (no packaging/mdemg-menubar or -linux-sidebar submodules exist)

CI-WORKFLOWS:
#workflow|trigger|scope
ci.yml|push:main+dev|build,test,lint,UATS,UNTS,USTS,UOTS,ULTS,UBENCH,config+route consumer guards,security
parser-tests.yml|push|UPTS 28 languages
uxts-canonical-specs.yml|push|hash verification
release.yml|trigger:tag-push|goreleaser cross-compile→GitHub Release (darwin+linux × arm64+amd64)
docker-publish.yml|push:main|ghcr.io image
auto-pr.yml|push:*_dev*|auto-create PR to main
branch-naming.yml|push|enforce handle_dev01-09 pattern
cli-publish.yml,codeql.yml,claude*.yml|various|CLI publish,CodeQL,review automation
sync-dev-after-merge.yml|PR merge|merge main back into source dev branch
