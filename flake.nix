{
  description = "Vigie dev environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    flake-compat = {
      url = "github:edolstra/flake-compat";
      flake = false;
    };
  };

  outputs = { self, nixpkgs, flake-utils, flake-compat }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_26
            gnumake
            pre-commit
            git-cliff
            # release tooling
            goreleaser
            upx
          ];

          shellHook = ''
            export GOPATH="$HOME/go"
            export PATH="$GOPATH/bin:$PATH"

            if ! golangci-lint --version 2>/dev/null | grep -q 'v2\.11\.4'; then
              echo "Installing golangci-lint v2.11.4 (compiled with Go 1.26)..."
              go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4
            fi
          '';
        };
      });
}
