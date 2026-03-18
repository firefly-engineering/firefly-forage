FROM nixos/nix:latest

# Enable nix-command and flakes so `nix profile install` works out of the box.
# The upstream nixos/nix image does not always have these enabled in nix.conf.
RUN mkdir -p /etc/nix && \
    echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

RUN nix profile install --profile /nix/var/nix/profiles/default --no-write-lock-file \
    nixpkgs#tmux \
    nixpkgs#git \
    nixpkgs#jq \
    && nix-collect-garbage -d
