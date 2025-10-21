{
  pkgs ? import <nixpkgs> { },
  srcFiles ? ./.,
}: with pkgs; buildGoModule {
  pname = "steam-query";
  version = "0.1.0";

  src = lib.cleanSourceWith {
    src = srcFiles;
    filter = path: type:
      let baseName = baseNameOf path;
      in !(baseName == "result");
  };

  vendorHash = "sha256-Z78u1XjvL+Zoao2j8MbA1BjuOZjfSluCiFzI8f0OSiI=";

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
