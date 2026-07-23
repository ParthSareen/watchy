package tui

func (m *Model) recalcLayout() {
	m.boxHeight = m.height - 2
	m.innerHeight = m.boxHeight - 3

	if m.leftHidden {
		m.leftWidth = 0
		m.rightWidth = m.width - 2
	} else {
		m.leftWidth = m.width * 30 / 100
		m.rightWidth = m.width - m.leftWidth - 4
		if m.rightWidth < 1 {
			m.rightWidth = 1
		}
	}

	if m.rightMode == modeSplit {
		splitWidth := m.rightWidth/2 - 1
		m.logViewport.Width = splitWidth
		m.logViewport.Height = m.innerHeight
		m.chat.SetSize(splitWidth, m.innerHeight)
		return
	}
	m.logViewport.Width = m.rightWidth
	m.logViewport.Height = m.innerHeight
	m.chat.SetSize(m.rightWidth, m.innerHeight)
}
