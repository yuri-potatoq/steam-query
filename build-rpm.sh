#!/bin/bash
set -e

VERSION=${VERSION:-0.1.0}
RELEASE=${RELEASE:-1}

echo "Building steam-query RPM version ${VERSION}-${RELEASE}"

# Setup RPM build environment
rpmdev-setuptree

# Build the Go binary
echo "Building Go binary..."
export CGO_ENABLED=1
export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"

go build -v -ldflags "-s -w" -o steam-query .

# Create source tarball
mkdir -p steam-query-${VERSION}
cp steam-query steam-query-${VERSION}/
tar -czf ~/rpmbuild/SOURCES/steam-query-${VERSION}.tar.gz steam-query-${VERSION}

# Create RPM spec file
cat > ~/rpmbuild/SPECS/steam-query.spec << EOF
Name:           steam-query
Version:        ${VERSION}
Release:        ${RELEASE}%{?dist}
Summary:        Steam Query application

License:        MIT
URL:            https://github.com/yuri-potatoq/steam-query
Source0:        %{name}-%{version}.tar.gz

Requires:       ffmpeg-free
BuildRequires:  golang >= 1.20
BuildRequires:  gcc
BuildRequires:  pkgconfig
BuildRequires:  ffmpeg-free-devel

%description
A tool for querying Steam data with FFmpeg support.
Built with CGO for FFmpeg integration.

%prep
%setup -q

%build
# Binary is already built, nothing to do here

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 steam-query %{buildroot}%{_bindir}/steam-query

%files
%{_bindir}/steam-query

%changelog
* $(date "+%a %b %d %Y") Steam Query Team <maintainer@example.com> - ${VERSION}-${RELEASE}
- Version ${VERSION} release
EOF

# Build RPM
echo "Building RPM package..."
rpmbuild -ba ~/rpmbuild/SPECS/steam-query.spec

# Copy RPMs to output directory
mkdir -p /output
cp ~/rpmbuild/RPMS/x86_64/*.rpm /output/
cp ~/rpmbuild/SRPMS/*.rpm /output/

echo "RPM packages created:"
ls -lh /output/
