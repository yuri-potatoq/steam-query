{
  pkgs ? import <nixpkgs> { },
  version,
  steam-query,
}: with pkgs; stdenv.mkDerivation {
  name = "steam-query_${version}_amd64.deb";

  nativeBuildInputs = [ dpkg fakeroot ];

  buildCommand = ''
    mkdir -p package/DEBIAN
    mkdir -p package/usr/bin

    cat > package/DEBIAN/control << EOF
    Package: steam-query
    Version: ${version}
    Section: utils
    Priority: optional
    Architecture: amd64
    Depends: ffmpeg, ca-certificates
    Maintainer: Steam Query Team <maintainer@example.com>
    Description: Steam Query application
     A tool for querying Steam data with FFmpeg support.
     Requires ca-certificates for TLS connections.
    EOF

    cp ${steam-query}/bin/steam-query package/usr/bin/
    chmod 755 package/usr/bin/steam-query

    # Build .deb
    fakeroot dpkg-deb --build package $out
  '';
}
