# deepcopy-gen

`deepcopy-gen` recursively scans Go packages and generates `DeepCopy` methods
for structs marked with `// +deepcopy-gen=true`. A package can instead add
`// +deepcopy-gen=package` in its package comment; individual structs can opt
out with `// +deepcopy-gen=false`.

From the repository root, compile the generator and regenerate copies for every
marked package under the working directory with:

```powershell
make deepcopy
```

To run the source generator without compiling the standalone binary, use
`go generate ./game/play/internal/module/player` for the player package tree.

The command writes `zz_generated.deepcopy.go` beside each marked package. The
generator clones pointers to marked structs, maps, slices, arrays, and nested
combinations of those types. It reports unsupported or unmarked struct fields
instead of silently generating a shallow copy.
