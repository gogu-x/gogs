package def

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// BattleRoomID 将开放房间的 Battle 路由与唯一编号一起交给队友。
func BattleRoomID(server, node int, id string) string {
	return fmt.Sprintf("%d:%d:%s", server, node, id)
}

// BattleRoomRoute 解析共享房间编号，只接受整数节点地址和有效 UUID。
func BattleRoomRoute(id string) (int, int, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return 0, 0, false
	}
	server, e1 := strconv.ParseUint(parts[0], 10, 31)
	node, e2 := strconv.ParseUint(parts[1], 10, 31)
	_, e3 := uuid.Parse(parts[2])
	return int(server), int(node), e1 == nil && e2 == nil && e3 == nil && server > 0 && node > 0
}
