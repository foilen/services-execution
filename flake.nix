{
  description = "services-execution: an application that runs as root and executes multiple applications";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "services-execution";
          version = "0.0.0";

          src = ./.;

          subPackages = [ "execution" ];

          vendorHash = null;

          meta = with pkgs.lib; {
            description = "An application that runs as root and executes multiple applications";
            homepage = "https://github.com/foilen/services-execution";
            license = licenses.mit;
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [ pkgs.go ];
        };
      });
}
