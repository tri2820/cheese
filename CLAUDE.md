# Using Go
Prefer `go vet` for quick type testing. 
Use `go run main.go` for running (to avoid littering the codebase with binary artifacts).

# Use `cd` with full path

```sh
cd full_path && command
```

# Using Nix
Write deps in `nix.flake`. Use `nix develop` when run commands.

Do NOT call go with nix store path
```
/nix/store/zzvsjgylnphvhms3lgr2qlwdxmc68z66-go-1.25.5/share/go/bin/go
```

