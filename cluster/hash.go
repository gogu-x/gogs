package cluster

import (
	"hash/fnv"
	"strconv"
)

// HashPick 根据 uid hash 取模选取节点，节点列表不变时结果固定。
func HashPick(serverID string, uid uint64) (InstInfo, bool) {
	ringsMu.RLock()
	nodes := rings[serverID]
	ringsMu.RUnlock()
	if len(nodes) == 0 {
		return InstInfo{}, false
	}
	h := fnvHash(strconv.FormatUint(uid, 10))
	return nodes[h%uint32(len(nodes))], true
}

func fnvHash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
