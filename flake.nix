{
  description = "Steam Query";

  nixConfig = {
    extra-substituters = [
      "https://cache.nixos.org"
      "https://nix-community.cachix.org"
    ];
    extra-trusted-public-keys = [
      "cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY="
      "nix-community.cachix.org-1:mB9FSh9qf2dCimDSUo8Zy7bkq5CX+/rkCWyvRCYg3Fs="
    ];
    # Allow unfree packages (some FFmpeg codecs might be unfree)
    allowUnfree = true;
  };

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
        packages = {
          # just run with nix run .# -- -game-page <url>
          default = pkgs.callPackage ./nix/steam-query.nix { inherit pkgs; srcFiles = ./.;};
        };

        devShells.default = with pkgs; mkShell {
          buildInputs = [            
            # https://github.com/golang/vscode-go/blob/master/docs/tools.md
            delve
            go-outline
            golangci-lint
            gomodifytags
            gopls
            gotests
            impl
            go
            
            pkg-config
            ffmpeg-headless.dev
            ffmpeg-headless
          ];

          shellHook = ''
            export CGO_ENABLED=1            
            export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
            export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
          '';
        };
      }
    );
}
