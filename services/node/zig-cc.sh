#!/bin/bash
# Wrapper for zig cc that filters out the --target= flag added by cc-rs
# (conflicts with zig's -target flag)
ARGS=()
for arg in "$@"; do
  case "$arg" in
    --target=*) ;; # skip cc-rs target flag
    *) ARGS+=("$arg") ;;
  esac
done
exec zig cc -target x86_64-linux-gnu "${ARGS[@]}"
