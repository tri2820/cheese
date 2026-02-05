# Using Go
Prefer `go vet` for quick type testing. 
Use `go run main.go` for running (to avoid littering the codebase with binary artifacts).

# Use `cd` with full path

The bash tool doesn't persist working directory between commands. Each command starts fresh from the original directory. You need to use a single command with 'cd /path && cmd' or use the -C flag.

Example:

```sh
cd full_path && go vet ./...
```

# Using Nix
Write deps in `nix.flake`. Use `nix develop` when run commands.

Do NOT call go with nix store path
```
/nix/store/zzvsjgylnphvhms3lgr2qlwdxmc68z66-go-1.25.5/share/go/bin/go
```

