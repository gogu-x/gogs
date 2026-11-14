package role

// State 返回主角色身份与成长状态，未加载时返回零值。
func (m *Mgr) State() (string, int32, int32) {
	if character, ok := m.GetCharacter(); ok {
		return character.GetID(), character.GetLevel(), character.GetBreakthrough()
	}
	return "", 0, 0
}
