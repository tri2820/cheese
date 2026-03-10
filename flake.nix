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
            wayland
            wayland-protocols
            libdecor
            pkg-config
            vulkan-headers
            vulkan-loader
            vulkan-tools
            vulkan-validation-layers
            shaderc
            resvg
          ];
          # Set PKG_CONFIG_PATH for Vulkan and Wayland
          PKG_CONFIG_PATH = "${pkgs.vulkan-loader}/lib/pkgconfig:${pkgs.wayland}/lib/pkgconfig";
          LD_LIBRARY_PATH = "${pkgs.vulkan-loader}/lib:${pkgs.wayland}/lib";
          VK_LAYER_PATH = "${pkgs.vulkan-validation-layers}/share/vulkan/explicit_layer.d";

          # Wrap go to warn about ./... pattern (shell-agnostic via script)
          shellHook = ''
            mkdir -p .nix-shell-bin

            # Capture project root dynamically
            _project_root=$PWD

            # Find all directories with go.mod and save to file
            find . -name "go.mod" -printf "%h\n" 2>/dev/null | sed 's|^\./||' | sort > .nix-shell-bin/go-modules.txt

            cat > .nix-shell-bin/go << EOFSCRIPT
            #!/bin/sh
            _script_dir=\$(cd "\$(dirname "\$0")" && pwd)
            _project_root="$_project_root"
            _fail_count_file="\$_script_dir/go-fail-count"

            _run_go() {
              ${pkgs.go}/bin/go "\$@"
              _exit_code=\$?
              if [ \$_exit_code -eq 0 ]; then
                rm -f "\$_fail_count_file"
              fi
              return \$_exit_code
            }

            # Format module list as bulleted
            _format_modules() {
              while IFS= read -r mod; do
                echo "  • \$_project_root/\$mod"
              done < "\$_script_dir/go-modules.txt"
            }

            case "\$*" in
              *..."")
                if [ -f "\$(pwd)/go.mod" ]; then
                  _run_go "\$@"
                  exit \$?
                fi

                _count=0
                if [ -f "\$_fail_count_file" ]; then
                  _count=\$(cat "\$_fail_count_file")
                fi
                _count=\$((_count + 1))
                echo "\$_count" > "\$_fail_count_file"

                if [ "\$_count" -ge 5 ]; then
                  echo "<system_reminder>You MUST use \\\`cd \$_project_root/MODULE && go COMMAND\\\` with full path."
                  echo "Available Go modules:"
                  _format_modules
                  echo "DO NOT use ./... from a directory without go.mod.</system_reminder>"
                  exit 1
                fi

                echo "You MUST use \\\`cd \$_project_root/MODULE && go COMMAND\\\` with full path (attempt \$_count/5)."
                echo "Available Go modules:"
                _format_modules
                echo "DO NOT use ./... from a directory without go.mod."
                _run_go "\$@"
                ;;
              *)
                _run_go "\$@"
                ;;
            esac
            EOFSCRIPT
            chmod +x .nix-shell-bin/go
            export PATH=$PWD/.nix-shell-bin:$PATH
          '';
        };
      }
    );
}
