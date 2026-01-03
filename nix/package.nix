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
    hash = "sha256-mTF/YEDtbCRphTVLs2KPevyXYa1Ag7W7Tjhe6IlbQSk=";
  };

  vendorHash = pkgs.lib.fakeHash;

  meta = with pkgs.lib; {
    description = "A simple Go project";
    license = licenses.gpl3;
    maintainers = with lib.maintainers; [ _2hexed ];
    platforms = platforms.linux ++ platforms.darwin;
  };
}
