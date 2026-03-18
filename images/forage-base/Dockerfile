FROM nixos/nix:latest

# Enable nix-command and flakes so `nix profile install` works out of the box.
# The upstream nixos/nix image does not always have these enabled in nix.conf.
RUN mkdir -p /etc/nix && \
    echo "experimental-features = nix-command flakes" >> /etc/nix/nix.conf

# git-minimal is already in the base image; only add what's missing.
RUN nix profile install --profile /nix/var/nix/profiles/default --no-write-lock-file \
    nixpkgs#tmux \
    nixpkgs#jq \
    && nix-collect-garbage -d

# Create the agent user with a proper home directory.
# OCI containers don't have NixOS user management, so we do it here.
RUN mkdir -p /home/agent && \
    echo "agent:x:1000:1000::/home/agent:/bin/bash" >> /etc/passwd && \
    echo "agent:x:1000:" >> /etc/group

ENV HOME=/home/agent
WORKDIR /home/agent
