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
    hash = "sha256-kxiZ3GC5xPcHgMyaU0++Yc07uLbFX76e0csvSaK+usM=";
  };

  vendorHash = pkgs.lib.fakeHash;

  meta = with pkgs.lib; {
    description = "A simple Go project";
    license = licenses.gpl3;
    maintainers = with lib.maintainers; [ _2hexed ];
    platforms = platforms.linux ++ platforms.darwin;
  };
}
