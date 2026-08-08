# Compatibility shim for tools that don't support flakes yet
# Imports the devShell from flake.nix
(builtins.getFlake (toString ./.)).devShells.${builtins.currentSystem}.default
