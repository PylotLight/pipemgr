package display

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

type PipelineStatus struct {
	ID           string
	Name         string
	Status       string
	Result       string
	CreatedDate  string
	FinishedDate string
	Stages       []StageStatus
	TimeElapsed  string
}

type StageStatus struct {
	ID     uuid.UUID
	Name   string
	Status string
	Result string
	Order  int
}

type model struct {
	statuses []PipelineStatus
	// stageStatuses []StageStatus
	pipelineTable  table.Model
	stageTable     table.Model
	focusIndex     int
	pipelineCursor int
	stageCursor    int
	width          int
	height         int
}

func InitialModel() model {
	pipelineColumns := []table.Column{
		{Title: "Pipeline", Width: 30},
		{Title: "Status", Width: 10},
		{Title: "Result", Width: 10},
		{Title: "Elapsed", Width: 10},
	}

	stageColumns := []table.Column{
		{Title: "Stage", Width: 10},
		{Title: "Status", Width: 10},
		{Title: "Result", Width: 10},
	}

	pipelineTable := table.New(
		table.WithColumns(pipelineColumns),
		table.WithFocused(true),
		table.WithHeight(30),
	)

	stageTable := table.New(
		table.WithColumns(stageColumns),
		table.WithFocused(false),
		table.WithHeight(30),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	pipelineTable.SetStyles(s)
	stageTable.SetStyles(s)

	return model{
		pipelineTable:  pipelineTable,
		stageTable:     stageTable,
		focusIndex:     0,
		pipelineCursor: 0,
		stageCursor:    0,
		width:          100,
		height:         30,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	// println()
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.focusIndex = (m.focusIndex + 1) % 2
		case "up", "down":
			if m.focusIndex == 0 {
				m.pipelineTable, cmd = m.pipelineTable.Update(msg)
				m.updateStagesTable()
			} else {
				cursorMov := m.stageTable.Cursor() + 1
				if msg.String() == "up" {
					cursorMov = m.stageTable.Cursor() - 1
				}
				m.stageTable, cmd = m.stageTable.Update(msg)
				m.stageTable.SetCursor(cursorMov)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.pipelineTable.SetHeight(m.height/2 - 2)
		m.stageTable.SetHeight(m.height/2 - 2)
	case []PipelineStatus:
		m.statuses = msg
		m.updateTables()
	}
	return m, cmd
}

func (m *model) updateTables() {
	var pipelineRows []table.Row
	sort.Slice(m.statuses, func(i, j int) bool {
		return m.statuses[i].Name < m.statuses[j].Name
	})
	for _, status := range m.statuses {
		pipelineRows = append(pipelineRows, table.Row{
			status.Name,
			status.Status,
			status.Result,
			status.TimeElapsed,
		})
	}
	m.pipelineTable.SetRows(pipelineRows)
	m.updateStagesTable()
}

// func (m *model) updateStagesTable() {
// 	if len(m.statuses) > 0 {
// 		selectedPipeline := m.statuses[m.pipelineTable.Cursor()]
// 		var stageRows []table.Row
// 		for _, stage := range selectedPipeline.Stages {
// 			stageRows = append(stageRows, table.Row{
// 				strconv.Itoa(stage.ID),
// 				stage.Name,
// 				stage.Status,
// 				stage.Result,
// 			})
// 		}
// 		// Sort stages by ID
// 		sort.Slice(stageRows, func(i, j int) bool {
// 			idI, _ := strconv.Atoi(stageRows[i][0])
// 			idJ, _ := strconv.Atoi(stageRows[j][0])
// 			return idI < idJ
// 		})
// 		m.stageTable.SetRows(stageRows)
// 	}
// }

func (m *model) updateStagesTable() {
	if len(m.statuses) > 0 {
		selectedPipeline := m.statuses[m.pipelineTable.Cursor()]
		// println(selectedPipeline.Name)

		sort.Slice(selectedPipeline.Stages, func(i, j int) bool {
			return selectedPipeline.Stages[i].Order < selectedPipeline.Stages[j].Order
		})

		var stageRows []table.Row
		for _, stage := range selectedPipeline.Stages {
			stageRows = append(stageRows, table.Row{
				stage.Name,
				stage.Status,
				stage.Result,
			})
		}
		m.stageTable.SetRows(stageRows)
	}
}

func (m model) View() string {
	if m.width < 80 || m.height < 15 {
		return "Terminal window too small. Please resize."
	}

	focusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("62"))

	unfocusedStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240"))

	// mainTableWidth := m.width/2 - 2
	// stageInfoWidth := m.width/2 - 2

	mainTableWidth := m.width/2 - 4
	stageInfoWidth := m.width/3 - 4

	m.pipelineTable.SetWidth(mainTableWidth)
	m.stageTable.SetWidth(stageInfoWidth)

	var mainTable, stageTable string

	if m.focusIndex == 0 {
		mainTable = focusedStyle.Width(mainTableWidth).Render(m.pipelineTable.View())
		stageTable = unfocusedStyle.Width(stageInfoWidth).Render(m.stageTable.View())
	} else {
		mainTable = unfocusedStyle.Width(mainTableWidth).Render(m.pipelineTable.View())
		stageTable = focusedStyle.Width(stageInfoWidth).Render(m.stageTable.View())
	}

	// if m.focusIndex == 0 {
	// 	mainTable = focusedStyle.Render(m.pipelineTable.View())
	// 	stageTable = unfocusedStyle.Render(m.stageTable.View())
	// } else {
	// 	mainTable = unfocusedStyle.Render(m.pipelineTable.View())
	// 	stageTable = focusedStyle.Render(m.stageTable.View())
	// }

	mainView := lipgloss.JoinHorizontal(lipgloss.Left, mainTable, stageTable)
	// mainView := lipgloss.JoinVertical(lipgloss.Left, mainTable, stageTable)

	help := "\nUse ↑/↓ to navigate. Tab to switch focus. Press q to quit."
	return lipgloss.JoinVertical(lipgloss.Left, mainView, help)
}

func PipelineStatuses(statuses []PipelineStatus) {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	go func() {
		for {
			p.Send(statuses)
			time.Sleep(time.Second)
		}
	}()
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
	}
}
