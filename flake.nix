{
  description = "Cheese - Pure Go Wayland toolkit and protocol generator";

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
          packages = with pkgs; [
            go
            gopls
            gotools
            wayland-protocols
            # For reference - we'll study these but reimplement
            # wayland-scanner  # Official C scanner
          ];

          shellHook = ''
            echo "Cheese development environment"
            echo "Go: $(go version)"
            echo "Protocols: ${pkgs.wayland-protocols}/share/wayland-protocols"
          '';
        };
      }
    );
}
