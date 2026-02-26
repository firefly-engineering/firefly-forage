package generator

import _ "embed"

// EvalConfigNix is the embedded eval-config.nix content.
// This is a stripped version of extra-container's eval-config.nix that omits
// the extraModule, allowing the outer container config to evaluate in ~0.5s
// instead of ~13s (no inner NixOS system evaluation is triggered).
//
//go:embed eval_config.nix
var EvalConfigNix string
