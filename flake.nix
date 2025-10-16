{
  description = "Steam Query - Go application with FFmpeg";

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
          default = pkgs.stdenv.mkDerivation {
            pname = "steam-query";
            version = "0.1.0";

            # Use path instead of ./. to include all files
            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: type:
                let baseName = baseNameOf path;
                in !(baseName == "flake.nix" || baseName == "flake.lock" || baseName == "result");
            };

            nativeBuildInputs = with pkgs; [
              go
              pkg-config
            ];

            buildInputs = with pkgs; [
              ffmpeg.dev
            ];

            buildPhase = ''
              echo "=== Starting build phase ==="
              
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/go-cache
              export GOPATH=$TMPDIR/go
              export GO111MODULE=off
              export CGO_ENABLED=1
              export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
              export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
              
              echo "Files in source directory:"
              ls -la
              
              # Copy source to GOPATH structure
              mkdir -p $GOPATH/src/steam-query
              echo "Copying source files..."
              cp -r * $GOPATH/src/steam-query/ 2>/dev/null || true
              cd $GOPATH/src/steam-query
              
              echo "Files copied to GOPATH:"
              ls -la
              
              echo "Building with CGO in GOPATH mode..."
              go build -v -ldflags "-s -w" -o steam-query . || {
                echo "Build failed!"
                exit 1
              }
              
              echo "Build successful!"
              ls -la steam-query
            '';

            installPhase = ''
              echo "=== Starting install phase ==="
              
              mkdir -p $out/bin
              cp $GOPATH/src/steam-query/steam-query $out/bin/
              echo "Binary installed to $out/bin/steam-query"
            '';

            meta = with pkgs.lib; {
              description = "Steam Query application";
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            pkg-config
            ffmpeg.dev
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