package talent

// Mgr persists learned nodes; clients cannot supply a point balance.
// +deepcopy-gen=true
//
//go:generate accessor-gen
type Mgr struct {
	Revision uint64         `bson:"revision"` // Mutation revision.
	Learned  map[int32]bool `bson:"learned"`  // Learned definition identifiers.
}

// New grants only the root nodes declared by the loaded catalog.
func New() *Mgr {
	if !catalogReady() {
		return nil
	}
	return &Mgr{Revision: 1, Learned: rootNodes()}
}
