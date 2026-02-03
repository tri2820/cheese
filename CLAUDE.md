# Using Go
Prefer `go vet` for quick type testing. 
Use `go run main.go` for running (to avoid littering the codebase with binary artifacts).

# Using Nix
Write deps in `nix.flake`. Use `nix develop` when run commands.

# Use `cd` with full path

```sh
cd full_path && command
```

