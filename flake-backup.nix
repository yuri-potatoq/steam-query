{
  description = "Go Projet Template";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        pname = "steam-query";
        version = "0.0.1";
        pkgs = import nixpkgs {
          inherit system;
        };

        tools = with pkgs; [
          # https://github.com/golang/vscode-go/blob/master/docs/tools.md
          delve
          go-outline
          golangci-lint
          gomodifytags
          gopls
          gotests
          impl

          # to dynamic linking
          # gcc main.c -o main $(pkg-config --cflags --libs libavformat libavcodec libavutil)
          # ffmpeg.dev
          # pkg-config
          # gcc
          
          # static linking 
          # gcc -static main.c -o main \$(pkg-config --static --cflags --libs libavformat libavcodec libavutil) -lpthread -lm -lz -llzma
          ffmpeg-static.dev
          pkg-config
          gcc
          glibc.static
        ];
      in
      rec {
        # `nix build`
        packages."${pname}" = pkgs.buildGoModule {
          inherit pname version;
          src = ./.;
          vendorSha256 = pkgs.lib.fakeSha256;
        };
        defaultPackage = packages."${pname}";

        # `nix run`
        apps."${pname}" = utils.lib.mkApp {
          drv = packages."${pname}";
        };
        defaultApp = apps."${pname}";

        # `nix develop`
        #devShell = with pkgs; mkShell {
          buildInputs = [ go ] ++ tools;
        };

        devShells.${system}.default = pkgs.mkShell {
          packages = with pkgs; [
            gcc
            pkg-config
            go
            (ffmpeg.override {
              shared = false;  # disable shared libs → static build
              gpl = true;
              withFdkAac = false;
              withOpenssl = false;
            })
          ];

          shellHook = ''
            echo "🔧 Static FFmpeg shell ready"
            export CGO_ENABLED=1
            export PKG_CONFIG_PATH=${pkgs.ffmpeg.dev}/lib/pkgconfig
          '';
        };
      });
}
