package generator

import _ "embed"

// EvalConfigNix is the embedded eval-config.nix content.
// This is a minimal NixOS module evaluator for container configs that imports
// only the ~7 modules needed (instead of 700+), allowing container configs
// to evaluate in ~0.5s instead of ~13s.
//
//go:embed eval_config.nix
var EvalConfigNix string
