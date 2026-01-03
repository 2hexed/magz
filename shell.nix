{
  pkgs ? import <nixpkgs> { },
}:

let
  project-cleanup = pkgs.writeShellScriptBin "project-cleanup" ''
    rm -fr magz magz_cache.db *.exe *.out *.log magz.config.json .DS_Store *.tmp *.bak pkg/ .vscode/ .idea/
  '';

  first-time-running = pkgs.writeShellScriptBin "first-time-running" ''
    go mod init magz
    go get
    go mod tidy
  '';

  build-binary = pkgs.writeShellScriptBin "build-binary" ''
    go build -o magz
  '';

  fmt-project = pkgs.writeShellScriptBin "fmt-project" ''
    echo "Working on Nix files.."
    nixfmt shell.nix && echo "OK!"

    echo "Working on Go files.."
    gofmt -w *.go && echo "OK!"

    echo "Working on JSON files.."
    prettier --log-level error --tab-width 4 -w *.json && echo "OK!"

    echo "Working on HTML files.."
    prettier --log-level error -w public/*.html && echo "OK!"

    echo "Working on JS files.."
    prettier --log-level error -w public/*.js && echo "OK!"

    echo "Working on Markdown files.."
    prettier --log-level error -w *.md && echo "OK!"
  '';
in

pkgs.mkShell {
  name = "magz";

  buildInputs = ([
    fmt-project
    build-binary
    project-cleanup
    first-time-running
  ])
  ++ (with pkgs; [
    # File formatting
    nodePackages_latest.prettier
    nixfmt-rfc-style
    # Source packages
    go_latest
  ]);

  shellHook = ''
    export GOPATH=$PWD
    echo "------------------------------------"
    echo "🐹 Go Media Server development shell"
    echo "To start: go run main.go"
    echo ""
    echo "First time? Below command executes mod init, mod tidy"
    echo "> first-time-running"
    echo ""
    echo "Building binary? Execute below"
    echo "> build-binary"
  '';
}
