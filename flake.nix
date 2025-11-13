{
  description = "keymerge - Deep merging for Go maps and slices";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_25
            golangci-lint
            goreleaser
            just
            # Additional tools for profiling
            graphviz  # for pprof visualizations
          ];

          shellHook = ''
            echo "keymerge development environment"
            echo "Go version: $(go version)"
            echo "Run 'just help' to see available commands"
          '';
        };

        # Optional: package the project as a Nix derivation
        packages.default = pkgs.buildGoModule {
          pname = "keymerge";
          version = "0.1.0";
          src = ./.;

          # This will need to be updated after first build
          # Run: nix build 2>&1 | grep "got:" to get the correct hash
          vendorHash = null;

          meta = with pkgs.lib; {
            description = "Deep merging for Go maps and slices";
            homepage = "https://github.com/sam-fredrickson/keymerge";
            license = licenses.unfree;  # Update this if you have a license
          };
        };
      }
    );
}
