{
  pkgs ? import <nixpkgs> { },
}:

pkgs.buildGoModule {
  pname = "magz";
  version = "unstable";

  src = pkgs.fetchFromGitHub {
    owner = "2hexed";
    repo = "magz";
    rev = "master";
    hash = "sha256-PaiYjK811QEOzQ1opZC2s6Dr1z/wX4mA9X0zyGXvDek=";
  };

  vendorHash = pkgs.lib.fakeHash; # "sha256-PaiYjK811QEOzQ1opZC2s6Dr1z/wX4mA9X0zyGXvDek=";

  meta = with pkgs.lib; {
    description = "A simple Go project";
    license = licenses.gpl3;
    maintainers = with lib.maintainers; [ _2hexed ];
    platforms = platforms.linux ++ platforms.darwin;
  };
}
