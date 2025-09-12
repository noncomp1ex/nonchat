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
        air
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

            # only backend
            if [ "$2" == "back" ]; then
              cd backend

              if [ "$3" == "air" ]; then
                air &
              else
                go run nonchat &
              fi

              pid=$!
              cd ..
              trap "kill $pid 2>/dev/null; exit" SIGINT SIGTERM
              wait -n
              exit
            fi

            tmux split-window -h "python3 ./hot.py"

            tmux split-window -v -b "caddy run"

            tmux select-pane -L

            tmux split-window -v "non run back air"

            tmux select-pane -R
            tmux select-pane -U
          fi
        '')
      ];
    };
  };
}
