{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = inputs: let
    system = "x86_64-linux";
    pkgs = import inputs.nixpkgs {inherit system;};
  in {
    devShells.${system}.default = pkgs.mkShell {
      packages = with pkgs; [
        # frontend
        caddy

        # backend
        go
        gopls

        # util
        inotify-tools

        (python3.withPackages (ppkgs:
          with ppkgs; [
            websockets
            nest-asyncio
          ]))

        (pkgs.writers.writeBashBin "non" ''
          if [ "$1" == "run" ]; then
            python3 ./hot.py &
            hotPID=$!

            caddy run &
            caddyPID=$!

            trap "kill $caddyPID $hotPID 2>/dev/null; exit" SIGINT SIGTERM

            wait -n

            kill $caddyPID $hotPID 2>/dev/null
          fi
        '')
      ];
    };
  };
}
