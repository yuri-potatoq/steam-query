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
        ffmpeg-minimal = pkgs.ffmpeg-headless;
      in
      {
        packages = {
          # Standard dynamic binary (works only on NixOS or with Nix installed)
          default = pkgs.buildGoModule {
            pname = "steam-query";
            version = "0.1.0";

            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: type:
                let baseName = baseNameOf path;
                in !(baseName == "result");
            };

            vendorHash = "sha256-XBo8pZGzvma8AM/KJdVI30E39ho+T2+daBxNsmvJeHI=";

            nativeBuildInputs = with pkgs; [
              pkg-config
            ];

            buildInputs = [
              ffmpeg-minimal.dev
            ];

            preBuild = ''
              export CGO_ENABLED=1
              export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
              export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
            '';

            ldflags = [ "-s" "-w" ];

            meta = with pkgs.lib; {
              description = "Steam Query application (Nix dynamic)";
            };
          };

          # Portable binary with bundled libraries (works on any x86_64 Linux)
          portable = pkgs.stdenv.mkDerivation {
            pname = "steam-query-portable";
            version = "0.1.0";

            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: type:
                let baseName = baseNameOf path;
                in !(baseName == "result" || baseName == "flake.nix" || baseName == "flake.lock");
            };

            nativeBuildInputs = with pkgs; [
              go
              pkg-config
              autoPatchelfHook
              makeWrapper
            ];

            buildInputs = [
              ffmpeg-minimal
              pkgs.stdenv.cc.cc.lib
            ];

            buildPhase = ''
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/go-cache
              export GOPATH=$TMPDIR/go
              export GO111MODULE=on
              export CGO_ENABLED=1
              export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
              export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
              
              echo "Building portable binary..."
              go build -v -ldflags "-s -w" -o steam-query .
            '';

            installPhase = ''
              mkdir -p $out/bin $out/lib
              
              # Copy the binary
              cp steam-query $out/bin/
              
              # Copy FFmpeg libraries and their dependencies
              echo "Collecting FFmpeg libraries..."
              for lib in ${ffmpeg-minimal}/lib/*.so*; do
                if [ -f "$lib" ]; then
                  cp -L "$lib" $out/lib/
                fi
              done
              
              # Collect all transitive dependencies
              echo "Collecting transitive dependencies..."
              for lib in $(ldd ${ffmpeg-minimal}/lib/libavformat.so* | grep "=> /" | awk '{print $3}'); do
                cp -L "$lib" $out/lib/ 2>/dev/null || true
              done
              
              for lib in $(ldd ${ffmpeg-minimal}/lib/libavcodec.so* | grep "=> /" | awk '{print $3}'); do
                cp -L "$lib" $out/lib/ 2>/dev/null || true
              done
              
              for lib in $(ldd ${ffmpeg-minimal}/lib/libavutil.so* | grep "=> /" | awk '{print $3}'); do
                cp -L "$lib" $out/lib/ 2>/dev/null || true
              done
              
              # Set RPATH to look in the lib directory relative to binary
              patchelf --set-rpath '$ORIGIN/../lib' $out/bin/steam-query
              
              # Create a wrapper script that sets LD_LIBRARY_PATH
              mv $out/bin/steam-query $out/bin/.steam-query-wrapped
              makeWrapper $out/bin/.steam-query-wrapped $out/bin/steam-query \
                --prefix LD_LIBRARY_PATH : $out/lib
              
              echo "Portable package created with $(ls $out/lib | wc -l) libraries"
            '';

            meta = with pkgs.lib; {
              description = "Steam Query application (portable with bundled libs)";
            };
          };

          # Musl static (most portable, single binary)
          musl-static = pkgs.pkgsMusl.buildGoModule {
            pname = "steam-query-musl";
            version = "0.1.0";

            src = pkgs.lib.cleanSourceWith {
              src = ./.;
              filter = path: type:
                let baseName = baseNameOf path;
                in !(baseName == "result");
            };

            vendorHash = "sha256-XBo8pZGzvma8AM/KJdVI30E39ho+T2+daBxNsmvJeHI=";

            nativeBuildInputs = with pkgs.pkgsMusl; [
              pkg-config
            ];

            buildInputs = with pkgs.pkgsMusl; [
              ffmpeg-headless.dev
            ];

            preBuild = ''
              export CGO_ENABLED=1
              export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
              export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
            '';

            ldflags = [
              "-s"
              "-w"
              "-linkmode"
              "external"
              "-extldflags"
              "-static"
            ];

            meta = with pkgs.lib; {
              description = "Steam Query application (musl static)";
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.pkg-config
            ffmpeg-minimal.dev
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