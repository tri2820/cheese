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
            pkg-config
            vulkan-headers
            vulkan-loader
            vulkan-tools
            vulkan-validation-layers
            shaderc
          ];
          # Set PKG_CONFIG_PATH for Vulkan
          PKG_CONFIG_PATH = "${pkgs.vulkan-loader}/lib/pkgconfig";
          LD_LIBRARY_PATH = "${pkgs.vulkan-loader}/lib";
          VK_LAYER_PATH = "${pkgs.vulkan-validation-layers}/share/vulkan/explicit_layer.d";
        };
      }
    );
}
