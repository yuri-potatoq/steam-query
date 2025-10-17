{
  description = "Steam Query - Multi-format builds";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        
        # Base package
        steam-query = pkgs.buildGoModule {
          pname = "steam-query";
          version = "0.1.0";

          src = pkgs.lib.cleanSourceWith {
            src = ./.;
            filter = path: type:
              let baseName = baseNameOf path;
              in !(baseName == "result");
          };

          vendorHash = "sha256-Z78u1XjvL+Zoao2j8MbA1BjuOZjfSluCiFzI8f0OSiI=";

          nativeBuildInputs = with pkgs; [ pkg-config ];
          buildInputs = with pkgs; [ ffmpeg-headless.dev ];

          preBuild = ''
            export CGO_ENABLED=1
            export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
            export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
          '';

          ldflags = [ "-s" "-w" ];

          meta = with pkgs.lib; {
            description = "Steam Query application";
            homepage = "https://github.com/yuri-potatoq/steam-query";
            license = licenses.mit;
            maintainers = [ ];
          };
        };
      in
      {
        packages = {
          default = steam-query;

          # Docker/OCI container
          docker = pkgs.dockerTools.buildLayeredImage {
            name = "steam-query";
            tag = "latest";
            
            contents = [
              steam-query
              pkgs.ffmpeg-headless
              pkgs.cacert  # For HTTPS
            ];
            
            config = {
              Cmd = [ "${steam-query}/bin/steam-query" ];
              Env = [
                "SSL_CERT_FILE=${pkgs.cacert}/etc/ssl/certs/ca-bundle.crt"
              ];
            };
          };

          # Debian/Ubuntu .deb package
          deb = pkgs.stdenv.mkDerivation {
            name = "steam-query-deb";
            
            buildInputs = [ pkgs.dpkg ];
            
            src = steam-query;
            
            buildPhase = ''
              mkdir -p deb/DEBIAN deb/usr/bin
              
              # Control file
              cat > deb/DEBIAN/control << EOF
              Package: steam-query
              Version: 0.1.0
              Section: utils
              Priority: optional
              Architecture: amd64
              Depends: ffmpeg
              Maintainer: Your Name <your@email.com>
              Description: Steam Query application
               A tool for querying Steam data
              EOF
              
              # Copy binary
              cp ${steam-query}/bin/steam-query deb/usr/bin/
              
              # Build package
              dpkg-deb --build deb
            '';
            
            installPhase = ''
              mkdir -p $out
              cp deb.deb $out/steam-query_0.1.0_amd64.deb
            '';
          };

          # RPM package for Fedora/RHEL
          rpm = pkgs.stdenv.mkDerivation {
            name = "steam-query-rpm";
            
            nativeBuildInputs = [ pkgs.rpm ];
            
            src = steam-query;
            
            buildPhase = ''
              mkdir -p rpm/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
              mkdir -p rpm/BUILD/usr/bin
              
              # Copy binary
              cp ${steam-query}/bin/steam-query rpm/BUILD/usr/bin/
              
              # Create spec file
              cat > rpm/SPECS/steam-query.spec << EOF
              Name:           steam-query
              Version:        0.1.0
              Release:        1%{?dist}
              Summary:        Steam Query application
              License:        MIT
              Requires:       ffmpeg-free
              
              %description
              A tool for querying Steam data
              
              %install
              mkdir -p %{buildroot}/usr/bin
              cp ${steam-query}/bin/steam-query %{buildroot}/usr/bin/
              
              %files
              /usr/bin/steam-query
              EOF
              
              # Build RPM
              rpmbuild --define "_topdir $(pwd)/rpm" -bb rpm/SPECS/steam-query.spec
            '';
            
            installPhase = ''
              mkdir -p $out
              cp rpm/RPMS/x86_64/*.rpm $out/
            '';
          };

        };

        # Dev shell
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            pkg-config
            ffmpeg-headless.dev
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