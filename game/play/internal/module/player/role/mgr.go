package role

// Role 保存角色模块拥有的养成、技能和装备穿戴关系。
type Role struct {
	ID           string   `bson:"id"`
	ConfigID     int32    `bson:"config_id"`
	Level        int32    `bson:"level"`
	Star         int32    `bson:"star"`
	Breakthrough int32    `bson:"breakthrough"`
	Skills       []Skill  `bson:"skills"`
	EquipmentIDs []string `bson:"equipment_ids"`
}

// Skill 保存角色已配置技能及其等级。
type Skill struct {
	ConfigID int32 `bson:"config_id"`
	Level    int32 `bson:"level"`
}

// Mgr 管理玩家拥有的角色数据。
type Mgr struct {
	Character *Role `bson:"character"`
}

// New 创建空的角色模块。
func New() *Mgr { return &Mgr{} }

// BattleCharacter 返回用于战斗组装的角色副本。
func (m *Mgr) BattleCharacter() (*Role, bool) {
	if m == nil || m.Character == nil || m.Character.ID == "" {
		return nil, false
	}
	copyRole := *m.Character
	copyRole.Skills = append([]Skill(nil), m.Character.Skills...)
	copyRole.EquipmentIDs = append([]string(nil), m.Character.EquipmentIDs...)
	return &copyRole, true
}
