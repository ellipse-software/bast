package ui

import tea "charm.land/bubbletea/v2"

// FilterIdleMouseMotion drops mouse-move events while the scrollbar is not
// being dragged. Cell-motion mode still delivers drag moves; ignoring the rest
// avoids a full View rebuild on every pointer twitch.
func FilterIdleMouseMotion(model tea.Model, msg tea.Msg) tea.Msg {
	app, ok := model.(*App)
	if !ok {
		return msg
	}
	if _, isMotion := msg.(tea.MouseMotionMsg); isMotion && !app.scrollbarDragging {
		return nil
	}
	return msg
}
