{
  pkgs ? import <nixpkgs> { },
  srcFiles ? ./.,
  version,
}: with pkgs; buildGoModule {
  pname = "steam-query";
  version = version;

  src = lib.cleanSourceWith {
    src = srcFiles;
    filter = path: type:
      let baseName = baseNameOf path;
      in !(baseName == "result");
  };

  vendorHash = "sha256-s26BHPcVqHhUmtU2sZWLWzXlglj0yICGz1OXCDXrvNI=";

  nativeBuildInputs = [
    pkg-config
  ];

  buildInputs = [
    ffmpeg-headless.dev
    ffmpeg-headless
  ];

  preBuild = ''
    export CGO_CFLAGS="$(pkg-config --cflags libavformat libavcodec libavutil)"
    export CGO_LDFLAGS="$(pkg-config --libs libavformat libavcodec libavutil)"
  '';

  ldflags = [ "-s" "-w" ];

  meta = with lib; {
    description = "Steam Query application (Nix dynamic)";
  };
}
