DIST|channels|v1|distribution channels and CI workflows
PLATFORMS:
#platform|install-method|submodule
macos|brew install reh3376/mdemg/mdemg|submodule:packaging/homebrew-mdemg
linux-bin|curl installer or .deb|submodule:packaging/mdemg_linux
linux-apt|apt install mdemg|submodule:packaging/apt-mdemg
linux-arch|AUR PKGBUILD|dir:packaging/aur
windows|PowerShell installer|submodule:packaging/mdemg-windows
docker|docker-compose.yml|—
helm|deploy/helm/mdemg/|—

COMPANION-APPS:
#platform|technology|submodule|status
macos-companion|Swift menubar app|submodule:packaging/mdemg-menubar|status:implemented
linux-companion|Tauri sidebar:Rust+JS,theme:Catppuccin|submodule:packaging/mdemg-linux-sidebar|status:implemented
windows-companion|—|—|status:NOT-IMPLEMENTED→GAP-13

CI-WORKFLOWS:
#workflow|trigger|scope
ci.yml|push:main+dev|build,test,lint,UATS,security
parser-tests.yml|push:main|UPTS 27 languages
uxts-canonical-specs.yml|push:main|hash verification
release.yml|trigger:tag-push|goreleaser cross-compile→GitHub Release:all-platforms
apt-publish.yml|trigger:post-release|GPG-signed APT repo
auto-pr.yml|push:*_dev*|auto-create PR to main
branch-naming.yml|push|enforce handle_dev01-09 pattern
